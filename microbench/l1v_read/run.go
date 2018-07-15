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
	Data                driver.GPUPtr
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

var (
	engine    core.Engine
	globalMem *mem.IdealMemController
	gpu       *gcn3.GPU
	gpuDriver *driver.Driver
	hsaco     *insts.HsaCo

	gData     driver.GPUPtr
	numRepeat int
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

func configure() {
	flag.Parse()

	if *isaDebug {
		platform.DebugISA = true
	}

	if *instTracing {
		platform.TraceInst = true
	}
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
	gData = gpuDriver.AllocateMemory(globalMem.Storage, uint64(64))
}

func run() {
	kernArg := KernelArgs{
		gData,
		0, 0, 0,
	}

	gpuDriver.LaunchKernel(hsaco, gpu.ToDriver, globalMem.Storage,
		[3]uint32{64, 1, 1},
		[3]uint16{64, 1, 1},
		&kernArg,
	)
}
