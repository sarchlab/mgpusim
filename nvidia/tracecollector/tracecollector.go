// Command tracecollector runs a CUDA program under the Accel-Sim NVBit tracer
// and turns the raw traces into the kernelslist.g + .traceg format that the
// nvidia simulator reads. It must run on a machine with an NVIDIA GPU.
//
// The processing follows mnt-collector: move the files out of the tracer's
// "traces" sub-directory, find the raw kernel list (kernelslist or
// kernelslist_ctx_<ctx>), decompress the kernel-*.trace.xz files, write a
// kernel list without the .xz suffixes, and run post-traces-processing on it.
//
// Usage:
//
//	go run ./nvidia/tracecollector \
//	    -tracer    <accel-sim>/util/tracer_nvbit/tracer_tool/tracer_tool.so \
//	    -processor <accel-sim>/util/tracer_nvbit/tracer_tool/traces-processing/post-traces-processing \
//	    -out traces/atax \
//	    -- nvidia/benchmarks/bin/atax -x 256 -y 256
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	processedList  = "kernelslist_processed"
	simulatorIndex = "kernelslist.g"
)

type options struct {
	tracer    string
	processor string
	outDir    string
	keepRaw   bool
	command   []string
}

func main() {
	opts := options{}
	flag.StringVar(&opts.tracer, "tracer", os.Getenv("TRACER_TOOL"),
		"Path to the Accel-Sim NVBit tracer_tool.so (env TRACER_TOOL).")
	flag.StringVar(&opts.processor, "processor", os.Getenv("TRACE_PROCESSOR"),
		"Path to Accel-Sim's post-traces-processing (env TRACE_PROCESSOR).")
	flag.StringVar(&opts.outDir, "out", "",
		"Output directory for the trace. It must not contain a trace yet.")
	flag.BoolVar(&opts.keepRaw, "keep-raw", false,
		"Keep the raw traces and kernelslist_processed after post-processing.")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"Usage: %s [flags] -- <cuda program> [args...]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	opts.command = flag.Args()

	if err := collect(context.Background(), opts); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func collect(ctx context.Context, opts options) error {
	if err := validate(&opts); err != nil {
		return err
	}

	if err := os.MkdirAll(opts.outDir, 0o755); err != nil {
		return err
	}

	fmt.Printf("Tracing %s into %s\n", strings.Join(opts.command, " "), opts.outDir)

	if err := runTraced(ctx, opts); err != nil {
		return fmt.Errorf("traced run failed: %w", err)
	}

	if err := hoistTracesFolder(opts.outDir); err != nil {
		return err
	}

	if err := decompress(ctx, opts.outDir); err != nil {
		return err
	}

	if err := postProcess(ctx, opts); err != nil {
		return err
	}

	fmt.Printf("\nTrace ready: %s\n", opts.outDir)
	fmt.Printf("Simulate it with:\n  go run ./nvidia -trace-dir %s\n", opts.outDir)

	return nil
}

func validate(opts *options) error {
	if len(opts.command) == 0 {
		return errors.New("no CUDA program given; put it after --")
	}

	if opts.outDir == "" {
		return errors.New("-out is required")
	}

	var err error

	for _, p := range []*string{&opts.tracer, &opts.processor, &opts.outDir} {
		if *p == "" {
			return errors.New("both -tracer and -processor are required")
		}

		if *p, err = filepath.Abs(*p); err != nil {
			return err
		}
	}

	for _, p := range []string{opts.tracer, opts.processor} {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("cannot find %s: %w", p, err)
		}
	}

	if entries, err := os.ReadDir(opts.outDir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s is not empty; remove it or choose another -out",
			opts.outDir)
	}

	return nil
}

func runTraced(ctx context.Context, opts options) error {
	cmd := exec.CommandContext(ctx, opts.command[0], opts.command[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"LD_PRELOAD="+opts.tracer,
		"USER_DEFINED_FOLDERS=1",
		"TRACES_FOLDER="+opts.outDir,
	)

	// Newer tracers record register values by default (trace version 6),
	// which the simulator does not need. Older tracers ignore the variable.
	if _, set := os.LookupEnv("ALLOW_REG_VAL_TRACING"); !set {
		cmd.Env = append(cmd.Env, "ALLOW_REG_VAL_TRACING=0")
	}

	return cmd.Run()
}

// hoistTracesFolder moves the files out of <out>/traces, where the tracer
// writes them when TRACES_FOLDER is set.
func hoistTracesFolder(outDir string) error {
	sub := filepath.Join(outDir, "traces")

	entries, err := os.ReadDir(sub)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	for _, e := range entries {
		err := os.Rename(filepath.Join(sub, e.Name()), filepath.Join(outDir, e.Name()))
		if err != nil {
			return err
		}
	}

	return os.Remove(sub)
}

// findRawKernelsList finds the kernel list written by the tracer. Depending
// on the tracer version it is called kernelslist or kernelslist_ctx_<ctx>.
func findRawKernelsList(outDir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(outDir, "kernelslist*"))
	if err != nil {
		return "", err
	}

	var lists []string

	for _, m := range matches {
		base := filepath.Base(m)
		if strings.HasSuffix(base, ".g") || base == processedList {
			continue
		}

		lists = append(lists, m)
	}

	switch len(lists) {
	case 0:
		return "", fmt.Errorf("the tracer did not write a kernelslist file in %s; "+
			"did the program launch any kernel on the GPU?", outDir)
	case 1:
		return lists[0], nil
	default:
		return "", fmt.Errorf("found %d kernel lists (%s): the program used "+
			"more than one CUDA context, which is not supported",
			len(lists), strings.Join(lists, ", "))
	}
}

