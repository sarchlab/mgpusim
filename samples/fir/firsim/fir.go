package main

import (
	"flag"
	"log"
	_ "net/http/pprof"
	"os"

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
	output              driver.GPUPtr
	filter              driver.GPUPtr
	input               driver.GPUPtr
	history             driver.GPUPtr
	numTaps             uint32
	hiddenGlobalOffsetX int64
	hiddenGlobalOffsetY int64
	hiddenGlobalOffsetZ int64
}

var (
	engine      core.Engine
	globalMem   *mem.IdealMemController
	gpu         *gcn3.GPU
	gpuDriver   *driver.Driver
	connection  core.Connection
	hsaco       *insts.HsaCo
	logger      *log.Logger
	traceOutput *os.File

	dataSize     int
	numTaps      int
	inputData    []float32
	filterData   []float32
	gFilterData  driver.GPUPtr
	gHistoryData driver.GPUPtr
	gInputData   driver.GPUPtr
	gOutputData  driver.GPUPtr
)

var kernelFilePath = flag.String("kernel file path", "../disasm/kernels.hsaco", "the kernel hsaco file")

func main() {
	flag.Parse()

	initPlatform()
	loadProgram()
	initMem()
	run()
	//checkResult()
}

func initPlatform() {
	engine, gpu, gpuDriver, globalMem = platform.BuildR9NanoPlatform()
}

func loadProgram() {
	hsaco = kernels.LoadProgram(*kernelFilePath, "FIR")
}

func initMem() {
	dataSize = 8192
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
