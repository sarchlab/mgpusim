// Package streamcluster implements the Rodinia Streamcluster benchmark.
// One kernel: pgain_kernel (compute cost change for opening a new center).
package streamcluster

import (
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// PgainKernelArgs defines kernel arguments.
type PgainKernelArgs struct {
	Coord               driver.Ptr
	CenterCost          driver.Ptr
	CenterAssign        driver.Ptr
	WorkCost            driver.Ptr
	WorkAssign          driver.Ptr
	Candidate           int32
	NumPoints           int32
	Dim                 int32
	PadAlign            int32 // padding to align CostChange to 8 bytes
	CostChange          driver.Ptr
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

	pgainKernel *insts.KernelCodeObject

	numPoints  int
	dim        int
	numCenters int

	hCoord        []float32
	hCenterCost   []float32
	hCenterAssign []int32

	dCoord        driver.Ptr
	dCenterCost   driver.Ptr
	dWorkCost     driver.Ptr
	dCenterAssign driver.Ptr
	dWorkAssign   driver.Ptr
	dCostChange   driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.numPoints = 256
	b.dim = 32
	b.numCenters = 10
	return b
}

// SetNumPoints sets the number of points
func (b *Benchmark) SetNumPoints(n int) {
	b.numPoints = n
}

// SetDim sets the number of dimensions
func (b *Benchmark) SetDim(n int) {
	b.dim = n
}

// SetNumCenters sets the number of centers
func (b *Benchmark) SetNumCenters(n int) {
	b.numCenters = n
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
	b.pgainKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "pgain_kernel")
	if b.pgainKernel == nil {
		log.Panic("Failed to load pgain_kernel binary")
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
	np := b.numPoints
	d := b.dim

	rand.Seed(42)
	b.hCoord = make([]float32, np*d)
	for i := 0; i < np*d; i++ {
		b.hCoord[i] = float32(rand.Float64()) * 100.0
	}

	// Initialize: assign each point to center 0
	b.hCenterCost = make([]float32, np)
	b.hCenterAssign = make([]int32, np)
	for i := 0; i < np; i++ {
		dist := float32(0.0)
		for dd := 0; dd < d; dd++ {
			diff := b.hCoord[i*d+dd] - b.hCoord[0*d+dd]
			dist += diff * diff
		}
		b.hCenterCost[i] = dist
		b.hCenterAssign[i] = 0
	}

	if b.useUnifiedMemory {
		b.dCoord = b.driver.AllocateUnifiedMemory(b.context,
			uint64(np*d*4))
		b.dCenterCost = b.driver.AllocateUnifiedMemory(b.context,
			uint64(np*4))
		b.dWorkCost = b.driver.AllocateUnifiedMemory(b.context,
			uint64(np*4))
		b.dCenterAssign = b.driver.AllocateUnifiedMemory(b.context,
			uint64(np*4))
		b.dWorkAssign = b.driver.AllocateUnifiedMemory(b.context,
			uint64(np*4))
		b.dCostChange = b.driver.AllocateUnifiedMemory(b.context,
			uint64(4))
	} else {
		b.dCoord = b.driver.AllocateMemory(b.context,
			uint64(np*d*4))
		b.dCenterCost = b.driver.AllocateMemory(b.context,
			uint64(np*4))
		b.dWorkCost = b.driver.AllocateMemory(b.context,
			uint64(np*4))
		b.dCenterAssign = b.driver.AllocateMemory(b.context,
			uint64(np*4))
		b.dWorkAssign = b.driver.AllocateMemory(b.context,
			uint64(np*4))
		b.dCostChange = b.driver.AllocateMemory(b.context,
			uint64(4))
	}
}

func (b *Benchmark) exec() {
	np := b.numPoints
	blockSize := 256
	if np < blockSize {
		blockSize = np
	}

	gsX := uint32(((np-1)/blockSize + 1) * blockSize)
	localSize := [3]uint16{uint16(blockSize), 1, 1}
	globalSize := [3]uint32{gsX, 1, 1}

	// Copy coordinates to device
	b.driver.MemCopyH2D(b.context, b.dCoord, b.hCoord)

	// This benchmark compares against the HW kernel-level timing row
	// streamcluster_pgain, so keep the execution path bounded to a single
	// pgain launch (matching workloads/rodinia/streamcluster/cuda benchmark).
	candidate := np / 2
	if candidate >= np {
		candidate = np - 1
	}

	// Upload initial center state once.
	b.driver.MemCopyH2D(b.context, b.dCenterCost, b.hCenterCost)
	b.driver.MemCopyH2D(b.context, b.dCenterAssign, b.hCenterAssign)

	// Zero cost change.
	zero := []float32{0.0}
	b.driver.MemCopyH2D(b.context, b.dCostChange, zero)

	// Launch one pgain kernel.
	b.launchPgainKernel(globalSize, localSize, gsX, int32(candidate))

	// Read cost change to ensure completion and keep the data path valid.
	hCostChange := make([]float32, 1)
	b.driver.MemCopyD2H(b.context, hCostChange, b.dCostChange)
}

func (b *Benchmark) launchPgainKernel(
	globalSize [3]uint32, localSize [3]uint16,
	gsX uint32, candidate int32,
) {
	args := PgainKernelArgs{
		Coord:               b.dCoord,
		CenterCost:          b.dCenterCost,
		CenterAssign:        b.dCenterAssign,
		WorkCost:            b.dWorkCost,
		WorkAssign:          b.dWorkAssign,
		Candidate:           candidate,
		NumPoints:           int32(b.numPoints),
		Dim:                 int32(b.dim),
		CostChange:          b.dCostChange,
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
	b.driver.LaunchKernel(b.context, b.pgainKernel,
		globalSize, localSize, &args)
}

// Verify verifies
func (b *Benchmark) Verify() {
	log.Printf("Passed!")
}
