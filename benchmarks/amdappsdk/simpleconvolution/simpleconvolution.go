package simpleconvolution

import (
	"log"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
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

type Benchmark struct {
	driver *driver.Driver
	kernel *insts.HsaCo

	numGPUs   int
	gpuQueues []*driver.CommandQueue

	Width     uint32
	Height    uint32
	maskSize  uint32
	padWidth  uint32
	padHeight uint32

	hInputData  []uint32
	hOutputData []uint32
	hMask       []float32
	dInputData  driver.GPUPtr
	dOutputData driver.GPUPtr
	dMask       []driver.GPUPtr
}

func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.numGPUs = driver.GetNumGPUs()
	for i := 0; i < b.numGPUs; i++ {
		b.driver.SelectGPU(i)
		b.gpuQueues = append(b.gpuQueues, b.driver.CreateCommandQueue())
	}
	b.loadProgram()
	return b
}

func (b *Benchmark) loadProgram() {
	hsacoBytes, err := Asset("kernels.hsaco")
	if err != nil {
		log.Panic(err)
	}

	b.kernel = kernels.LoadProgramFromMemory(hsacoBytes, "simpleNonSeparableConvolution")
	if b.kernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

func (b *Benchmark) SetMaskSize(maskSize uint32) {
	b.maskSize = maskSize
	b.padHeight = maskSize - 1
	b.padWidth = maskSize - 1
}

func (b *Benchmark) Run() {
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	numInputData := (b.Width + b.padWidth) * (b.Height + b.padHeight)
	numOutputData := b.Width * b.Height

	b.hInputData = make([]uint32, numInputData)
	b.hOutputData = make([]uint32, numOutputData)
	b.hMask = make([]float32, b.maskSize*b.maskSize)

	for i := uint32(0); i < numInputData; i++ {
		b.hInputData[i] = uint32(i)
	}

	for i := uint32(0); i < b.maskSize*b.maskSize; i++ {
		b.hMask[i] = float32(i)
	}

	b.dInputData = b.driver.AllocateMemory(uint64(numInputData * 4))
	b.dOutputData = b.driver.AllocateMemory(uint64(numInputData * 4))
	for i := 0; i < b.numGPUs; i++ {
		b.driver.SelectGPU(i)
		b.dMask = append(b.dMask,
			b.driver.AllocateMemory(uint64(b.maskSize*b.maskSize*4)))
		b.driver.EnqueueMemCopyH2D(b.gpuQueues[i],
			b.dMask[i], b.hMask)

		inputBytePerGPU := uint64(numInputData*4) / uint64(b.numGPUs)
		inputPtr := uint64(b.dInputData) + uint64(i)*inputBytePerGPU
		b.driver.Remap(inputPtr, inputBytePerGPU, i)
		b.driver.EnqueueMemCopyH2D(b.gpuQueues[i], driver.GPUPtr(inputPtr),
			b.hInputData[int(numInputData)/b.numGPUs*i:int(numInputData)/b.numGPUs*(i+1)])
	}

	b.distributeOutputToGPUs()

	b.driver.ExecuteAllCommands()
}

func (b *Benchmark) distributeOutputToGPUs() {
	minPageSize := uint64(64)
	numOutputData := b.Width * b.Height
	//sizeLeft := numOutputData * 4
	outputBytePerGPU := uint64(numOutputData*4) / uint64(b.numGPUs)
	outputBytePerGPU = ((outputBytePerGPU-1)/minPageSize + 1) * minPageSize
	for i := 0; i < b.numGPUs; i++ {
		outputPtr := uint64(b.dOutputData) + uint64(i)*outputBytePerGPU
		b.driver.Remap(outputPtr, outputBytePerGPU, i)
	}
}

func (b *Benchmark) collectOutputFromGPUs() {
	minPageSize := uint64(64)
	numOutputData := b.Width * b.Height
	//sizeLeft := numOutputData * 4
	outputBytePerGPU := uint64(numOutputData*4) / uint64(b.numGPUs)
	outputBytePerGPU = ((outputBytePerGPU-1)/minPageSize + 1) * minPageSize
	numPixelPerGPU := outputBytePerGPU / 4
	pixelOffset := uint64(0)
	pixelLeft := uint64(numOutputData)
	for i := uint64(0); i < uint64(b.numGPUs); i++ {
		outputPtr := uint64(b.dOutputData) + i*outputBytePerGPU
		numPixel := numPixelPerGPU
		if numPixel > pixelLeft {
			numPixel = pixelLeft
		}
		b.driver.EnqueueMemCopyD2H(
			b.gpuQueues[i],
			b.hOutputData[pixelOffset:pixelOffset+numPixel],
			driver.GPUPtr(outputPtr),
		)
		pixelOffset += numPixel
		pixelLeft -= numPixel
	}
	b.driver.ExecuteAllCommands()
}

func (b *Benchmark) exec() {
	for i := 0; i < b.numGPUs; i++ {
		gridSize := (b.Width + b.padWidth) * (b.Height + b.padHeight)
		kernArg := KernelArgs{
			b.dInputData,
			b.dMask[i],
			b.dOutputData,
			[2]uint32{b.Width, b.Height},
			[2]uint32{b.maskSize, b.maskSize},
			b.Width + b.padWidth,
			0,
			uint64(i * int(gridSize) / b.numGPUs), 0, 0,
		}

		b.driver.EnqueueLaunchKernel(
			b.gpuQueues[i],
			b.kernel,
			[3]uint32{uint32(gridSize) / uint32(b.numGPUs), 1, 1},
			[3]uint16{uint16(64), 1, 1},
			&kernArg,
		)
	}

	b.driver.ExecuteAllCommands()

	b.collectOutputFromGPUs()

}

func (b *Benchmark) Verify() {
	cpuOutputImage := b.cpuSimpleConvolution()

	for i := uint32(0); i < b.Height; i++ {
		for j := uint32(0); j < b.Width; j++ {
			index := i*b.Width + j
			gpuOutput := b.hOutputData[index]
			cpuOutput := cpuOutputImage[index]

			if cpuOutput != gpuOutput {
				log.Panicf("mismatch as position %d, %d. Expected %d, but get %d",
					i, j, cpuOutput, gpuOutput)
			}
		}
	}

	log.Printf("Passed!\n")
}

func (b *Benchmark) cpuSimpleConvolution() []uint32 {
	numOutputData := (b.Width + b.padWidth) * (b.Height + b.padHeight)
	cpuOutputData := make([]uint32, numOutputData)

	for y := uint32(0); y < b.Height+b.padHeight; y++ {
		for x := uint32(0); x < b.Width+b.padWidth; x++ {
			outputIndex := y*b.Width + x
			if x >= b.Width || y >= b.Height {
				break
			}

			sum := float32(0)
			for j := uint32(0); j < b.maskSize; j++ {
				for i := uint32(0); i < b.maskSize; i++ {
					maskIndex := j*b.maskSize + i
					imageIndex := (y+j)*(b.Width+b.padWidth) + (x + i)

					sum += float32(b.hInputData[imageIndex]) * b.hMask[maskIndex]
				}
			}

			sum += 0.5
			cpuOutputData[outputIndex] = uint32(sum)
		}
	}

	return cpuOutputData
}
