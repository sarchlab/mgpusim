// Package lavamd implements the Rodinia LavaMD benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/rodinia_lavamd) for the MGPUSim MI300X
// (CDNA3 / gfx942) model.
//
// LavaMD is a short-range molecular dynamics benchmark using cell-list
// decomposition. Particles are grouped into an NxNxN grid of boxes; each
// particle interacts (Lennard-Jones type potential) with all particles in
// its box and its (up to 26) neighboring boxes. One workgroup processes one
// box. The kernel binary is compiled for gfx942 only (see native/), so the
// benchmark must be run with `-arch cdna3` (the MI300X configuration).
package lavamd

import (
	// embed hsaco files
	_ "embed"
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
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
	*State
	benchmarks.Saved[checkpointConfig, State]

	driver *driver.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new LavaMD benchmark.
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Saved = benchmarks.NewSaved(b.State, b.checkpointConfig, b.bind)

	b.driver = driver
	b.Context = b.driver.Init()

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
	b.Gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) submit() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the rodinia lavamd benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.driver.CreateCommandQueue(b.Context)
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

	b.HPosX = make([]float32, b.TotalParticles)
	b.HPosY = make([]float32, b.TotalParticles)
	b.HPosZ = make([]float32, b.TotalParticles)
	b.HNeighborList = make([]int32, b.TotalBoxes*27)
	b.HNeighborCount = make([]int32, b.TotalBoxes)

	var seed uint32 = 42

	for bz := 0; bz < nb; bz++ {
		for by := 0; by < nb; by++ {
			for bx := 0; bx < nb; bx++ {
				boxID := bz*nb*nb + by*nb + bx
				base := boxID * ppb

				for p := 0; p < ppb; p++ {
					b.HPosX[base+p] = float32(bx)*boxSize +
						randFloat(&seed, 0.5, boxSize-0.5)
					b.HPosY[base+p] = float32(by)*boxSize +
						randFloat(&seed, 0.5, boxSize-0.5)
					b.HPosZ[base+p] = float32(bz)*boxSize +
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
								b.HNeighborList[boxID*27+count] = int32(nid)
								count++
							}
						}
					}
				}
				b.HNeighborCount[boxID] = int32(count)
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

	b.TotalBoxes = b.NumBoxes * b.NumBoxes * b.NumBoxes
	b.TotalParticles = b.TotalBoxes * b.ParticlesPerBox

	b.initHostData()

	posBytes := uint64(b.TotalParticles * 4)
	neighBytes := uint64(b.TotalBoxes * 27 * 4)
	ncountBytes := uint64(b.TotalBoxes * 4)

	alloc := b.driver.AllocateMemory
	if b.UseUnifiedMemory {
		alloc = b.driver.AllocateUnifiedMemory
	}

	b.GPosX = alloc(b.Context, posBytes)
	b.GPosY = alloc(b.Context, posBytes)
	b.GPosZ = alloc(b.Context, posBytes)
	b.GForceX = alloc(b.Context, posBytes)
	b.GForceY = alloc(b.Context, posBytes)
	b.GForceZ = alloc(b.Context, posBytes)
	b.GEnergy = alloc(b.Context, posBytes)
	b.GNeighborList = alloc(b.Context, neighBytes)
	b.GNeighborCount = alloc(b.Context, ncountBytes)

	b.driver.EnqueueMemCopyH2D(b.Queue, b.GPosX, b.HPosX)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GPosY, b.HPosY)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GPosZ, b.HPosZ)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GNeighborList, b.HNeighborList)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GNeighborCount, b.HNeighborCount)
}

func (b *Benchmark) exec() {
	args := KernelArgs{
		PosX:            b.GPosX,
		PosY:            b.GPosY,
		PosZ:            b.GPosZ,
		ForceX:          b.GForceX,
		ForceY:          b.GForceY,
		ForceZ:          b.GForceZ,
		EnergyOut:       b.GEnergy,
		NeighborList:    b.GNeighborList,
		NeighborCount:   b.GNeighborCount,
		ParticlesPerBox: int32(b.ParticlesPerBox),
		TotalBoxes:      int32(b.TotalBoxes),
	}

	// Grid: one workgroup per box, blockSize work-items per workgroup.
	globalX := uint32(b.TotalBoxes) * blockSize

	b.driver.EnqueueLaunchKernel(
		b.Queue,
		b.hsaco,
		[3]uint32{globalX, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&args,
	)
}

// Verify checks the GPU result against a CPU reference computation.
//
//nolint:funlen // Keep the CPU reference algorithm and comparison together.
func (b *Benchmark) verify() error {
	gpuFX := make([]float32, b.TotalParticles)
	gpuFY := make([]float32, b.TotalParticles)
	gpuFZ := make([]float32, b.TotalParticles)
	gpuPE := make([]float32, b.TotalParticles)
	if err := b.driver.ReadResult(b.Readbacks[0], gpuFX); err != nil {
		return err
	}
	if err := b.driver.ReadResult(b.Readbacks[1], gpuFY); err != nil {
		return err
	}
	if err := b.driver.ReadResult(b.Readbacks[2], gpuFZ); err != nil {
		return err
	}
	if err := b.driver.ReadResult(b.Readbacks[3], gpuPE); err != nil {
		return err
	}

	ppb := b.ParticlesPerBox

	for boxID := 0; boxID < b.TotalBoxes; boxID++ {
		baseI := boxID * ppb
		nNeighbors := int(b.HNeighborCount[boxID])

		for p := 0; p < ppb; p++ {
			i := baseI + p
			px := b.HPosX[i]
			py := b.HPosY[i]
			pz := b.HPosZ[i]

			var fx, fy, fz, pe float32

			for n := 0; n < nNeighbors; n++ {
				nbox := int(b.HNeighborList[boxID*27+n])
				baseJ := nbox * ppb

				for q := 0; q < ppb; q++ {
					j := baseJ + q

					dx := px - b.HPosX[j]
					dy := py - b.HPosY[j]
					dz := pz - b.HPosZ[j]

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

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, b.driver.EnqueueMemCopyD2H(b.Queue, b.GForceX, 4*(b.TotalParticles)))
	b.Readbacks = append(b.Readbacks, b.driver.EnqueueMemCopyD2H(b.Queue, b.GForceY, 4*(b.TotalParticles)))
	b.Readbacks = append(b.Readbacks, b.driver.EnqueueMemCopyD2H(b.Queue, b.GForceZ, 4*(b.TotalParticles)))
	b.Readbacks = append(b.Readbacks, b.driver.EnqueueMemCopyD2H(b.Queue, b.GEnergy, 4*(b.TotalParticles)))
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

// Step submits computation, then verification readback, then consumes results.
func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}

	switch b.Phase {
	case 0:
		b.submit()
		b.Phase = 1
	case 1:
		if !b.Verification {
			b.Ended = true
			return nil
		}
		b.enqueueVerification()
		b.Phase = 2
	case 2:
		if err := b.verify(); err != nil {
			return err
		}
		b.Readbacks = nil
		b.Ended = true
	}
	return nil
}
