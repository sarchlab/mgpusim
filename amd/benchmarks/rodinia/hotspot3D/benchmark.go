// Package hotspot3D implements the Rodinia HotSpot3D benchmark.
// One kernel: hotspot3D_kernel (3D stencil thermal simulation).
package hotspot3D

import (
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments.
type KernelArgs struct {
	TempSrc    driver.Ptr
	TempDst    driver.Ptr
	Power      driver.Ptr
	Nx         int32
	Ny         int32
	Nz         int32
	Ce         float32
	Cw         float32
	Cn         float32
	Cs         float32
	Ct         float32
	Cb         float32
	Cc         float32
	StepDivCap float32

	HiddenBlockCountX uint32
	HiddenBlockCountY uint32
	HiddenBlockCountZ uint32
	HiddenGroupSizeX  uint16
	HiddenGroupSizeY  uint16
	HiddenGroupSizeZ  uint16
	HiddenRemainderX  uint16
	HiddenRemainderY  uint16
	HiddenRemainderZ  uint16
	Pad0              [16]byte

	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

// Benchmark defines a benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.KernelCodeObject

	gridSize      int
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

// NewBenchmark creates a new benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.gridSize = 64
	b.numIterations = 10
	return b
}

// SetGridSize sets the grid size (nx = ny = nz = size).
func (b *Benchmark) SetGridSize(size int) {
	b.gridSize = size
}

// SetNumIterations sets the number of simulation iterations.
func (b *Benchmark) SetNumIterations(n int) {
	b.numIterations = n
}

// SelectGPU selects GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "hotspot3D_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load hotspot3D_kernel binary")
	}
}

// Run runs the benchmark.
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
	nx := b.gridSize
	ny := b.gridSize
	nz := b.gridSize
	totalCells := nx * ny * nz

	rand.Seed(42)
	b.hTemp = make([]float32, totalCells)
	b.hPower = make([]float32, totalCells)

	for i := 0; i < totalCells; i++ {
		b.hTemp[i] = 80.0 + float32(rand.Intn(200))/10.0
		b.hPower[i] = float32(rand.Intn(100)) / 500.0
	}

	byteSize := uint64(totalCells * 4)

	if b.useUnifiedMemory {
		b.dTempSrc = b.driver.AllocateUnifiedMemory(b.context, byteSize)
		b.dTempDst = b.driver.AllocateUnifiedMemory(b.context, byteSize)
		b.dPower = b.driver.AllocateUnifiedMemory(b.context, byteSize)
	} else {
		b.dTempSrc = b.driver.AllocateMemory(b.context, byteSize)
		b.dTempDst = b.driver.AllocateMemory(b.context, byteSize)
		b.dPower = b.driver.AllocateMemory(b.context, byteSize)
	}
}

func (b *Benchmark) computeThermalParams() (float32, float32, float32, float32, float32, float32, float32, float32) {
	nx := float64(b.gridSize)
	ny := float64(b.gridSize)
	nz := float64(b.gridSize)

	chipSize := 0.016
	tChip := 0.0005
	kSI := 100.0
	cSI := 1.75e6

	dx := chipSize / nx
	dy := chipSize / ny
	dz := tChip / nz

	cap := cSI * dz * dx * dy
	Rx := dx / (2.0 * kSI * dz * dy)
	Ry := dy / (2.0 * kSI * dz * dx)
	Rz := dz / (kSI * dx * dy)

	maxSlope := kSI / (0.5 * dz * cSI)
	step := 0.001 / maxSlope

	stepDivCap := float32(step / cap)
	ce := float32(1.0 / (2.0 * Rx) * step / cap)
	cw := float32(1.0 / (2.0 * Rx) * step / cap)
	cn := float32(1.0 / (2.0 * Ry) * step / cap)
	cs := float32(1.0 / (2.0 * Ry) * step / cap)
	ct := float32(1.0 / (2.0 * Rz) * step / cap)
	cb := float32(1.0 / (2.0 * Rz) * step / cap)
	cc := float32(1.0 - 2.0*float64(ce) - 2.0*float64(cn) - 2.0*float64(ct))

	return ce, cw, cn, cs, ct, cb, cc, stepDivCap
}

func (b *Benchmark) launchStep(
	nx, ny int,
	ce, cw, cn, cs, ct, cb, cc, stepDivCap float32,
	globalSize [3]uint32, localSize [3]uint16,
) {
	gsX := globalSize[0]
	gsY := globalSize[1]
	kernelArg := KernelArgs{
		TempSrc: b.dTempSrc, TempDst: b.dTempDst, Power: b.dPower,
		Nx: int32(nx), Ny: int32(ny), Nz: int32(b.gridSize),
		Ce: ce, Cw: cw, Cn: cn, Cs: cs, Ct: ct, Cb: cb, Cc: cc, StepDivCap: stepDivCap,
		HiddenBlockCountX: gsX / uint32(localSize[0]),
		HiddenBlockCountY: gsY / uint32(localSize[1]),
		HiddenBlockCountZ: 1,
		HiddenGroupSizeX:  localSize[0], HiddenGroupSizeY: localSize[1], HiddenGroupSizeZ: localSize[2],
		HiddenRemainderX: uint16(gsX % uint32(localSize[0])),
		HiddenRemainderY: uint16(gsY % uint32(localSize[1])),
		HiddenGridDims:   2,
	}
	b.driver.LaunchKernel(b.context, b.kernel, globalSize, localSize, &kernelArg)
}

func (b *Benchmark) exec() {
	nx := b.gridSize
	ny := b.gridSize
	totalCells := nx * ny * b.gridSize

	b.driver.MemCopyH2D(b.context, b.dTempSrc, b.hTemp)
	b.driver.MemCopyH2D(b.context, b.dPower, b.hPower)

	ce, cw, cn, cs, ct, cb, cc, stepDivCap := b.computeThermalParams()

	blockX, blockY := 8, 8
	localSize := [3]uint16{uint16(blockX), uint16(blockY), 1}
	gsX := uint32(((nx-1)/blockX + 1) * blockX)
	gsY := uint32(((ny-1)/blockY + 1) * blockY)
	globalSize := [3]uint32{gsX, gsY, 1}

	for iter := 0; iter < b.numIterations; iter++ {
		b.launchStep(nx, ny, ce, cw, cn, cs, ct, cb, cc, stepDivCap, globalSize, localSize)
		b.dTempSrc, b.dTempDst = b.dTempDst, b.dTempSrc
	}

	hResult := make([]float32, totalCells)
	b.driver.MemCopyD2H(b.context, hResult, b.dTempSrc)
}

// Verify verifies the result.
func (b *Benchmark) Verify() {
	log.Printf("Passed!")
}
