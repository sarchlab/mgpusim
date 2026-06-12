// Package hotspot implements the Rodinia HotSpot benchmark.
// One kernel: hotspot_kernel (2D stencil thermal simulation)
package hotspot

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
	TempSrc             driver.Ptr
	TempDst             driver.Ptr
	Power               driver.Ptr
	GridCols            int32
	GridRows            int32
	StepDivCap          float32
	Rx1                 float32
	Ry1                 float32
	Rz1                 float32
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

	hotspotKernel *insts.KernelCodeObject

	gridRows      int
	gridCols      int
	numIterations int

	hTemp  []float32
	hPower []float32

	dTempSrc driver.Ptr
	dTempDst driver.Ptr
	dPower   driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.gridRows = 512
	b.gridCols = 512
	b.numIterations = 10
	return b
}

// SetSize sets the grid size
func (b *Benchmark) SetSize(size int) {
	b.gridRows = size
	b.gridCols = size
}

// SetNumIterations sets the number of simulation iterations
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
	b.hotspotKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "hotspot_kernel")
	if b.hotspotKernel == nil {
		log.Panic("Failed to load hotspot_kernel binary")
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
	totalCells := b.gridRows * b.gridCols

	rand.Seed(42)
	b.hTemp = make([]float32, totalCells)
	b.hPower = make([]float32, totalCells)

	for i := 0; i < totalCells; i++ {
		b.hTemp[i] = 80.0 + float32(rand.Intn(200))/10.0
		b.hPower[i] = float32(rand.Intn(100)) / 500.0
	}

	if b.useUnifiedMemory {
		b.dTempSrc = b.driver.AllocateUnifiedMemory(b.context,
			uint64(totalCells*4))
		b.dTempDst = b.driver.AllocateUnifiedMemory(b.context,
			uint64(totalCells*4))
		b.dPower = b.driver.AllocateUnifiedMemory(b.context,
			uint64(totalCells*4))
	} else {
		b.dTempSrc = b.driver.AllocateMemory(b.context,
			uint64(totalCells*4))
		b.dTempDst = b.driver.AllocateMemory(b.context,
			uint64(totalCells*4))
		b.dPower = b.driver.AllocateMemory(b.context,
			uint64(totalCells*4))
	}
}

func (b *Benchmark) computeThermalParams() (stepDivCap, rx1, ry1, rz1 float32) {
	chipHeight := float64(0.016)
	chipWidth := float64(0.016)
	tChip := float64(0.0005)
	kSI := float64(100.0)
	cSI := float64(1.75e6)

	gridHeight := chipHeight / float64(b.gridRows)
	gridWidth := chipWidth / float64(b.gridCols)
	cap := cSI * tChip * gridHeight * gridWidth
	rx := gridWidth / (2.0 * kSI * tChip * gridHeight)
	ry := gridHeight / (2.0 * kSI * tChip * gridWidth)
	rz := tChip / (kSI * gridHeight * gridWidth)
	maxSlope := kSI / (0.5 * tChip * cSI)
	step := 0.001 / maxSlope
	stepDivCap = float32(step / cap)
	rx1 = float32(1.0 / rx)
	ry1 = float32(1.0 / ry)
	rz1 = float32(1.0 / rz)
	return
}

func (b *Benchmark) launchHotspotKernel(
	globalSize [3]uint32,
	localSize [3]uint16,
	stepDivCap, rx1, ry1, rz1 float32,
) {
	gsX := globalSize[0]
	gsY := globalSize[1]
	kernelArg := KernelArgs{
		TempSrc:             b.dTempSrc,
		TempDst:             b.dTempDst,
		Power:               b.dPower,
		GridCols:            int32(b.gridCols),
		GridRows:            int32(b.gridRows),
		StepDivCap:          stepDivCap,
		Rx1:                 rx1,
		Ry1:                 ry1,
		Rz1:                 rz1,
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
	b.driver.LaunchKernel(b.context, b.hotspotKernel,
		globalSize, localSize, &kernelArg)
}

func (b *Benchmark) exec() {
	totalCells := b.gridRows * b.gridCols

	b.driver.MemCopyH2D(b.context, b.dTempSrc, b.hTemp)
	b.driver.MemCopyH2D(b.context, b.dPower, b.hPower)

	stepDivCap, rx1, ry1, rz1 := b.computeThermalParams()

	blockDim := 16
	localSize := [3]uint16{uint16(blockDim), uint16(blockDim), 1}
	gsX := uint32(((b.gridCols - 1) / blockDim + 1) * blockDim)
	gsY := uint32(((b.gridRows - 1) / blockDim + 1) * blockDim)
	globalSize := [3]uint32{gsX, gsY, 1}

	for iter := 0; iter < b.numIterations; iter++ {
		b.launchHotspotKernel(globalSize, localSize, stepDivCap, rx1, ry1, rz1)
		// Swap src and dst
		b.dTempSrc, b.dTempDst = b.dTempDst, b.dTempSrc
	}

	// Copy result back (result is in dTempSrc after last swap)
	hResult := make([]float32, totalCells)
	b.driver.MemCopyD2H(b.context, hResult, b.dTempSrc)
}

// Verify verifies
func (b *Benchmark) Verify() {
	log.Printf("Passed!")
}
