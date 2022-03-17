// Package simpleconvolution implements the Simple Convolution benchmark from
// AMDAPPSDK.
package simpleconvolution

import (
	"log"

	// embed hsaco files
	_ "embed"

	"gitlab.com/akita/mgpusim/v3/driver"
	"gitlab.com/akita/mgpusim/v3/insts"
	"gitlab.com/akita/mgpusim/v3/kernels"
)

// KernelArgs defines kernel arguments
type KernelArgs struct {
	Input                           driver.Ptr
	Mask                            driver.Ptr
	Output                          driver.Ptr
	InputDimensions, MaskDimensions [2]uint32
	NExWidth                        uint32
	Padding                         uint32
	OffsetX, OffsetY, OffsetZ       uint64
}

// Benchmark defines a benchmark
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	kernel  *insts.HsaCo
	gpus    []int

	Width     uint32
	Height    uint32
	maskSize  uint32
	padWidth  uint32
	padHeight uint32

	hInputData  []uint32
	hOutputData []uint32
	hMask       []float32
	dInputData  driver.Ptr
	dOutputData driver.Ptr
	dMasks      []driver.Ptr

	useUnifiedMemory bool
}

// NewBenchmark returns a benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.loadProgram()
	return b
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

//go:embed kernels.hsaco
var hsacoBytes []byte

func (b *Benchmark) loadProgram() {
	b.kernel = kernels.LoadProgramFromMemory(hsacoBytes, "simpleNonSeparableConvolution")
	if b.kernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SetMaskSize sets masksize
func (b *Benchmark) SetMaskSize(maskSize uint32) {
	b.maskSize = maskSize
	b.padHeight = maskSize - 1
	b.padWidth = maskSize - 1
}

// Run runs
func (b *Benchmark) Run() {
	b.driver.SelectGPU(b.context, b.gpus[0])
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
		// b.hInputData[i] = i
		b.hInputData[i] = 1
	}

	for i := uint32(0); i < b.maskSize*b.maskSize; i++ {
		// b.hMask[i] = float32(i)
		b.hMask[i] = float32(1)
	}

	if b.useUnifiedMemory {
		b.dInputData = b.driver.AllocateUnifiedMemory(b.context,
			uint64(numInputData*4))
		b.dOutputData = b.driver.AllocateUnifiedMemory(b.context,
			uint64(numInputData*4))
	} else {
		b.dInputData = b.driver.AllocateMemory(b.context,
			uint64(numInputData*4))
		b.driver.Distribute(b.context, b.dInputData,
			uint64(numInputData*4), b.gpus)
		b.dOutputData = b.driver.AllocateMemory(b.context,
			uint64(numInputData*4))
		b.driver.Distribute(b.context, b.dInputData,
			uint64(numInputData*4), b.gpus)
	}

	b.dMasks = make([]driver.Ptr, len(b.gpus))
	for i, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		if b.useUnifiedMemory {
			b.dMasks[i] = b.driver.AllocateUnifiedMemory(
				b.context,
				uint64(b.maskSize*b.maskSize*4))
		} else {
			b.dMasks[i] = b.driver.AllocateMemory(
				b.context,
				uint64(b.maskSize*b.maskSize*4))
		}
		b.driver.MemCopyH2D(b.context, b.dMasks[i], b.hMask)
	}

	b.driver.MemCopyH2D(b.context, b.dInputData, b.hInputData)
	b.driver.MemCopyH2D(b.context, b.dOutputData, b.hOutputData)
}

func (b *Benchmark) exec() {
	queues := make([]*driver.CommandQueue, len(b.gpus))
	for i, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		queues[i] = b.driver.CreateCommandQueue(b.context)

		gridSize := ((b.Width + b.padWidth) * (b.Height + b.padHeight)) /
			uint32(len(b.gpus))

		kernArg := KernelArgs{
			b.dInputData,
			b.dMasks[i],
			b.dOutputData,
			[2]uint32{b.Width, b.Height},
			[2]uint32{b.maskSize, b.maskSize},
			b.Width + b.padWidth,
			0,
			uint64(gridSize * uint32(i)), 0, 0,
		}

		b.driver.EnqueueLaunchKernel(
			queues[i],
			b.kernel,
			[3]uint32{gridSize, 1, 1},
			[3]uint16{uint16(64), 1, 1},
			&kernArg,
		)
	}

	for _, q := range queues {
		b.driver.DrainCommandQueue(q)
	}

	b.driver.MemCopyD2H(b.context, b.hOutputData, b.dOutputData)
}

// Verify verifies
func (b *Benchmark) Verify() {
	cpuOutputImage := b.cpuSimpleConvolution()

	mismatch := false
	for i := uint32(0); i < b.Height; i++ {
		for j := uint32(0); j < b.Width; j++ {
			index := i*b.Width + j
			gpuOutput := b.hOutputData[index]
			cpuOutput := cpuOutputImage[index]

			if cpuOutput != gpuOutput {
				log.Printf("mismatch as position %d, %d (addr 0x%x). "+
					"Expected %d, but get %d",
					i, j,
					uint64(b.dOutputData)+uint64(4*index),
					cpuOutput, gpuOutput)
				mismatch = true
			}
		}
	}

	if mismatch {
		panic("verification failed")
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