// decompress unpacks the kernel-*.trace.xz files listed in the raw kernel list
// and writes kernelslist_processed, which lists the uncompressed files.
func decompress(ctx context.Context, outDir string) error {
	rawList, err := findRawKernelsList(outDir)
	if err != nil {
		return err
	}

	lines, err := readLines(rawList)
	if err != nil {
		return err
	}

	for i, line := range lines {
		name := strings.TrimSpace(line)
		lines[i] = name

		if !strings.HasPrefix(name, "kernel") || !strings.HasSuffix(name, ".xz") {
			continue
		}

		if err := unxz(ctx, filepath.Join(outDir, name)); err != nil {
			return err
		}

		lines[i] = strings.TrimSuffix(name, ".xz")
	}

	return os.WriteFile(filepath.Join(outDir, processedList),
		[]byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func unxz(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "xz", "-d", "-f", path)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to decompress %s: %w", path, err)
	}

	return nil
}

// postProcess runs post-traces-processing on kernelslist_processed. Older
// versions always write plain-text .traceg files. Newer versions write
// compressed .tracez files unless given --text, so postProcess reruns them
// with --text when needed.
func postProcess(ctx context.Context, opts options) error {
	listPath := filepath.Join(opts.outDir, processedList)

	if err := runProcessor(ctx, opts.processor, listPath); err != nil {
		return err
	}

	if usesTracez(opts.outDir) {
		fmt.Println("The post-processor wrote .tracez files; rerunning it with --text.")

		if err := runProcessor(ctx, opts.processor, listPath, "--text"); err != nil {
			return err
		}
	}

	if err := decompressProcessed(ctx, opts.outDir); err != nil {
		return err
	}

	if err := checkProcessedTrace(opts.outDir); err != nil {
		return err
	}

	return removeIntermediateFiles(opts)
}

func runProcessor(ctx context.Context, processor string, args ...string) error {
	cmd := exec.CommandContext(ctx, processor, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("post-processing failed: %w", err)
	}

	return nil
}

func usesTracez(outDir string) bool {
	lines, err := readLines(filepath.Join(outDir, simulatorIndex))
	if err != nil {
		return false
	}

	for _, line := range lines {
		if strings.HasSuffix(strings.TrimSpace(line), ".tracez") {
			return true
		}
	}

	return false
}

// decompressProcessed unpacks .traceg.xz files, which some post-processor
// versions write when their input is compressed.
func decompressProcessed(ctx context.Context, outDir string) error {
	indexPath := filepath.Join(outDir, simulatorIndex)

	lines, err := readLines(indexPath)
	if err != nil {
		return fmt.Errorf("post-processing did not produce %s: %w",
			simulatorIndex, err)
	}

	changed := false

	for i, line := range lines {
		name := strings.TrimSpace(line)
		if !strings.HasPrefix(name, "kernel") || !strings.HasSuffix(name, ".xz") {
			continue
		}

		if err := unxz(ctx, filepath.Join(outDir, name)); err != nil {
			return err
		}

		lines[i] = strings.TrimSuffix(name, ".xz")
		changed = true
	}

	if !changed {
		return nil
	}

	return os.WriteFile(indexPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// removeIntermediateFiles deletes the raw traces and the processed kernel
// list, as mnt-collector did. The tracer's own kernel list and stats file
// are small and kept for reference.
func removeIntermediateFiles(opts options) error {
	if opts.keepRaw {
		return nil
	}

	patterns := []string{"kernel-*.trace", "kernel-*.trace.xz", processedList}

	for _, pattern := range patterns {
		files, err := filepath.Glob(filepath.Join(opts.outDir, pattern))
		if err != nil {
			return err
		}

		for _, f := range files {
			if err := os.Remove(f); err != nil {
				return err
			}
		}
	}

	return nil
}

// checkProcessedTrace makes sure that the post-processor wrote plain-text
// .traceg files, which is the only format the simulator reads.
func checkProcessedTrace(outDir string) error {
	lines, err := readLines(filepath.Join(outDir, simulatorIndex))
	if err != nil {
		return fmt.Errorf("post-processing did not produce %s: %w",
			simulatorIndex, err)
	}

	for _, line := range lines {
		name := strings.TrimSpace(line)
		if strings.HasPrefix(name, "kernel") && !strings.HasSuffix(name, ".traceg") {
			return fmt.Errorf("kernelslist.g lists %s; the simulator only reads "+
				"plain-text .traceg files", name)
		}
	}

	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	return lines, scanner.Err()
}
