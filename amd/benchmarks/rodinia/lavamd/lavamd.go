// Package lavamd implements the Rodinia LavaMD benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/rodinia_lavamd) for the MGPUSim MI300A
// (CDNA3 / gfx942) model.
//
// LavaMD is a short-range molecular dynamics benchmark using cell-list
// decomposition. Particles are grouped into an NxNxN grid of boxes; each
// particle interacts (Lennard-Jones type potential) with all particles in
// its box and its (up to 26) neighboring boxes. One workgroup processes one
// box. The kernel binary is compiled for gfx942 only (see native/), so the
// benchmark must be run with `-arch cdna3` (the MI300A configuration).
package lavamd

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// blockSize is the workgroup size (compile-time constant in the kernel, so no
// hidden ABI args are emitted).
const blockSize = 128

// Lennard-Jones constants, matching the kernel.
const (
	ljA     float32 = 2.0
	ljB     float32 = 1.0
	boxSize float32 = 10.0
)

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout below is verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 80): nine 8-byte global_buffer pointers followed by
// two 4-byte by_value int32 scalars, packed with no padding (mgpusim
// serializes args with binary.Write, which does not insert alignment padding).
// The kernel uses a constant block size, so no hidden ABI arguments are
// emitted.
type KernelArgs struct {
	PosX            driver.Ptr // offset 0
	PosY            driver.Ptr // offset 8
	PosZ            driver.Ptr // offset 16
	ForceX          driver.Ptr // offset 24
	ForceY          driver.Ptr // offset 32
	ForceZ          driver.Ptr // offset 40
	EnergyOut       driver.Ptr // offset 48
	NeighborList    driver.Ptr // offset 56
	NeighborCount   driver.Ptr // offset 64
	ParticlesPerBox int32      // offset 72
	TotalBoxes      int32      // offset 76
}

// Benchmark defines the LavaMD benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch arch.Type

	// NumBoxes is the number of boxes per dimension (the grid is
	// NumBoxes^3 boxes).
	NumBoxes int
	// ParticlesPerBox is the number of particles in each box.
	ParticlesPerBox int

	totalBoxes     int
	totalParticles int

	hPosX          []float32
	hPosY          []float32
	hPosZ          []float32
	hNeighborList  []int32
	hNeighborCount []int32

	gPosX          driver.Ptr
	gPosY          driver.Ptr
	gPosZ          driver.Ptr
	gForceX        driver.Ptr
	gForceY        driver.Ptr
	gForceZ        driver.Ptr
	gEnergy        driver.Ptr
	gNeighborList  driver.Ptr
	gNeighborCount driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new LavaMD benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "lavamd_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. LavaMD uses a single GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// Run runs the benchmark.
func (b *Benchmark) Run() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the rodinia lavamd benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

// lcgRand is a simple linear-congruential PRNG matching the HIP host code.
func lcgRand(state *uint32) uint32 {
	*state = *state*1664525 + 1013904223
	return *state
}

func randFloat(state *uint32, lo, hi float32) float32 {
	return lo + float32(lcgRand(state)&0xFFFF)/65535.0*(hi-lo)
}

// initHostData builds the deterministic input arrays (positions, neighbor
// lists, neighbor counts). It is shared by initMem and Verify so the reference
// uses the exact same inputs.
func (b *Benchmark) initHostData() { //nolint:funlen,gocognit
	nb := b.NumBoxes
	ppb := b.ParticlesPerBox

	b.hPosX = make([]float32, b.totalParticles)
	b.hPosY = make([]float32, b.totalParticles)
	b.hPosZ = make([]float32, b.totalParticles)
	b.hNeighborList = make([]int32, b.totalBoxes*27)
	b.hNeighborCount = make([]int32, b.totalBoxes)

	var seed uint32 = 42

	for bz := 0; bz < nb; bz++ {
		for by := 0; by < nb; by++ {
			for bx := 0; bx < nb; bx++ {
				boxID := bz*nb*nb + by*nb + bx
				base := boxID * ppb

				for p := 0; p < ppb; p++ {
					b.hPosX[base+p] = float32(bx)*boxSize +
						randFloat(&seed, 0.5, boxSize-0.5)
					b.hPosY[base+p] = float32(by)*boxSize +
						randFloat(&seed, 0.5, boxSize-0.5)
					b.hPosZ[base+p] = float32(bz)*boxSize +
						randFloat(&seed, 0.5, boxSize-0.5)
				}

				count := 0
				for dz := -1; dz <= 1; dz++ {
					for dy := -1; dy <= 1; dy++ {
						for dx := -1; dx <= 1; dx++ {
							nnx := bx + dx
							nny := by + dy
							nnz := bz + dz
							if nnx >= 0 && nnx < nb &&
								nny >= 0 && nny < nb &&
								nnz >= 0 && nnz < nb {
								nid := nnz*nb*nb + nny*nb + nnx
								b.hNeighborList[boxID*27+count] = int32(nid)
								count++
							}
						}
					}
				}
				b.hNeighborCount[boxID] = int32(count)
			}
		}
	}
}

