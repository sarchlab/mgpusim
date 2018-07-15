package main

import (
	"flag"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/driver"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/platform"
	"gitlab.com/yaotsu/mem"
)

type KernelArgs struct {
	Data driver.GPUPtr
}

var (
	engine    core.Engine
	globalMem *mem.IdealMemController
	gpu       *gcn3.GPU
	gpuDriver *driver.Driver
	hsaco     *insts.HsaCo

	numWfPerWG int
	numWG      int
)

func main() {
	configure()
	initPlatform()
	loadProgram()
	initMem()
	run()
}

var isaDebug = flag.Bool("debug-isa", false, "Generate the ISA debugging file.")
var timing = flag.Bool("timing", false, "Run detailed timing simulation.")
var instTracing = flag.Bool("trace-inst", false,
	"Generate instruction trace for visualization purposes.")
var numWfPerWGFlag = flag.Int("numWfPerWG", 1, "The number of repeat read.")
var numWGFlag = flag.Int("numWG", 128, "The number of repeat read.")

func configure() {
	flag.Parse()

	if *isaDebug {
		platform.DebugISA = true
	}

	if *instTracing {
		platform.TraceInst = true
	}

	numWG = *numWGFlag
	numWfPerWG = *numWfPerWGFlag
}

func initPlatform() {
	if *timing {
		engine, gpu, gpuDriver, globalMem = platform.BuildR9NanoPlatform()
	} else {
		engine, gpu, gpuDriver, globalMem = platform.BuildEmuPlatform()
	}
}

func loadProgram() {
	hsaco = kernels.LoadProgram("microbench/kernels.hsaco", "")
}

func initMem() {
}

func run() {
	kernArg := new(KernelArgs)
	gpuDriver.LaunchKernel(
		hsaco, gpu.ToDriver, globalMem.Storage,
		[3]uint32{64 * uint32(numWfPerWG) * uint32(numWG), 1, 1},
		[3]uint16{64 * uint16(numWfPerWG), 1, 1},
		kernArg,
	)
}
