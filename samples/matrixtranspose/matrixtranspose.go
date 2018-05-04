package main

import (
	"flag"
	"log"

	"fmt"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/driver"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/platform"
	"gitlab.com/yaotsu/mem"
)

type MatrixTransposeKernelArgs struct {
	output              driver.GPUPtr
	input               driver.GPUPtr
	block, padding      uint32
	hiddenGlobalOffsetX int64
	hiddenGlobalOffsetY int64
	hiddenGlobalOffsetZ int64
}

var (
	engine    core.Engine
	globalMem *mem.IdealMemController
	storage   *mem.Storage
	gpu       *gcn3.GPU
	gpuDriver *driver.Driver
	kernel    *insts.HsaCo

	width       int
	height      int
	numTaps     int
	hInputData  []float32
	hOutputData []float32
	dInputData  driver.GPUPtr
	dOutputData driver.GPUPtr
)

var kernelFilePath = flag.String(
	"kernel file path",
	"kernels.hsaco",
	"The path to the kernel hsaco file.",
)
var timing = flag.Bool("timing", false, "Run detailed timing simulation.")
var parallel = flag.Bool("parallel", false, "Run the simulation in parallel.")
var isaDebug = flag.Bool("debug-isa", false, "Generate the ISA debugging file.")
var instTracing = flag.Bool("trace-inst", false, "Generate instruction trace for visualization purposes.")
var verify = flag.Bool("verify", false, "Verify the emulation result.")
var dataWidth = flag.Int("width", 256, "The number of columns in the input matrix.")
var dataHeight = flag.Int("height", 256, "The number of rows in the input matrix.")

func main() {
	configure()
	initPlatform()
	loadProgram()
	initMem()
	run()

	if *verify {
		checkResult()
	}
}

func configure() {
	flag.Parse()

	if *parallel {
		platform.UseParallelEngine = true
	}

	if *isaDebug {
		platform.DebugISA = true
	}

	if *instTracing {
		platform.TraceInst = true
	}

	width = *dataWidth
	height = *dataHeight
}

func initPlatform() {
	if *timing {
		engine, gpu, gpuDriver, globalMem = platform.BuildR9NanoPlatform()
	} else {
		engine, gpu, gpuDriver, globalMem = platform.BuildEmuPlatform()
	}
	storage = globalMem.Storage
}

func loadProgram() {
	kernel = kernels.LoadProgram(*kernelFilePath, "matrixTranspose")
	if kernel == nil {
		log.Fatal("Error loading kernel")
	}
}

func initMem() {
	numData := width * height

	hInputData = make([]float32, numData)
	hOutputData = make([]float32, numData)

	for i := 0; i < numData; i++ {
		hInputData[i] = float32(i)
	}

	dInputData = gpuDriver.AllocateMemory(storage, uint64(numData*4))
	dOutputData = gpuDriver.AllocateMemory(storage, uint64(numData*4))

	gpuDriver.MemoryCopyHostToDevice(dInputData, hInputData, storage)
}

func run() {
	kernArg := MatrixTransposeKernelArgs{
		dOutputData,
		dInputData,
		0,
		0,
		0, 0, 0,
	}

	gpuDriver.LaunchKernel(kernel, gpu, globalMem.Storage,
		[3]uint32{uint32(width / 4), uint32(height / 4), 1},
		[3]uint16{16, 16, 1},
		&kernArg,
	)
}

func checkResult() {
	gpuDriver.MemoryCopyDeviceToHost(hOutputData, dOutputData, storage)

	for i := 0; i < width; i++ {
		for j := 0; j < height; j++ {
			fmt.Printf("%f ", hOutputData[i*width+j])
		}
		fmt.Printf("\n")
	}

	log.Printf("Passed!\n")
}
