package main

import (
	"flag"
	"log"
	"math"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/driver"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/platform"
	"gitlab.com/yaotsu/mem"
)

type FirKernelArgs struct {
	Output              driver.GPUPtr
	Filter              driver.GPUPtr
	Input               driver.GPUPtr
	History             driver.GPUPtr
	NumTaps             uint32
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

	dataSize     int
	numTaps      int
	inputData    []float32
	filterData   []float32
	gFilterData  driver.GPUPtr
	gHistoryData driver.GPUPtr
	gInputData   driver.GPUPtr
	gOutputData  driver.GPUPtr
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
var numData = flag.Int("dataSize", 4096, "The number of samples to filter.")

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

	dataSize = *numData
}

func initPlatform() {
	if *timing {
		engine, gpu, gpuDriver, globalMem = platform.BuildR9NanoPlatform()
	} else {
		engine, gpu, gpuDriver, globalMem = platform.BuildEmuPlatform()
	}
}

func loadProgram() {
	hsaco = kernels.LoadProgram(*kernelFilePath, "FIR")
}

func initMem() {
	numTaps = 16
	gFilterData = gpuDriver.AllocateMemory(globalMem.Storage, uint64(numTaps*4))
	gHistoryData = gpuDriver.AllocateMemory(globalMem.Storage, uint64(numTaps*4))
	gInputData = gpuDriver.AllocateMemory(globalMem.Storage, uint64(dataSize*4))
	gOutputData = gpuDriver.AllocateMemory(globalMem.Storage, uint64(dataSize*4))

	filterData = make([]float32, numTaps)
	for i := 0; i < numTaps; i++ {
		filterData[i] = float32(i)
	}

	inputData = make([]float32, dataSize)
	for i := 0; i < dataSize; i++ {
		inputData[i] = float32(i)
	}

	gpuDriver.MemoryCopyHostToDevice(gFilterData, filterData, globalMem.Storage)
	gpuDriver.MemoryCopyHostToDevice(gInputData, inputData, globalMem.Storage)
}

func run() {
	kernArg := FirKernelArgs{
		gOutputData,
		gFilterData,
		gInputData,
		gHistoryData,
		uint32(numTaps),
		0, 0, 0,
	}

	gpuDriver.LaunchKernel(hsaco, gpu, globalMem.Storage,
		[3]uint32{uint32(dataSize), 1, 1},
		[3]uint16{256, 1, 1},
		&kernArg,
	)
}

func checkResult() {
	gpuOutput := make([]float32, dataSize)
	gpuDriver.MemoryCopyDeviceToHost(gpuOutput, gOutputData, globalMem.Storage)

	for i := 0; i < dataSize; i++ {
		var sum float32
		sum = 0

		for j := 0; j < numTaps; j++ {
			if i < j {
				continue
			}
			sum += inputData[i-j] * filterData[j]
		}

		if math.Abs(float64(sum-gpuOutput[i])) >= 1e-5 {
			log.Fatalf("At position %d, expected %f, but get %f.\n",
				i, sum, gpuOutput[i])
		}
	}

	log.Printf("Passed!\n")
}
