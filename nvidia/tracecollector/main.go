// Command tracecollector prepares the inputs of the nvidia simulator on a
// machine with an NVIDIA GPU. By default it does two separate runs of a CUDA
// program:
//
//   - Trace: runs the program under the Accel-Sim NVBit tracer and turns the
//     raw traces into the kernelslist.g + .traceg format that the simulator
//     reads.
//   - Profile: runs the program under Nsight Compute (ncu) and records the
//     cycles and duration of every kernel on the real GPU.
//
// -trace-only and -profile-only do just one of them. A profile that cannot
// be taken (no ncu, no GPU, no permission, ...) is reported as not available
// and does not fail a trace.
//
// Usage:
//
//	go run ./nvidia/tracecollector \
//	    -tracer    <accel-sim>/util/tracer_nvbit/tracer_tool/tracer_tool.so \
//	    -processor <accel-sim>/util/tracer_nvbit/tracer_tool/traces-processing/post-traces-processing \
//	    -out traces/atax \
//	    -- nvidia/benchmarks/bin/polybench-atax -x 256 -y 256
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type options struct {
	tracer      string
	processor   string
	ncu         string
	outDir      string
	keepRaw     bool
	traceOnly   bool
	profileOnly bool
	command     []string
}

func (o options) doTrace() bool   { return !o.profileOnly }
func (o options) doProfile() bool { return !o.traceOnly }

func main() {
	opts := options{}
	flag.StringVar(&opts.tracer, "tracer", os.Getenv("TRACER_TOOL"),
		"Path to the Accel-Sim NVBit tracer_tool.so (env TRACER_TOOL).")
	flag.StringVar(&opts.processor, "processor", os.Getenv("TRACE_PROCESSOR"),
		"Path to Accel-Sim's post-traces-processing (env TRACE_PROCESSOR).")
	flag.StringVar(&opts.ncu, "ncu", os.Getenv("NCU"),
		"Path to Nsight Compute's ncu (env NCU). Default: PATH, then the usual "+
			"CUDA install locations.")
	flag.StringVar(&opts.outDir, "out", "",
		"Output directory. It must not contain a trace yet unless -profile-only "+
			"is given.")
	flag.BoolVar(&opts.keepRaw, "keep-raw", false,
		"Keep the raw traces and kernelslist_processed after post-processing.")
	flag.BoolVar(&opts.traceOnly, "trace-only", false,
		"Only collect the trace; do not profile.")
	flag.BoolVar(&opts.profileOnly, "profile-only", false,
		"Only profile with ncu; do not trace. -out may already hold a trace.")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"Usage: %s [flags] -- <cuda program> [args...]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	opts.command = flag.Args()

	os.Exit(run(context.Background(), opts))
}

// run validates the options, collects the trace and the profile, prints the
// "# Trace Info" and "# Profile Info" reports, and returns the exit code.
func run(ctx context.Context, opts options) int {
	if err := validate(&opts); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}

	if err := os.MkdirAll(opts.outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}

	var traceErr error
	if opts.doTrace() {
		traceErr = collectTrace(ctx, opts)
	}

	var profile *profileResult
	if opts.doProfile() {
		profile = collectProfile(ctx, opts)
	}

	fmt.Println()
	fmt.Print(traceReport(opts, traceErr))
	fmt.Println()

	profileText := profileReport(opts, profile)
	fmt.Print(profileText)

	if profile != nil {
		saveProfileReport(opts.outDir, profile, profileText)
	}

	switch {
	case traceErr != nil:
		return 1
	case opts.profileOnly && !profile.available():
		return 1
	default:
		return 0
	}
}

func validate(opts *options) error {
	if len(opts.command) == 0 {
		return errors.New("no CUDA program given; put it after --")
	}

	if opts.traceOnly && opts.profileOnly {
		return errors.New("-trace-only and -profile-only cannot be used together")
	}

	if opts.outDir == "" {
		return errors.New("-out is required")
	}

	var err error
	if opts.outDir, err = filepath.Abs(opts.outDir); err != nil {
		return err
	}

	if !opts.doTrace() {
		return nil
	}

	for _, p := range []*string{&opts.tracer, &opts.processor} {
		if *p == "" {
			return errors.New("both -tracer and -processor are required to " +
				"trace (or pass -profile-only)")
		}

		if *p, err = filepath.Abs(*p); err != nil {
			return err
		}

		if _, err := os.Stat(*p); err != nil {
			return fmt.Errorf("cannot find %s: %w", *p, err)
		}
	}

	if entries, err := os.ReadDir(opts.outDir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s is not empty; remove it or choose another -out",
			opts.outDir)
	}

	return nil
}

// traceReport formats the "# Trace Info" section.
func traceReport(opts options, traceErr error) string {
	var b strings.Builder

	b.WriteString("# Trace Info\n")

	switch {
	case !opts.doTrace():
		b.WriteString("Status:         skipped (-profile-only)\n")
		return b.String()
	case traceErr != nil:
		fmt.Fprintf(&b, "Status:         failed: %v\n", traceErr)
		return b.String()
	}

	summary, err := summarizeTrace(opts.outDir)
	if err != nil {
		fmt.Fprintf(&b, "Status:         failed: %v\n", err)
		return b.String()
	}

	b.WriteString("Status:         OK\n")
	fmt.Fprintf(&b, "Directory:      %s\n", opts.outDir)
	fmt.Fprintf(&b, "Kernels:        %d\n", summary.kernels)
	fmt.Fprintf(&b, "Memory copies:  %d\n", summary.memcpys)
	fmt.Fprintf(&b, "Trace size:     %.4f MB (%s and %d .traceg files)\n",
		float64(summary.bytes)/bytesPerMB, simulatorIndex, summary.kernels)
	fmt.Fprintf(&b, "Simulate with:  ./nvidia/nvidia -trace-dir %s\n", opts.outDir)

	return b.String()
}
