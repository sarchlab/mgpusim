// Package matrixtranspose implements the matrix transpose benchmark from
// AMDAPPSDK.
package matrixtranspose

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
	Output              driver.Ptr
	Input               driver.Ptr
	Block               driver.LocalPtr
	WIWidth             uint32
	WIHeight            uint32
	NumWGWidth          uint32
	GroupXOffset        uint32
	GroupYOffset        uint32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// CDNA3KernelArgs defines kernel arguments for CDNA3 architecture (GFX942)
type CDNA3KernelArgs struct {
	Output       driver.Ptr      // offset 0
	Input        driver.Ptr      // offset 8
	Block        driver.LocalPtr // offset 16 (LDS allocation for HIP_DYNAMIC_SHARED)
	BlockPad     uint32          // offset 20 (pad to match 8-byte ptr in HSACO)
	WIWidth      uint32          // offset 24
	WIHeight     uint32          // offset 28
	NumWGWidth   uint32          // offset 32
	GroupXOffset uint32          // offset 36
	GroupYOffset uint32          // offset 40
	Pad          uint32          // offset 44 - alignment padding
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
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.KernelCodeObject

	Arch               arch.Type
	Width              int
	elemsPerThread1Dim int
	blockSize          int

	hInputData  []uint32
	hOutputData []uint32
	dInputData  driver.Ptr
	dOutputData driver.Ptr

	useUnifiedMemory bool
}

// NewBenchmark makes a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.elemsPerThread1Dim = 4
	b.blockSize = 16
	return b
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory use Unified Memory
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
	b.kernel = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "matrixTranspose")
	if b.kernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// Run runs
func (b *Benchmark) Run() {
	b.loadProgram()

	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues, b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	numData := b.Width * b.Width

	b.hInputData = make([]uint32, numData)
	b.hOutputData = make([]uint32, numData)

	for i := 0; i < numData; i++ {
		b.hInputData[i] = uint32(i)
	}

	if b.useUnifiedMemory {
		b.dInputData = b.driver.AllocateUnifiedMemory(
			b.context, uint64(numData*4))
		b.dOutputData = b.driver.AllocateUnifiedMemory(
			b.context, uint64(numData*4))
	} else {
		b.dInputData = b.driver.AllocateMemory(
			b.context, uint64(numData*4))
		b.dOutputData = b.driver.AllocateMemory(
			b.context, uint64(numData*4))
		b.driver.Distribute(b.context, b.dInputData, uint64(numData*4), b.gpus)
		b.driver.Distribute(b.context, b.dOutputData, uint64(numData*4), b.gpus)
	}

	b.driver.MemCopyH2D(b.context, b.dInputData, b.hInputData)
}

func (b *Benchmark) createCDNA3KernelArgs(
	blockPtr driver.LocalPtr,
	wiWidth, wiHeight, numWGWidth, wgXPerGPU uint32,
	wiWidthPerGPU, gpuIndex int,
) CDNA3KernelArgs {
	wgSizeX := uint16(b.blockSize)
	wgSizeY := uint16(b.blockSize)
	wgSizeZ := uint16(1)
	gridSizeX := uint32(wiWidthPerGPU)
	gridSizeY := wiHeight

	return CDNA3KernelArgs{
		Output:              b.dOutputData,
		Input:               b.dInputData,
		Block:               blockPtr,
		WIWidth:             wiWidth,
		WIHeight:            wiHeight,
		NumWGWidth:          numWGWidth,
		GroupXOffset:        wgXPerGPU * uint32(gpuIndex),
		GroupYOffset:        0,
		HiddenBlockCountX:   gridSizeX / uint32(wgSizeX),
		HiddenBlockCountY:   gridSizeY / uint32(wgSizeY),
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    wgSizeX,
		HiddenGroupSizeY:    wgSizeY,
		HiddenGroupSizeZ:    wgSizeZ,
		HiddenRemainderX:    uint16(gridSizeX % uint32(wgSizeX)),
		HiddenRemainderY:    uint16(gridSizeY % uint32(wgSizeY)),
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      2,
	}
}

func (b *Benchmark) enqueueKernel(
	queue *driver.CommandQueue,
	wiWidth, wiHeight, numWGWidth, wgXPerGPU uint32,
	wiWidthPerGPU int,
	gpuIndex int,
) {
	blockPtr := driver.LocalPtr(b.blockSize * b.blockSize *
		b.elemsPerThread1Dim * b.elemsPerThread1Dim * 4)

	if b.Arch == arch.CDNA3 {
		kernArg := b.createCDNA3KernelArgs(blockPtr, wiWidth, wiHeight, numWGWidth, wgXPerGPU, wiWidthPerGPU, gpuIndex)
		wgSizeX := uint16(b.blockSize)
		wgSizeY := uint16(b.blockSize)
		gridSizeX := uint32(wiWidthPerGPU)
		gridSizeY := wiHeight

		b.driver.EnqueueLaunchKernel(
			queue,
			b.kernel,
			[3]uint32{gridSizeX, gridSizeY, 1},
			[3]uint16{wgSizeX, wgSizeY, 1},
			&kernArg,
		)
	} else {
		kernArg := GCN3KernelArgs{
			b.dOutputData,
			b.dInputData,
			blockPtr,
			wiWidth, wiHeight, numWGWidth,
			wgXPerGPU * uint32(gpuIndex), 0,
			0, 0, 0,
		}

		b.driver.EnqueueLaunchKernel(
			queue,
			b.kernel,
			[3]uint32{uint32(wiWidthPerGPU), wiHeight, 1},
			[3]uint16{uint16(b.blockSize), uint16(b.blockSize), 1},
			&kernArg,
		)
	}
}

func (b *Benchmark) exec() {
	wiWidth := uint32(b.Width / b.elemsPerThread1Dim)
	wiHeight := uint32(b.Width / b.elemsPerThread1Dim)
	numWGWidth := wiWidth / uint32(b.blockSize)
	wgXPerGPU := numWGWidth / uint32(len(b.queues))

	for i, queue := range b.queues {
		wiWidthPerGPU := int(wiWidth) / len(b.queues)
		b.enqueueKernel(queue, wiWidth, wiHeight, numWGWidth, wgXPerGPU, wiWidthPerGPU, i)
	}

	for _, q := range b.queues {
		b.driver.DrainCommandQueue(q)
	}

	b.driver.MemCopyD2H(b.context, b.hOutputData, b.dOutputData)
}

// Verify verifies
func (b *Benchmark) Verify() {
	failed := false
	for i := 0; i < b.Width; i++ {
		for j := 0; j < b.Width; j++ {
			actual := b.hOutputData[i*b.Width+j]
			expected := b.hInputData[j*b.Width+i]
			if expected != actual {
				log.Printf("mismatch at (%d, %d), expected %d, but get %d\n",
					i, j, expected, actual)
				failed = true
			}
		}
	}

	if failed {
		panic("failed to verify matrix transpose result")
	}
	log.Printf("Passed!\n")
}
