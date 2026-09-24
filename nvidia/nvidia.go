// Command nvidia simulates an NVIDIA GPU running a SASS trace collected with
// the Accel-Sim NVBit tracer. See README.md for how to collect a trace.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sarchlab/mgpusim/v5/nvidia/platform"
	"github.com/sarchlab/mgpusim/v5/nvidia/runner"
	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

func main() {
	traceDir := flag.String("trace-dir", "",
		"The directory that contains kernelslist.g and the .traceg files.")
	device := flag.String("device", "H100", "The GPU to simulate: H100 or A100.")
	visTracing := flag.Bool("trace-vis", false,
		"Record a Daisen visualization trace into an SQLite database (slow).")
	progress := flag.Duration("progress", 0,
		"Print the simulation status to stderr at this interval, e.g. 5s.")
	output := flag.String("output", "",
		"The name of the SQLite database written with -trace-vis.")
	flag.Parse()

	opts := runner.Options{
		TraceDir:   *traceDir,
		VisTracing: *visTracing,
		OutputFile: *output,
		Progress:   *progress,
	}

	if err := run(opts, *device); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(opts runner.Options, deviceName string) error {
	traceDir := opts.TraceDir
	if traceDir == "" {
		return fmt.Errorf("-trace-dir is required")
	}

	kernelsList := filepath.Join(traceDir, trace.KernelsListFileName)
	if _, err := os.Stat(kernelsList); err != nil {
		return fmt.Errorf("%s not found; is %s a post-processed trace directory?",
			kernelsList, traceDir)
	}

	device, err := platform.DeviceByName(deviceName)
	if err != nil {
		return err
	}

	opts.Device = device

	result, err := runner.Run(opts)
	if err != nil {
		return err
	}

	fmt.Printf("device:          %s @ %.0f MHz\n",
		result.Device, float64(result.Freq)/1e6)
	fmt.Printf("kernels:         %d\n", result.NumKernels)
	fmt.Printf("warps:           %d\n", result.NumWarps)
	fmt.Printf("instructions:    %d\n", result.NumInsts)
	fmt.Printf("simulated time:  %.3f us\n", result.Seconds()*1e6)
	fmt.Printf("simulated cycles: %d\n", result.Cycles())

	return nil
}
