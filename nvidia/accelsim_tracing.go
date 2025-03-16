package main

import (
	"flag"
	"io"
	"os"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/nvidia/benchmark"
	"github.com/sarchlab/mgpusim/v4/nvidia/platform"
	"github.com/sarchlab/mgpusim/v4/nvidia/runner"
	"github.com/tebeka/atexit"

	log "github.com/sirupsen/logrus"
)

type Params struct {
	TraceDir *string
}

// get trace directory from parameter
func parseFlags() *Params {
	params := &Params{
		TraceDir: flag.String("trace-dir", "data/simple-trace-example", "The directory that contains the trace files"),
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
	platform := new(platform.A100PlatformBuilder).
		WithFreq(1 * sim.Hz).
		Build()

	runner := new(runner.RunnerBuilder).
		WithPlatform(platform).
		Build()
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
