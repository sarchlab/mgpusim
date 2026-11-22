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
	"encoding/binary"
	"fmt"
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

const blockSize = 128

const calibratedParticles = 100

type Vector struct{ V, X, Y, Z float64 }
type Neighbor struct {
	X, Y, Z, Box int32
	Offset       int64
}
type Box struct {
	X, Y, Z, ID int32
	Offset      int64
	Count       int32
	_           int32
	Neighbors   [26]Neighbor
}

type Dimensions struct {
	CurrentArg, ArchArg, CoresArg, Boxes1D                     int32
	NumberBoxes, BoxBytes, Particles, VectorBytes, ChargeBytes int64
}

// KernelArgs reproduces the ROCm LP64 layout.
type KernelArgs struct {
	Alpha                             float64
	Dim                               Dimensions
	Boxes, Positions, Charges, Forces gpuruntime.Ptr
}

// Benchmark defines the LavaMD benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new LavaMD benchmark.
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
		cdna3HSACOBytes, "kernel_gpu_cuda")
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

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
}

// lcgRand is a simple linear-congruential PRNG matching the HIP host code.
func (b *Benchmark) neighborBoxes(home int) []int {
	size := b.NumBoxes
	x, y, z := home%size, (home/size)%size, home/(size*size)
	result := make([]int, 0, 26)
	for dz := -1; dz <= 1; dz++ {
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				xx, yy, zz := x+dx, y+dy, z+dz
				if (dx == 0 && dy == 0 && dz == 0) || xx < 0 || yy < 0 || zz < 0 || xx >= size || yy >= size || zz >= size {
					continue
				}
				result = append(result, (zz*size+yy)*size+xx)
			}
		}
	}
	return result
}

func (b *Benchmark) initMem() {
	if b.NumBoxes <= 0 {
		b.NumBoxes = 1
	}
	if b.ParticlesPerBox != 0 && b.ParticlesPerBox != calibratedParticles {
		panic("v1.1 LavaMD fixes the number of particles per box at 100")
	}
	n := b.NumBoxes
	boxCount := n * n * n
	particleCount := boxCount * calibratedParticles
	boxes := make([]Box, boxCount)
	for i := range boxes {
		x, y, z := i%n, (i/n)%n, i/(n*n)
		box := &boxes[i]
		box.X, box.Y, box.Z, box.ID = int32(x), int32(y), int32(z), int32(i)
		box.Offset = int64(i * calibratedParticles)
		neighbors := b.neighborBoxes(i)
		box.Count = int32(len(neighbors))
		for j, other := range neighbors {
			box.Neighbors[j] = Neighbor{
				X: int32(other % n), Y: int32((other / n) % n), Z: int32(other / (n * n)),
				Box: int32(other), Offset: int64(other * calibratedParticles)}
		}
	}
	b.Positions = make([]Vector, particleCount)
	b.Charges = make([]float64, particleCount)
	rand := benchmarks.NewGlibcRand(42)
	decimal := func() float64 { return float64(rand.Next()%10+1) / 10.0 }
	for i := range b.Positions {
		b.Positions[i] = Vector{decimal(), decimal(), decimal(), decimal()}
	}
	for i := range b.Charges {
		b.Charges[i] = decimal()
	}
	b.Dim = Dimensions{Boxes1D: int32(n), NumberBoxes: int64(boxCount), BoxBytes: int64(binary.Size(boxes)),
		Particles: int64(particleCount), VectorBytes: int64(particleCount * 32), ChargeBytes: int64(particleCount * 8)}
	alloc := b.rt.AllocateMemory
	if b.UseUnifiedMemory {
		alloc = b.rt.AllocateUnifiedMemory
	}
	b.GBoxes = alloc(b.Context, uint64(b.Dim.BoxBytes))
	b.GPositions = alloc(b.Context, uint64(b.Dim.VectorBytes))
	b.GCharges = alloc(b.Context, uint64(b.Dim.ChargeBytes))
	b.GForces = alloc(b.Context, uint64(b.Dim.VectorBytes))
	b.Upload(b.rt, b.Queue, b.GBoxes, boxes)
	b.Upload(b.rt, b.Queue, b.GPositions, b.Positions)
	b.Upload(b.rt, b.Queue, b.GCharges, b.Charges)
	b.Upload(b.rt, b.Queue, b.GForces, make([]Vector, particleCount))
}

const alpha = 0.5

func (b *Benchmark) exec() {
	args := KernelArgs{
		Alpha: alpha, Dim: b.Dim, Boxes: b.GBoxes, Positions: b.GPositions,
		Charges: b.GCharges, Forces: b.GForces,
	}
	grid := [3]uint32{uint32(b.Dim.NumberBoxes) * blockSize, 1, 1}
	group := [3]uint16{blockSize, 1, 1}
	b.rt.EnqueueLaunchKernel(b.Queue, b.hsaco, grid, group, &args)
}

func (b *Benchmark) verify() error {
	actual := make([]Vector, len(b.Positions))
	if err := benchmarks.Read(b.rt, b.Readbacks[0], actual); err != nil {
		return err
	}
	for box := 0; box < int(b.Dim.NumberBoxes); box++ {
		neighbors := append([]int{box}, b.neighborBoxes(box)...)
		for i := 0; i < calibratedParticles; i++ {
			index := box*calibratedParticles + i
			position := b.Positions[index]
			var want Vector
			for _, otherBox := range neighbors {
				start := otherBox * calibratedParticles
				for j := start; j < start+calibratedParticles; j++ {
					other := b.Positions[j]
					r2 := position.V + other.V - (position.X*other.X + position.Y*other.Y + position.Z*other.Z)
					value := math.Exp(-2 * alpha * alpha * r2)
					force, charge := 2*value, b.Charges[j]
					want.V += charge * value
					want.X += charge * (force * (position.X - other.X))
					want.Y += charge * (force * (position.Y - other.Y))
					want.Z += charge * (force * (position.Z - other.Z))
				}
			}
			got := actual[index]
			for component, value := range []float64{got.V, got.X, got.Y, got.Z} {
				ref := []float64{want.V, want.X, want.Y, want.Z}[component]
				if math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value-ref) > 1e-10*math.Max(1, math.Abs(ref)) {
					return fmt.Errorf("particle %d component %d: got %g, expected %g", index, component, value, ref)
				}
			}
		}
	}
	log.Printf("Passed!\n")
	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GForces, int(b.Dim.VectorBytes)))
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
