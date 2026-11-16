// Package cutcp implements the Parboil CUTCP benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/parboil_cutcp) for the MGPUSim MI300X
// (CDNA3 / gfx942) model.
//
// CUTCP (Coulombic potential with cutoff) is a direct summation of
// electrostatic charge interactions on a 3D grid: each thread computes the
// potential at one grid point by iterating over all atoms within a cutoff
// radius. The kernel binary is compiled for gfx942 only (see native/), so
// the benchmark must be run with `-arch cdna3` (the MI300X configuration).
//
// The native kernel uses a constant BLOCK_SIZE (= 128) for its 1D launch,
// so the compiler emits no hidden ABI arguments (kernarg_segment_size = 32).
package cutcp

import (
	// embed hsaco files
	_ "embed"
	"fmt"
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// blockSize is the constant 1D work-group size baked into the gfx942 kernel.
const blockSize = 128

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout below is verified against the compiled kernel's AMDGPU
// metadata (kernarg_segment_size = 32): two 8-byte global_buffer pointers
// followed by four 4-byte by_value scalars, packed with no padding
// (mgpusim serializes args with binary.Write, which does not insert
// alignment padding). The kernel uses a constant BLOCK_SIZE for its launch
// geometry, so no hidden ABI arguments are emitted.
type KernelArgs struct {
	Atoms       gpuruntime.Ptr // offset 0  (flat float4 array {x,y,z,charge})
	Potential   gpuruntime.Ptr // offset 8
	NumAtoms    int32          // offset 16
	GridSide    int32          // offset 20
	GridSpacing float32        // offset 24
	Cutoff2     float32        // offset 28
}

// Benchmark defines the CUTCP benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new CUTCP benchmark.
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)

	b.rt = rt
	b.Context = b.rt.Init()

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "cutcp_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. CUTCP uses a single GPU.
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
		log.Panic("the parboil cutcp benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
}

// lcgRand reproduces the host LCG PRNG from the original HIP benchmark so
// that Verify() can regenerate the exact same atom data.
func lcgRand(state *uint32) uint32 {
	*state = *state*1664525 + 1013904223
	return *state
}

func randFloat(state *uint32, lo, hi float32) float32 {
	return lo + float32(lcgRand(state)&0xFFFF)/65535.0*(hi-lo)
}

func (b *Benchmark) initMem() {
	if b.NumAtoms <= 0 {
		b.NumAtoms = 64
	}
	if b.GridSide <= 0 {
		b.GridSide = 8
	}
	if b.GridSpacing <= 0 {
		b.GridSpacing = 0.5
	}
	if b.Cutoff <= 0 {
		b.Cutoff = 12.0
	}

	totalPoints := b.GridSide * b.GridSide * b.GridSide
	domainSize := float32(b.GridSide) * b.GridSpacing

	// Generate synthetic atom data with the same LCG/seed as the HIP host.
	b.Atoms = make([]float32, b.NumAtoms*4)
	var seed uint32 = 42
	for i := 0; i < b.NumAtoms; i++ {
		x := randFloat(&seed, 0.0, domainSize)
		y := randFloat(&seed, 0.0, domainSize)
		z := randFloat(&seed, 0.0, domainSize)
		q := randFloat(&seed, -1.0, 1.0)
		b.Atoms[i*4+0] = x
		b.Atoms[i*4+1] = y
		b.Atoms[i*4+2] = z
		b.Atoms[i*4+3] = q
	}

	b.Potential = make([]float32, totalPoints)

	if b.UseUnifiedMemory {
		b.GAtoms = b.rt.AllocateUnifiedMemory(
			b.Context, uint64(b.NumAtoms*4*4))
		b.GPot = b.rt.AllocateUnifiedMemory(
			b.Context, uint64(totalPoints*4))
	} else {
		b.GAtoms = b.rt.AllocateMemory(
			b.Context, uint64(b.NumAtoms*4*4))
		b.GPot = b.rt.AllocateMemory(
			b.Context, uint64(totalPoints*4))
	}

	b.Upload(b.rt, b.Queue, b.GAtoms, b.Atoms)
}

func (b *Benchmark) exec() {
	totalPoints := b.GridSide * b.GridSide * b.GridSide
	gridSize := uint32((totalPoints + blockSize - 1) / blockSize)
	globalX := gridSize * blockSize
	cutoff2 := b.Cutoff * b.Cutoff

	args := KernelArgs{
		Atoms:       b.GAtoms,
		Potential:   b.GPot,
		NumAtoms:    int32(b.NumAtoms),
		GridSide:    int32(b.GridSide),
		GridSpacing: b.GridSpacing,
		Cutoff2:     cutoff2,
	}

	b.rt.EnqueueLaunchKernel(
		b.Queue,
		b.hsaco,
		[3]uint32{globalX, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&args,
	)
}

// cpuPotential computes the reference potential at a single grid point.
func (b *Benchmark) cpuPotential(px, py, pz, cutoff2 float32) float32 {
	var pot float32
	for i := 0; i < b.NumAtoms; i++ {
		dx := px - b.Atoms[i*4+0]
		dy := py - b.Atoms[i*4+1]
		dz := pz - b.Atoms[i*4+2]
		r2 := dx*dx + dy*dy + dz*dz
		if r2 < cutoff2 && r2 > 1e-12 {
			pot += b.Atoms[i*4+3] / float32(math.Sqrt(float64(r2)))
		}
	}
	return pot
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) verify() error {
	totalPoints := b.GridSide * b.GridSide * b.GridSide
	gpuPot := make([]float32, totalPoints)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], gpuPot); err != nil {
		return err
	}

	cutoff2 := b.Cutoff * b.Cutoff
	side := b.GridSide

	for idx := 0; idx < totalPoints; idx++ {
		gz := idx / (side * side)
		gy := (idx / side) % side
		gx := idx % side
		px := float32(gx) * b.GridSpacing
		py := float32(gy) * b.GridSpacing
		pz := float32(gz) * b.GridSpacing

		ref := b.cpuPotential(px, py, pz, cutoff2)
		got := gpuPot[idx]

		diff := math.Abs(float64(got - ref))
		tol := 1e-3 * (math.Abs(float64(ref)) + 1e-6)
		if diff > tol {
			return fmt.Errorf("At grid[%d] (%d,%d,%d), expected %f, but got %f.\n",
				idx, gx, gy, gz, ref, got)
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(
		b.Readbacks,
		benchmarks.Download(b.rt, b.Queue, b.GPot, 4*(b.GridSide*b.GridSide*b.GridSide)),
	)
}

// Step submits computation, then verification readback, then consumes results.
func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if err := b.ReleaseUploads(b.rt); err != nil {
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
