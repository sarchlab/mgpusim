package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/driver"
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

func main() {
	args := getInputArguments()

	benchmarkBuilder := driver.NewBenchmarkBuilder().WithTraceDirPath(args.inputTraceDir)
	myBenchmark, _ := benchmarkBuilder.Build()

	myGPU := buildAmpereGPU()

	driverBuilder := driver.NewDriverBuilder().WithGPU(myGPU)
	myDriver, _ := driverBuilder.Build()
	myDriver.Exec(myBenchmark)
}

func buildAmpereGPU() *gpu.GPU {
	gpuBuilder := gpu.NewGPUBuilder().
		WithGPCCnt(8).
		WithSMCnt(16).
		WithSMUnitCnt(4).
		WithGPUStrategy("round-robin").
		WithSMStrategy("round-robin").
		WithL2CacheConfig(4*1024*1024*nvidia.BYTE).
		WithL1CacheConfig(192*1024*nvidia.BYTE).
		WithL0CacheConfig(16*1024*nvidia.BYTE).
		WithRegisterFileConfig(256*1024*nvidia.BYTE, 4*nvidia.BYTE).
		WithALUConfig("int32", 16)
	myGPU, _ := gpuBuilder.Build()
	return myGPU
}
