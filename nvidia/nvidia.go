package main

import (
	"flag"
	"io"
	"os"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/simulation"
	"github.com/sarchlab/mgpusim/v4/nvidia/benchmark"
	"github.com/sarchlab/mgpusim/v4/nvidia/platform"
	"github.com/sarchlab/mgpusim/v4/nvidia/runner"
	"github.com/tebeka/atexit"

	log "github.com/sirupsen/logrus"
)

type Params struct {
	TraceDir *string
	Device   *string
}

// get trace directory from parameter
func parseFlags() *Params {
	params := &Params{
		TraceDir: flag.String("trace-dir", "data/simple-trace-example", "The directory that contains the trace files"),
		Device:   flag.String("device", "H100", "Device type: H100 or A100 (required)"),
	}

	flag.Parse()

	return params
}

func main() {
	params := parseFlags()
	initLogSetting()

	benchmark := new(benchmark.BenchmarkBuilder).
		WithTraceDirectory(*params.TraceDir).
		Build()

	// A100
	// platform := new(platform.A100PlatformBuilder).
	// 	WithFreq(1065 * sim.MHz).
	// 	Build()
	var plat *platform.Platform // <-- declare outside if/else

	b := simulation.MakeBuilder()
	simulation := b.WithoutMonitoring().Build()

	if *params.Device == "A100" {
		plat = (&platform.A100PlatformBuilder{}).
			WithFreq(1 * sim.Hz).
			WithSimulation(simulation).
			Build()
	} else if *params.Device == "H100" {
		plat = (&platform.H100PlatformBuilder{}).
			WithFreq(1 * sim.Hz).
			WithSimulation(simulation).
			Build()
	} else {
		log.Fatal("Invalid device type. Please specify 'A100' or 'H100'.")
		return
	}

	// tracingBackend := tracing.NewDBTracer("")
	// tracingBackend.Init()
	// b := simulation.MakeBuilder()
	// simulation := b.Build()

	runner := new(runner.RunnerBuilder).
		WithPlatform(plat).
		WithSimulation(simulation).
		Build()
	// runner.Init()
	runner.AddBenchmark(benchmark)

	runner.Run()

	atexit.Exit(0)
}

func initLogSetting() {
	file, err := os.OpenFile("logfile.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("Failed to open log file:", err)
	}
	multiWriter := io.MultiWriter(file) //, os.Stdout)

	log.SetOutput(multiWriter)
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
}