func (b *Benchmark) initMem() {
	if b.NumBoxes <= 0 {
		b.NumBoxes = 4
	}
	if b.ParticlesPerBox <= 0 {
		b.ParticlesPerBox = 100
	}

	b.totalBoxes = b.NumBoxes * b.NumBoxes * b.NumBoxes
	b.totalParticles = b.totalBoxes * b.ParticlesPerBox

	b.initHostData()

	posBytes := uint64(b.totalParticles * 4)
	neighBytes := uint64(b.totalBoxes * 27 * 4)
	ncountBytes := uint64(b.totalBoxes * 4)

	alloc := b.driver.AllocateMemory
	if b.useUnifiedMemory {
		alloc = b.driver.AllocateUnifiedMemory
	}

	b.gPosX = alloc(b.context, posBytes)
	b.gPosY = alloc(b.context, posBytes)
	b.gPosZ = alloc(b.context, posBytes)
	b.gForceX = alloc(b.context, posBytes)
	b.gForceY = alloc(b.context, posBytes)
	b.gForceZ = alloc(b.context, posBytes)
	b.gEnergy = alloc(b.context, posBytes)
	b.gNeighborList = alloc(b.context, neighBytes)
	b.gNeighborCount = alloc(b.context, ncountBytes)

	b.driver.MemCopyH2D(b.context, b.gPosX, b.hPosX)
	b.driver.MemCopyH2D(b.context, b.gPosY, b.hPosY)
	b.driver.MemCopyH2D(b.context, b.gPosZ, b.hPosZ)
	b.driver.MemCopyH2D(b.context, b.gNeighborList, b.hNeighborList)
	b.driver.MemCopyH2D(b.context, b.gNeighborCount, b.hNeighborCount)
}

func (b *Benchmark) exec() {
	args := KernelArgs{
		PosX:            b.gPosX,
		PosY:            b.gPosY,
		PosZ:            b.gPosZ,
		ForceX:          b.gForceX,
		ForceY:          b.gForceY,
		ForceZ:          b.gForceZ,
		EnergyOut:       b.gEnergy,
		NeighborList:    b.gNeighborList,
		NeighborCount:   b.gNeighborCount,
		ParticlesPerBox: int32(b.ParticlesPerBox),
		TotalBoxes:      int32(b.totalBoxes),
	}

	// Grid: one workgroup per box, blockSize work-items per workgroup.
	globalX := uint32(b.totalBoxes) * blockSize

	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsaco,
		[3]uint32{globalX, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&args,
	)

	b.driver.DrainCommandQueue(b.queue)
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) Verify() {
	gpuFX := make([]float32, b.totalParticles)
	gpuFY := make([]float32, b.totalParticles)
	gpuFZ := make([]float32, b.totalParticles)
	gpuPE := make([]float32, b.totalParticles)
	b.driver.MemCopyD2H(b.context, gpuFX, b.gForceX)
	b.driver.MemCopyD2H(b.context, gpuFY, b.gForceY)
	b.driver.MemCopyD2H(b.context, gpuFZ, b.gForceZ)
	b.driver.MemCopyD2H(b.context, gpuPE, b.gEnergy)

	ppb := b.ParticlesPerBox

	for boxID := 0; boxID < b.totalBoxes; boxID++ {
		baseI := boxID * ppb
		nNeighbors := int(b.hNeighborCount[boxID])

		for p := 0; p < ppb; p++ {
			i := baseI + p
			px := b.hPosX[i]
			py := b.hPosY[i]
			pz := b.hPosZ[i]

			var fx, fy, fz, pe float32

			for n := 0; n < nNeighbors; n++ {
				nbox := int(b.hNeighborList[boxID*27+n])
				baseJ := nbox * ppb

				for q := 0; q < ppb; q++ {
					j := baseJ + q

					dx := px - b.hPosX[j]
					dy := py - b.hPosY[j]
					dz := pz - b.hPosZ[j]

					r2 := dx*dx + dy*dy + dz*dz

					if r2 > 1e-10 {
						r2inv := float32(1.0) / r2
						r6inv := r2inv * r2inv * r2inv

						force := r2inv * r6inv * (ljA*r6inv - ljB)
						pe += r6inv * (ljA*r6inv - ljB)

						fx += force * dx
						fy += force * dy
						fz += force * dz
					}
				}
			}

			checkClose(i, "force_x", fx, gpuFX[i])
			checkClose(i, "force_y", fy, gpuFY[i])
			checkClose(i, "force_z", fz, gpuFZ[i])
			checkClose(i, "energy", pe, gpuPE[i])
		}
	}

	log.Printf("Passed!\n")
}

func checkClose(idx int, name string, ref, got float32) {
	denom := math.Abs(float64(ref))
	if denom < 1.0 {
		denom = 1.0
	}
	if math.Abs(float64(ref-got))/denom > 1e-3 {
		log.Fatalf("At particle %d, %s expected %f, but got %f.\n",
			idx, name, ref, got)
	}
}
