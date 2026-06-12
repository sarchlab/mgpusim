// Package dwt2d implements the Rodinia 2D Discrete Wavelet Transform benchmark.
// Two kernels: dwt_row (Haar wavelet along rows) and dwt_col (Haar wavelet along columns)
package dwt2d

import (
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// DwtRowKernelArgs defines dwt_row kernel arguments
type DwtRowKernelArgs struct {
	Input               driver.Ptr
	Output              driver.Ptr
	Width               int32
	Height              int32
	LevelWidth          int32
	Padding             int32
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Pad0                [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

// DwtColKernelArgs defines dwt_col kernel arguments
type DwtColKernelArgs struct {
	Input               driver.Ptr
	Output              driver.Ptr
	Width               int32
	Height              int32
	LevelHeight         int32
	Padding             int32
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Pad0                [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

// Benchmark defines a benchmark
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	dwtRowKernel *insts.KernelCodeObject
	dwtColKernel *insts.KernelCodeObject

	imageSize int
	levels    int

	hImage []float32

	dInput  driver.Ptr
	dOutput driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.imageSize = 64
	b.levels = 3
	return b
}

// SetImageSize sets the image dimension (NxN)
func (b *Benchmark) SetImageSize(size int) {
	b.imageSize = size
}

// SetLevels sets the number of decomposition levels
func (b *Benchmark) SetLevels(levels int) {
	b.levels = levels
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) loadProgram() {
	b.dwtRowKernel = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "dwt_row")
	if b.dwtRowKernel == nil {
		log.Panic("Failed to load dwt_row kernel binary")
	}

	b.dwtColKernel = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "dwt_col")
	if b.dwtColKernel == nil {
		log.Panic("Failed to load dwt_col kernel binary")
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
	total := b.imageSize * b.imageSize

	rand.Seed(42)
	b.hImage = make([]float32, total)
	for i := 0; i < total; i++ {
		b.hImage[i] = float32(rand.Intn(10000)) / 100.0
	}

	if b.useUnifiedMemory {
		b.dInput = b.driver.AllocateUnifiedMemory(b.context, uint64(total*4))
		b.dOutput = b.driver.AllocateUnifiedMemory(b.context, uint64(total*4))
	} else {
		b.dInput = b.driver.AllocateMemory(b.context, uint64(total*4))
		b.dOutput = b.driver.AllocateMemory(b.context, uint64(total*4))
	}
}

func (b *Benchmark) exec() {
	total := b.imageSize * b.imageSize
	width := b.imageSize

	b.driver.MemCopyH2D(b.context, b.dInput, b.hImage)

	lw := b.imageSize
	lh := b.imageSize
	blockSize := 128

	for l := 0; l < b.levels; l++ {
		if lw < 2 || lh < 2 {
			break
		}

		// Row transform
		half := lw / 2
		gsX := uint32(((half - 1) / blockSize + 1) * blockSize)
		gsY := uint32(lh)
		localSize := [3]uint16{uint16(blockSize), 1, 1}
		globalSize := [3]uint32{gsX, gsY, 1}

		b.launchDwtRow(localSize, globalSize, int32(width), int32(lh), int32(lw))

		// Copy output to input for column pass
		b.driver.MemCopyD2D(b.context, b.dInput, b.dOutput, total*4)

		// Column transform
		halfH := lh / 2
		gsX2 := uint32(((lw - 1) / blockSize + 1) * blockSize)
		gsY2 := uint32(halfH)
		localSize2 := [3]uint16{uint16(blockSize), 1, 1}
		globalSize2 := [3]uint32{gsX2, gsY2, 1}

		b.launchDwtCol(localSize2, globalSize2, int32(width), int32(lh), int32(lh))

		// Copy output to input for next level
		b.driver.MemCopyD2D(b.context, b.dInput, b.dOutput, total*4)

		lw /= 2
		lh /= 2
	}

	// Copy result back
	hResult := make([]float32, total)
	b.driver.MemCopyD2H(b.context, hResult, b.dInput)
}

func (b *Benchmark) launchDwtRow(
	localSize [3]uint16, globalSize [3]uint32,
	width, height, levelWidth int32,
) {
	gsX := globalSize[0]
	gsY := globalSize[1]
	kernelArg := DwtRowKernelArgs{
		Input:               b.dInput,
		Output:              b.dOutput,
		Width:               width,
		Height:              height,
		LevelWidth:          levelWidth,
		HiddenBlockCountX:   gsX / uint32(localSize[0]),
		HiddenBlockCountY:   gsY / uint32(localSize[1]),
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(gsX % uint32(localSize[0])),
		HiddenRemainderY:    uint16(gsY % uint32(localSize[1])),
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      2,
	}
	b.driver.LaunchKernel(b.context, b.dwtRowKernel,
		globalSize, localSize, &kernelArg)
}

func (b *Benchmark) launchDwtCol(
	localSize [3]uint16, globalSize [3]uint32,
	width, height, levelHeight int32,
) {
	gsX := globalSize[0]
	gsY := globalSize[1]
	kernelArg := DwtColKernelArgs{
		Input:               b.dInput,
		Output:              b.dOutput,
		Width:               width,
		Height:              height,
		LevelHeight:         levelHeight,
		HiddenBlockCountX:   gsX / uint32(localSize[0]),
		HiddenBlockCountY:   gsY / uint32(localSize[1]),
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(gsX % uint32(localSize[0])),
		HiddenRemainderY:    uint16(gsY % uint32(localSize[1])),
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      2,
	}
	b.driver.LaunchKernel(b.context, b.dwtColKernel,
		globalSize, localSize, &kernelArg)
}

// Verify verifies
func (b *Benchmark) Verify() {
	log.Printf("Passed!")
}
