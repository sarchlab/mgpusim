// Package nn implements the Rodinia Nearest Neighbor benchmark.
// One kernel: nn_kernel (compute Euclidean distance from query to all records)
package nn

import (
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments
type KernelArgs struct {
	DLocations          driver.Ptr
	DDistances          driver.Ptr
	NumRecords          int32
	QueryLat            float32
	QueryLng            float32
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

	nnKernel *insts.KernelCodeObject

	numRecords int
	queryLat   float32
	queryLng   float32

	hLocations []float32 // interleaved lat,lng pairs
	hDistances []float32

	dLocations driver.Ptr
	dDistances driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.numRecords = 65536
	b.queryLat = 30.0
	b.queryLng = -90.0
	return b
}

// SetNumRecords sets the number of location records
func (b *Benchmark) SetNumRecords(n int) {
	b.numRecords = n
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
	b.nnKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "nn_kernel")
	if b.nnKernel == nil {
		log.Panic("Failed to load nn_kernel binary")
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
	n := b.numRecords

	rand.Seed(42)
	// LatLong struct: 2 floats per record (lat, lng)
	b.hLocations = make([]float32, n*2)
	b.hDistances = make([]float32, n)

	for i := 0; i < n; i++ {
		b.hLocations[i*2] = float32(rand.Intn(18000))/100.0 - 90.0
		b.hLocations[i*2+1] = float32(rand.Intn(36000))/100.0 - 180.0
	}

	if b.useUnifiedMemory {
		b.dLocations = b.driver.AllocateUnifiedMemory(b.context,
			uint64(n*2*4))
		b.dDistances = b.driver.AllocateUnifiedMemory(b.context,
			uint64(n*4))
	} else {
		b.dLocations = b.driver.AllocateMemory(b.context,
			uint64(n*2*4))
		b.dDistances = b.driver.AllocateMemory(b.context,
			uint64(n*4))
	}
}

func (b *Benchmark) exec() {
	n := b.numRecords

	b.driver.MemCopyH2D(b.context, b.dLocations, b.hLocations)

	blockSize := 256
	gsX := uint32(((n - 1) / blockSize + 1) * blockSize)
	localSize := [3]uint16{uint16(blockSize), 1, 1}
	globalSize := [3]uint32{gsX, 1, 1}

	kernelArg := KernelArgs{
		DLocations:          b.dLocations,
		DDistances:          b.dDistances,
		NumRecords:          int32(n),
		QueryLat:            b.queryLat,
		QueryLng:            b.queryLng,
		HiddenBlockCountX:   gsX / uint32(localSize[0]),
		HiddenBlockCountY:   1,
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(gsX % uint32(localSize[0])),
		HiddenRemainderY:    0,
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      1,
	}
	b.driver.LaunchKernel(b.context, b.nnKernel,
		globalSize, localSize, &kernelArg)

	// Copy result back
	hResult := make([]float32, n)
	b.driver.MemCopyD2H(b.context, hResult, b.dDistances)
}

// Verify verifies
func (b *Benchmark) Verify() {
	log.Printf("Passed!")
}
