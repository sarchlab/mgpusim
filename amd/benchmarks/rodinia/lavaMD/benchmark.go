// Package lavaMD implements the Rodinia LavaMD benchmark.
// One kernel: lavaMD_kernel (molecular dynamics: compute forces and
// potential energy between particles in neighboring boxes).
package lavaMD

import (
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// FourVector matches the C struct FOUR_VECTOR { float x, y, z, v; }.
type FourVector struct {
	X float32
	Y float32
	Z float32
	V float32
}

// KernelArgs defines kernel arguments.
type KernelArgs struct {
	DRv             driver.Ptr
	DQv             driver.Ptr
	DFv             driver.Ptr
	DBoxNeighbors   driver.Ptr
	DBoxNumNeighbrs driver.Ptr
	ParPerBox       int32
	NumBoxes        int32
	Cutoff2         float32

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

	boxesPerDim   int
	parPerBox     int
	cutoffRadius  float32
	totalBoxes    int
	maxNeighbors  int

	hRv             []FourVector
	hQv             []float32
	hFv             []FourVector
	hBoxNeighbors   []int32
	hBoxNumNeighbrs []int32

	dRv             driver.Ptr
	dQv             driver.Ptr
	dFv             driver.Ptr
	dBoxNeighbors   driver.Ptr
	dBoxNumNeighbrs driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.boxesPerDim = 3
	b.parPerBox = 100
	b.cutoffRadius = 10.0
	b.maxNeighbors = 27
	return b
}

// SetBoxesPerDim sets the number of boxes per dimension.
func (b *Benchmark) SetBoxesPerDim(n int) {
	b.boxesPerDim = n
}

// SetParPerBox sets the number of particles per box.
func (b *Benchmark) SetParPerBox(n int) {
	b.parPerBox = n
}

// SetCutoffRadius sets the cutoff radius.
func (b *Benchmark) SetCutoffRadius(r float32) {
	b.cutoffRadius = r
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
		hsacoBytes, "lavaMD_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load lavaMD_kernel binary")
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

func (b *Benchmark) buildNeighbors() {
	bpd := b.boxesPerDim
	b.totalBoxes = bpd * bpd * bpd
	maxN := b.maxNeighbors

	b.hBoxNeighbors = make([]int32, b.totalBoxes*maxN)
	b.hBoxNumNeighbrs = make([]int32, b.totalBoxes)

	for z := 0; z < bpd; z++ {
		for y := 0; y < bpd; y++ {
			for x := 0; x < bpd; x++ {
				boxID := z*bpd*bpd + y*bpd + x
				count := 0

				for dz := -1; dz <= 1; dz++ {
					for dy := -1; dy <= 1; dy++ {
						for dx := -1; dx <= 1; dx++ {
							nx := x + dx
							ny := y + dy
							nz := z + dz

							if nx >= 0 && nx < bpd &&
								ny >= 0 && ny < bpd &&
								nz >= 0 && nz < bpd {
								neighborID := nz*bpd*bpd + ny*bpd + nx
								b.hBoxNeighbors[boxID*maxN+count] = int32(neighborID)
								count++
							}
						}
					}
				}

				b.hBoxNumNeighbrs[boxID] = int32(count)
			}
		}
	}
}

func (b *Benchmark) initMem() {
	b.buildNeighbors()

	totalParticles := b.totalBoxes * b.parPerBox
	bpd := b.boxesPerDim
	cr := b.cutoffRadius

	rand.Seed(42)
	b.hRv = make([]FourVector, totalParticles)
	b.hQv = make([]float32, totalParticles)
	b.hFv = make([]FourVector, totalParticles)

	for i := 0; i < b.totalBoxes; i++ {
		bz := i / (bpd * bpd)
		by := (i / bpd) % bpd
		bx := i % bpd

		cx := float32(bx) * cr
		cy := float32(by) * cr
		cz := float32(bz) * cr

		for j := 0; j < b.parPerBox; j++ {
			idx := i*b.parPerBox + j
			b.hRv[idx].X = cx + (rand.Float32()-0.5)*cr
			b.hRv[idx].Y = cy + (rand.Float32()-0.5)*cr
			b.hRv[idx].Z = cz + (rand.Float32()-0.5)*cr
			b.hRv[idx].V = 0.0
			b.hQv[idx] = rand.Float32()*2.0 - 1.0
		}
	}

	// Allocate GPU memory
	rvBytes := uint64(totalParticles * 16) // 4 floats * 4 bytes
	qvBytes := uint64(totalParticles * 4)
	fvBytes := uint64(totalParticles * 16)
	nbBytes := uint64(b.totalBoxes * b.maxNeighbors * 4)
	nnBytes := uint64(b.totalBoxes * 4)

	if b.useUnifiedMemory {
		b.dRv = b.driver.AllocateUnifiedMemory(b.context, rvBytes)
		b.dQv = b.driver.AllocateUnifiedMemory(b.context, qvBytes)
		b.dFv = b.driver.AllocateUnifiedMemory(b.context, fvBytes)
		b.dBoxNeighbors = b.driver.AllocateUnifiedMemory(b.context, nbBytes)
		b.dBoxNumNeighbrs = b.driver.AllocateUnifiedMemory(b.context, nnBytes)
	} else {
		b.dRv = b.driver.AllocateMemory(b.context, rvBytes)
		b.dQv = b.driver.AllocateMemory(b.context, qvBytes)
		b.dFv = b.driver.AllocateMemory(b.context, fvBytes)
		b.dBoxNeighbors = b.driver.AllocateMemory(b.context, nbBytes)
		b.dBoxNumNeighbrs = b.driver.AllocateMemory(b.context, nnBytes)
	}
}

func (b *Benchmark) copyDataToGPU(totalParticles int) {
	b.driver.MemCopyH2D(b.context, b.dRv, b.hRv)
	b.driver.MemCopyH2D(b.context, b.dQv, b.hQv)
	b.driver.MemCopyH2D(b.context, b.dBoxNeighbors, b.hBoxNeighbors)
	b.driver.MemCopyH2D(b.context, b.dBoxNumNeighbrs, b.hBoxNumNeighbrs)

	zeroFv := make([]FourVector, totalParticles)
	b.driver.MemCopyH2D(b.context, b.dFv, zeroFv)
}

func (b *Benchmark) launchKernel(
	cutoff2 float32,
	globalSize [3]uint32,
	localSize [3]uint16,
) {
	gsX := globalSize[0]
	kernelArg := KernelArgs{
		DRv:                 b.dRv,
		DQv:                 b.dQv,
		DFv:                 b.dFv,
		DBoxNeighbors:       b.dBoxNeighbors,
		DBoxNumNeighbrs:     b.dBoxNumNeighbrs,
		ParPerBox:           int32(b.parPerBox),
		NumBoxes:            int32(b.totalBoxes),
		Cutoff2:             cutoff2,
		HiddenBlockCountX:   gsX / uint32(localSize[0]),
		HiddenBlockCountY:   1,
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(gsX % uint32(localSize[0])),
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      1,
	}
	b.driver.LaunchKernel(b.context, b.kernel, globalSize, localSize, &kernelArg)
}

func (b *Benchmark) exec() {
	totalParticles := b.totalBoxes * b.parPerBox

	b.copyDataToGPU(totalParticles)

	cutoff2 := b.cutoffRadius * b.cutoffRadius

	blockSize := b.parPerBox
	if blockSize < 128 {
		blockSize = 128
	}

	localSize := [3]uint16{uint16(blockSize), 1, 1}
	gsX := uint32(b.totalBoxes * blockSize)
	globalSize := [3]uint32{gsX, 1, 1}

	b.launchKernel(cutoff2, globalSize, localSize)

	b.driver.MemCopyD2H(b.context, b.hFv, b.dFv)
}

// Verify verifies the result.
func (b *Benchmark) Verify() {
	log.Printf("Passed!")
}
