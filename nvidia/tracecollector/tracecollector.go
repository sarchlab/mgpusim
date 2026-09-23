// Command tracecollector runs a CUDA program under the Accel-Sim NVBit tracer
// and turns the raw traces into the kernelslist.g + .traceg format that the
// nvidia simulator reads. It must run on a machine with an NVIDIA GPU.
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

const kernelsList = "kernelslist"

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
		"Keep the raw .trace files after post-processing.")
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

	if fileExists(filepath.Join(opts.outDir, "kernelslist.g")) {
		return fmt.Errorf("%s already contains a trace", opts.outDir)
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

	return cmd.Run()
}

// hoistTracesFolder moves the files out of <out>/traces for tracer versions
// that always write into a "traces" sub-directory.
func hoistTracesFolder(outDir string) error {
	sub := filepath.Join(outDir, "traces")
	if !fileExists(filepath.Join(sub, kernelsList)) {
		return nil
	}

	entries, err := os.ReadDir(sub)
	if err != nil {
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

// decompress unpacks kernel-*.trace.xz files and rewrites kernelslist so that
// it lists the uncompressed files.
func decompress(ctx context.Context, outDir string) error {
	listPath := filepath.Join(outDir, kernelsList)

	lines, err := readLines(listPath)
	if err != nil {
		return fmt.Errorf("the tracer did not write %s; did the program "+
			"launch any kernel on the GPU? %w", listPath, err)
	}

	changed := false

	for i, line := range lines {
		name := strings.TrimSpace(line)
		if !strings.HasPrefix(name, "kernel") || !strings.HasSuffix(name, ".xz") {
			continue
		}

		cmd := exec.CommandContext(ctx, "xz", "-d", "-f", filepath.Join(outDir, name))
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to decompress %s: %w", name, err)
		}

		lines[i] = strings.TrimSuffix(name, ".xz")
		changed = true
	}

	if !changed {
		return nil
	}

	return os.WriteFile(listPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func postProcess(ctx context.Context, opts options) error {
	listPath := filepath.Join(opts.outDir, kernelsList)

	cmd := exec.CommandContext(ctx, opts.processor, listPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("post-processing failed: %w", err)
	}

	if err := checkProcessedTrace(opts.outDir); err != nil {
		return err
	}

	if opts.keepRaw {
		return nil
	}

	raw, err := filepath.Glob(filepath.Join(opts.outDir, "kernel-*.trace"))
	if err != nil {
		return err
	}

	for _, f := range raw {
		if err := os.Remove(f); err != nil {
			return err
		}
	}

	return nil
}

// checkProcessedTrace makes sure that the post-processor wrote plain-text
// .traceg files, which is the only format the simulator reads.
func checkProcessedTrace(outDir string) error {
	lines, err := readLines(filepath.Join(outDir, "kernelslist.g"))
	if err != nil {
		return fmt.Errorf("post-processing did not produce kernelslist.g: %w", err)
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
