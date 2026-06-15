// Package simpleconvolution implements the Simple Convolution benchmark from
// AMDAPPSDK.
package simpleconvolution

import (
	"log"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/arch"
	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// GCN3KernelArgs defines kernel arguments for GCN3 architecture
type GCN3KernelArgs struct {
	Input                           driver.Ptr
	Mask                            driver.Ptr
	Output                          driver.Ptr
	InputDimensions, MaskDimensions [2]uint32
	NExWidth                        uint32
	Padding                         uint32
	OffsetX, OffsetY, OffsetZ       uint64
}

// CDNA3KernelArgs defines kernel arguments for CDNA3 architecture (GFX942)
type CDNA3KernelArgs struct {
	Input           driver.Ptr // offset 0
	Mask            driver.Ptr // offset 8
	Output          driver.Ptr // offset 16
	InputDimensions [2]uint32  // offset 24 (inputWidth, inputHeight)
	MaskDimensions  [2]uint32  // offset 32 (maskWidth, maskHeight)
	NExWidth        uint32     // offset 40
	Pad             uint32     // offset 44 - alignment padding
	// Hidden kernel arguments (required by HIP runtime for GFX942)
	HiddenBlockCountX   uint32   // offset 48
	HiddenBlockCountY   uint32   // offset 52
	HiddenBlockCountZ   uint32   // offset 56
	HiddenGroupSizeX    uint16   // offset 60
	HiddenGroupSizeY    uint16   // offset 62
	HiddenGroupSizeZ    uint16   // offset 64
	HiddenRemainderX    uint16   // offset 66
	HiddenRemainderY    uint16   // offset 68
	HiddenRemainderZ    uint16   // offset 70
	Padding             [16]byte // offset 72-87 - reserved
	HiddenGlobalOffsetX int64    // offset 88
	HiddenGlobalOffsetY int64    // offset 96
	HiddenGlobalOffsetZ int64    // offset 104
	HiddenGridDims      uint16   // offset 112
}

// Benchmark defines a benchmark
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	kernel  *insts.KernelCodeObject
	gpus    []int

	Arch      arch.Type
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
var gcn3HSACOBytes []byte

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

func (b *Benchmark) loadProgram() {
	var hsacoBytes []byte
	if b.Arch == arch.CDNA3 {
		hsacoBytes = cdna3HSACOBytes
	} else {
		hsacoBytes = gcn3HSACOBytes
	}
	b.kernel = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "simpleNonSeparableConvolution")
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
	b.loadProgram()

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
			uint64(numOutputData*4))
	} else {
		b.dInputData = b.driver.AllocateMemory(b.context,
			uint64(numInputData*4))
		b.driver.Distribute(b.context, b.dInputData,
			uint64(numInputData*4), b.gpus)
		b.dOutputData = b.driver.AllocateMemory(b.context,
			uint64(numOutputData*4))
		b.driver.Distribute(b.context, b.dOutputData,
			uint64(numOutputData*4), b.gpus)
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

func (b *Benchmark) enqueueKernel(queue *driver.CommandQueue, gpuIndex int, gridSize uint32) {
	if b.Arch == arch.CDNA3 {
		wgSizeX := uint16(64)
		wgSizeY := uint16(1)
		wgSizeZ := uint16(1)

		kernArg := CDNA3KernelArgs{
			Input:           b.dInputData,
			Mask:            b.dMasks[gpuIndex],
			Output:          b.dOutputData,
			InputDimensions: [2]uint32{b.Width, b.Height},
			MaskDimensions:  [2]uint32{b.maskSize, b.maskSize},
			NExWidth:        b.Width + b.padWidth,
			// Hidden kernel arguments for GFX942
			HiddenBlockCountX:   gridSize / uint32(wgSizeX),
			HiddenBlockCountY:   1,
			HiddenBlockCountZ:   1,
			HiddenGroupSizeX:    wgSizeX,
			HiddenGroupSizeY:    wgSizeY,
			HiddenGroupSizeZ:    wgSizeZ,
			HiddenRemainderX:    uint16(gridSize % uint32(wgSizeX)),
			HiddenRemainderY:    0,
			HiddenRemainderZ:    0,
			HiddenGlobalOffsetX: int64(gridSize * uint32(gpuIndex)),
			HiddenGlobalOffsetY: 0,
			HiddenGlobalOffsetZ: 0,
			HiddenGridDims:      1,
		}

		b.driver.EnqueueLaunchKernel(
			queue,
			b.kernel,
			[3]uint32{gridSize, 1, 1},
			[3]uint16{wgSizeX, 1, 1},
			&kernArg,
		)
	} else {
		kernArg := GCN3KernelArgs{
			b.dInputData,
			b.dMasks[gpuIndex],
			b.dOutputData,
			[2]uint32{b.Width, b.Height},
			[2]uint32{b.maskSize, b.maskSize},
			b.Width + b.padWidth,
			0,
			uint64(gridSize * uint32(gpuIndex)), 0, 0,
		}

		b.driver.EnqueueLaunchKernel(
			queue,
			b.kernel,
			[3]uint32{gridSize, 1, 1},
			[3]uint16{uint16(64), 1, 1},
			&kernArg,
		)
	}
}

func (b *Benchmark) exec() {
	queues := make([]*driver.CommandQueue, len(b.gpus))
	gridSize := ((b.Width + b.padWidth) * (b.Height + b.padHeight)) / uint32(len(b.gpus))

	for i, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		queues[i] = b.driver.CreateCommandQueue(b.context)
		b.enqueueKernel(queues[i], i, gridSize)
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
