// Package hotspot implements the Rodinia Hotspot benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/rodinia_hotspot) for the MGPUSim MI300X
// (CDNA3 / gfx942) model.
//
// Hotspot is an iterative 2D stencil thermal simulation: each grid cell's
// temperature is updated from its four neighbors (N/S/E/W), the local power
// density, and thermal resistances. The kernel is launched NumIterations
// times with ping-pong source/destination buffers. The kernel binary is
// compiled for gfx942 only (see native/), so the benchmark must be run with
// `-arch cdna3` (the MI300X configuration).
package hotspot

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

// blockSize is the (constant) thread-block dimension baked into the kernel.
const blockSize = 16

// Rodinia thermal constants.
const (
	ambTemp    = 80.0
	chipHeight = 0.016
	chipWidth  = 0.016
	tChip      = 0.0005
	kSi        = 100.0
	cSi        = 1.75e6
)

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// Verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 48): three 8-byte global_buffer pointers followed
// by six 4-byte by_value scalars, packed with no padding (mgpusim serializes
// args with binary.Write, which does not insert alignment padding). The
// kernel uses a constant block dimension, so no hidden ABI arguments are
// emitted.
type KernelArgs struct {
	TempSrc    gpuruntime.Ptr // offset 0
	TempDst    gpuruntime.Ptr // offset 8
	Power      gpuruntime.Ptr // offset 16
	GridCols   int32          // offset 24
	GridRows   int32          // offset 28
	StepDivCap float32        // offset 32
	Rx1        float32        // offset 36
	Ry1        float32        // offset 40
	Rz1        float32        // offset 44
}

// Benchmark defines the Hotspot benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[checkpointConfig, State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new Hotspot benchmark.
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.checkpointConfig, b.bind)

	b.rt = rt
	b.Context = b.rt.Init()

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "hotspot_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. Hotspot uses a single GPU.
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
		log.Panic("the rodinia hotspot benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.GridSize <= 0 {
		b.GridSize = 32
	}
	if b.NumIterations <= 0 {
		b.NumIterations = 10
	}

	n := b.GridSize
	total := n * n

	b.Temp = make([]float32, total)
	b.Power = make([]float32, total)
	for i := 0; i < total; i++ {
		// Deterministic synthetic init (reproducible in Verify).
		b.Temp[i] = float32(ambTemp) + float32(i%200)/10.0
		b.Power[i] = float32(i%100) / 500.0
	}

	if b.UseUnifiedMemory {
		b.GTempA = b.rt.AllocateUnifiedMemory(b.Context, uint64(total*4))
		b.GTempB = b.rt.AllocateUnifiedMemory(b.Context, uint64(total*4))
		b.GPower = b.rt.AllocateUnifiedMemory(b.Context, uint64(total*4))
	} else {
		b.GTempA = b.rt.AllocateMemory(b.Context, uint64(total*4))
		b.GTempB = b.rt.AllocateMemory(b.Context, uint64(total*4))
		b.GPower = b.rt.AllocateMemory(b.Context, uint64(total*4))
	}

	b.Upload(b.rt, b.Queue, b.GTempA, b.Temp)
	b.Upload(b.rt, b.Queue, b.GPower, b.Power)
}

// thermalParams computes the derived thermal scalars for the current grid.
func (b *Benchmark) thermalParams() (stepDivCap, rx1, ry1, rz1 float32) {
	n := float64(b.GridSize)
	gridHeight := chipHeight / n
	gridWidth := chipWidth / n
	cap := cSi * tChip * gridHeight * gridWidth
	rx := gridWidth / (2.0 * kSi * tChip * gridHeight)
	ry := gridHeight / (2.0 * kSi * tChip * gridWidth)
	rz := tChip / (kSi * gridHeight * gridWidth)
	maxSlope := kSi / (0.5 * tChip * cSi)
	step := 0.001 / maxSlope
	stepDivCap = float32(step / cap)
	rx1 = float32(1.0 / rx)
	ry1 = float32(1.0 / ry)
	rz1 = float32(1.0 / rz)
	return
}

func (b *Benchmark) exec() {
	n := b.GridSize
	gridDim := uint32((n + blockSize - 1) / blockSize)
	globalX := gridDim * blockSize
	globalY := gridDim * blockSize

	stepDivCap, rx1, ry1, rz1 := b.thermalParams()

	src := b.GTempA
	dst := b.GTempB

	for k := 0; k < b.NumIterations; k++ {
		args := KernelArgs{
			TempSrc:    src,
			TempDst:    dst,
			Power:      b.GPower,
			GridCols:   int32(n),
			GridRows:   int32(n),
			StepDivCap: stepDivCap,
			Rx1:        rx1,
			Ry1:        ry1,
			Rz1:        rz1,
		}

		b.rt.EnqueueLaunchKernel(
			b.Queue,
			b.hsaco,
			[3]uint32{globalX, globalY, 1},
			[3]uint16{blockSize, blockSize, 1},
			&args,
		)

		src, dst = dst, src
	}

	// After the loop, src holds the most recent result.
	b.FinalSrc = src
}

// Verify checks the GPU result against a CPU reference computation that
// reproduces the same iterative stencil exactly.
func (b *Benchmark) verify() error { //nolint:funlen,gocognit
	n := b.GridSize
	total := n * n

	gpu := make([]float32, total)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], gpu); err != nil {
		return err
	}

	stepDivCap, rx1, ry1, rz1 := b.thermalParams()

	cur := make([]float32, total)
	next := make([]float32, total)
	copy(cur, b.Temp)

	for k := 0; k < b.NumIterations; k++ {
		for row := 0; row < n; row++ {
			for col := 0; col < n; col++ {
				idx := row*n + col
				tc := cur[idx]

				tn := tc
				if row > 0 {
					tn = cur[(row-1)*n+col]
				}
				ts := tc
				if row < n-1 {
					ts = cur[(row+1)*n+col]
				}
				tw := tc
				if col > 0 {
					tw = cur[row*n+(col-1)]
				}
				te := tc
				if col < n-1 {
					te = cur[row*n+(col+1)]
				}

				delta := stepDivCap *
					(b.Power[idx] +
						(tn+ts-2.0*tc)*ry1 +
						(tw+te-2.0*tc)*rx1 +
						(float32(ambTemp)-tc)*rz1)

				next[idx] = tc + delta
			}
		}
		cur, next = next, cur
	}

	for i := 0; i < total; i++ {
		ref := float64(cur[i])
		got := float64(gpu[i])

		denom := math.Abs(ref)
		if denom < 1.0 {
			denom = 1.0
		}
		if math.Abs(ref-got)/denom > 1e-3 {
			return fmt.Errorf("At cell %d (row %d, col %d), expected %f, but got %f.\n",
				i, i/n, i%n, ref, got)
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.FinalSrc, 4*((b.GridSize)*(b.GridSize))))
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
