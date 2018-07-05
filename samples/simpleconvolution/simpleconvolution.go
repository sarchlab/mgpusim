package main

import (
	"flag"
	"log"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/driver"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/platform"
	"gitlab.com/yaotsu/mem"
)

type KernelArgs struct {
	Input                           driver.GPUPtr
	Mask                            driver.GPUPtr
	Output                          driver.GPUPtr
	InputDimensions, MaskDimensions [2]uint32
	NExWidth                        uint32
	Padding                         uint32
	OffsetX, OffsetY, OffsetZ       uint64
}

var (
	engine    core.Engine
	globalMem *mem.IdealMemController
	storage   *mem.Storage
	gpu       *gcn3.GPU
	gpuDriver *driver.Driver
	kernel    *insts.HsaCo

	width     uint32
	height    uint32
	padWidth  uint32
	padHeight uint32
	maskSize  uint32

	hInputData  []uint32
	hOutputData []uint32
	hMask       []float32
	dInputData  driver.GPUPtr
	dOutputData driver.GPUPtr
	dMask       driver.GPUPtr
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
var widthFlag = flag.Uint("width", 254, "The width of the input matrix.")
var heightFlag = flag.Uint("height", 254, "The height of the input matrix.")
var maskSizeFlag = flag.Uint("mask-size", 3, "The size of the mask.")

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

	width = uint32(*widthFlag)
	height = uint32(*heightFlag)
	maskSize = uint32(*maskSizeFlag)
	padWidth = maskSize - 1
	padHeight = maskSize - 1
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
	kernel = kernels.LoadProgram(*kernelFilePath, "simpleNonSeparableConvolution")
	if kernel == nil {
		log.Fatal("Error loading kernel")
	}
}

func initMem() {
	numInputData := (width + padWidth) * (height + padHeight)
	numOutputData := width * height

	hInputData = make([]uint32, numInputData)
	hOutputData = make([]uint32, numOutputData)
	hMask = make([]float32, maskSize*maskSize)

	for i := uint32(0); i < numInputData; i++ {
		hInputData[i] = uint32(i)
	}

	for i := uint32(0); i < maskSize*maskSize; i++ {
		hMask[i] = float32(i)
	}

	dInputData = gpuDriver.AllocateMemory(storage, uint64(numInputData*4))
	dOutputData = gpuDriver.AllocateMemory(storage, uint64(numInputData*4))
	dMask = gpuDriver.AllocateMemory(storage, uint64(maskSize*maskSize*4))

	gpuDriver.MemoryCopyHostToDevice(dInputData, hInputData, gpu.ToDriver)
	gpuDriver.MemoryCopyHostToDevice(dOutputData, hOutputData, gpu.ToDriver)
	gpuDriver.MemoryCopyHostToDevice(dMask, hMask, gpu.ToDriver)
}

func run() {
	kernArg := KernelArgs{
		dInputData,
		dMask,
		dOutputData,
		[2]uint32{width, height},
		[2]uint32{maskSize, maskSize},
		width + padWidth,
		0, 0, 0, 0,
	}

	gridSize := (width + padWidth) * (height + padHeight)
	gpuDriver.LaunchKernel(kernel, gpu.ToDriver, globalMem.Storage,
		[3]uint32{uint32(gridSize), 1, 1},
		[3]uint16{uint16(64), 1, 1},
		&kernArg,
	)
}

func checkResult() {
	cpuOutputImage := cpuSimpleConvolution()

	gpuDriver.MemoryCopyDeviceToHost(hOutputData, dOutputData, gpu.ToDriver)
	for i := uint32(0); i < height; i++ {
		for j := uint32(0); j < width; j++ {
			index := i*width + j
			gpuOutput := hOutputData[index]
			cpuOutput := cpuOutputImage[index]

			if cpuOutput != gpuOutput {
				log.Panicf("mismatch as position %d, %d. Expected %d, but get %d",
					i, j, cpuOutput, gpuOutput)
			}
		}
	}

	log.Printf("Passed!\n")
}

func cpuSimpleConvolution() []uint32 {
	numOutputData := (width + padWidth) * (height + padHeight)
	cpuOutputData := make([]uint32, numOutputData)

	for y := uint32(0); y < height+padHeight; y++ {
		for x := uint32(0); x < width+padWidth; x++ {
			outputIndex := y*width + x
			if x >= width || y >= height {
				break
			}

			sum := float32(0)
			for j := uint32(0); j < maskSize; j++ {
				for i := uint32(0); i < maskSize; i++ {
					maskIndex := j*maskSize + i
					imageIndex := (y+j)*(width+padWidth) + (x + i)

					sum += float32(hInputData[imageIndex]) * hMask[maskIndex]
				}
			}

			sum += 0.5
			cpuOutputData[outputIndex] = uint32(sum)
		}
	}

	return cpuOutputData
}
