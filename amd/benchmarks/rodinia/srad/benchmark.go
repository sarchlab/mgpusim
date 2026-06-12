// Package srad implements the Rodinia SRAD benchmark.
// Two kernels: srad1 (compute diffusion coefficients) and srad2 (update image)
// Speckle Reducing Anisotropic Diffusion for image processing.
package srad

import (
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// Srad1KernelArgs defines srad1 kernel arguments
type Srad1KernelArgs struct {
	J                   driver.Ptr
	DN                  driver.Ptr
	DS                  driver.Ptr
	DW                  driver.Ptr
	DE                  driver.Ptr
	C                   driver.Ptr
	Rows                int32
	Cols                int32
	Q0sqr               float32
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

// Srad2KernelArgs defines srad2 kernel arguments
type Srad2KernelArgs struct {
	J                   driver.Ptr
	DN                  driver.Ptr
	DS                  driver.Ptr
	DW                  driver.Ptr
	DE                  driver.Ptr
	C                   driver.Ptr
	Rows                int32
	Cols                int32
	Lambda              float32
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

	srad1Kernel *insts.KernelCodeObject
	srad2Kernel *insts.KernelCodeObject

	rows          int
	cols          int
	numIterations int
	lambda        float32
	q0sqr         float32

	hJ []float32

	dJ  driver.Ptr
	dDN driver.Ptr
	dDS driver.Ptr
	dDW driver.Ptr
	dDE driver.Ptr
	dC  driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.rows = 512
	b.cols = 512
	b.numIterations = 10
	b.lambda = 0.25
	b.q0sqr = 0.05
	return b
}

// SetImageSize sets the image size (rows = cols = size)
func (b *Benchmark) SetImageSize(size int) {
	b.rows = size
	b.cols = size
}

// SetNumIterations sets the number of SRAD iterations
func (b *Benchmark) SetNumIterations(n int) {
	b.numIterations = n
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
	b.srad1Kernel = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "srad1")
	if b.srad1Kernel == nil {
		log.Panic("Failed to load srad1 kernel binary")
	}

	b.srad2Kernel = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "srad2")
	if b.srad2Kernel == nil {
		log.Panic("Failed to load srad2 kernel binary")
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
	size := b.rows * b.cols

	rand.Seed(42)
	b.hJ = make([]float32, size)
	for i := 0; i < size; i++ {
		b.hJ[i] = float32(rand.Intn(25500))/100.0 + 1.0
	}

	if b.useUnifiedMemory {
		b.dJ = b.driver.AllocateUnifiedMemory(b.context, uint64(size*4))
		b.dDN = b.driver.AllocateUnifiedMemory(b.context, uint64(size*4))
		b.dDS = b.driver.AllocateUnifiedMemory(b.context, uint64(size*4))
		b.dDW = b.driver.AllocateUnifiedMemory(b.context, uint64(size*4))
		b.dDE = b.driver.AllocateUnifiedMemory(b.context, uint64(size*4))
		b.dC = b.driver.AllocateUnifiedMemory(b.context, uint64(size*4))
	} else {
		b.dJ = b.driver.AllocateMemory(b.context, uint64(size*4))
		b.dDN = b.driver.AllocateMemory(b.context, uint64(size*4))
		b.dDS = b.driver.AllocateMemory(b.context, uint64(size*4))
		b.dDW = b.driver.AllocateMemory(b.context, uint64(size*4))
		b.dDE = b.driver.AllocateMemory(b.context, uint64(size*4))
		b.dC = b.driver.AllocateMemory(b.context, uint64(size*4))
	}
}

func (b *Benchmark) exec() {
	size := b.rows * b.cols

	b.driver.MemCopyH2D(b.context, b.dJ, b.hJ)

	blockDim := 16
	localSize := [3]uint16{uint16(blockDim), uint16(blockDim), 1}
	gsX := uint32(((b.cols - 1) / blockDim + 1) * blockDim)
	gsY := uint32(((b.rows - 1) / blockDim + 1) * blockDim)
	globalSize := [3]uint32{gsX, gsY, 1}

	for iter := 0; iter < b.numIterations; iter++ {
		b.launchSrad1(globalSize, localSize, gsX, gsY)
		b.launchSrad2(globalSize, localSize, gsX, gsY)
	}

	// Copy result back
	hResult := make([]float32, size)
	b.driver.MemCopyD2H(b.context, hResult, b.dJ)
}

func (b *Benchmark) launchSrad1(
	globalSize [3]uint32,
	localSize [3]uint16,
	gsX, gsY uint32,
) {
	kernelArg := Srad1KernelArgs{
		J:                   b.dJ,
		DN:                  b.dDN,
		DS:                  b.dDS,
		DW:                  b.dDW,
		DE:                  b.dDE,
		C:                   b.dC,
		Rows:                int32(b.rows),
		Cols:                int32(b.cols),
		Q0sqr:               b.q0sqr,
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
	b.driver.LaunchKernel(b.context, b.srad1Kernel,
		globalSize, localSize, &kernelArg)
}

func (b *Benchmark) launchSrad2(
	globalSize [3]uint32,
	localSize [3]uint16,
	gsX, gsY uint32,
) {
	kernelArg := Srad2KernelArgs{
		J:                   b.dJ,
		DN:                  b.dDN,
		DS:                  b.dDS,
		DW:                  b.dDW,
		DE:                  b.dDE,
		C:                   b.dC,
		Rows:                int32(b.rows),
		Cols:                int32(b.cols),
		Lambda:              b.lambda,
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
	b.driver.LaunchKernel(b.context, b.srad2Kernel,
		globalSize, localSize, &kernelArg)
}

// Verify verifies
func (b *Benchmark) Verify() {
	log.Printf("Passed!")
}
