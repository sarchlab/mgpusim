package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/benchmark"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpu"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
)

type inputArguments struct {
	inputTraceDir string
	// deparse        bool
	// outputTraceDir string
}

func getInputArguments() *inputArguments {
	i := &inputArguments{}

	flag.Usage = func() {
		fmt.Println("Usage: ./as_trace_parser [options] trace")
		flag.PrintDefaults()
	}

	flag.Parse()
	if len(flag.Args()) < 1 {
		flag.Usage()
		log.Panic("Error: should specify an input trace")
	}

	i.inputTraceDir = flag.Arg(0)
	return i
}

func buildAmpereGPU() *gpu.GPU {
	gpu := gpu.NewGPU().WithGPUStrategy("default").
		WithGPCNum(8).WithGPCStrategy("default").WithL2CacheSize(4*1024*1024*nvidia.BYTE).
		WithSMNum(16).WithSMStrategy("default").WithL1CacheSize(192*1024*nvidia.BYTE).
		WithSMUnitNum(4).WithSMUnitStrategy("default").WithL0CacheSize(16*1024*nvidia.BYTE).
		WithRegisterFileSize(256*1024*nvidia.BYTE).WithLaneSize(4*nvidia.BYTE).
		WithALU("int32", 16)
	gpu.Build()
	return gpu
}

func main() {
	args := getInputArguments()
	gpu := buildAmpereGPU()
	benchmark := benchmark.NewBenchMark().WithTraceDirPath(args.inputTraceDir)
	benchmark.Build()
	benchmark.Exec(gpu)
}
