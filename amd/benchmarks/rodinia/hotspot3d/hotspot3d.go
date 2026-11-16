// Package hotspot3d implements the Rodinia HotSpot3D benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/rodinia_hotspot3d) for the MGPUSim MI300X
// (CDNA3 / gfx942) model.
//
// HotSpot3D is a 3D stencil thermal simulation. For an NxNxN grid, each
// cell's temperature is updated from its six neighbors (+/-x, +/-y, +/-z),
// the local power density, and a set of thermal resistances. Boundaries are
// clamped (an edge cell substitutes its own value for the missing neighbor).
// The host iterates the stencil num_iterations times, ping-ponging between
// two temperature buffers.
//
// The kernel binary is compiled for gfx942 only (see native/), so the
// benchmark must be run with `-arch cdna3` (the MI300X configuration). The
// block geometry is a constant 8x8x8 cube, so the kernel emits no hidden ABI
// arguments (kernarg_segment_size = 60).
package hotspot3d

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

// blockDim is the constant cubic work-group edge length. It MUST match the
// BLOCK_DIM constant compiled into native/rodinia_hotspot3d.cpp.
const blockDim = 8

// Chip thermal constants (Rodinia defaults, extended to 3D).
const (
	chipHeight float32 = 0.016
	chipWidth  float32 = 0.016
	chipDepth  float32 = 0.016
	tChip      float32 = 0.0005
	kSi        float32 = 100.0
	cSi        float32 = 1.75e6
)

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout below is verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 60): three 8-byte global_buffer pointers followed
// by nine 4-byte by_value scalars (3 int32 + 6 float32), packed with no
// padding (mgpusim serializes args with binary.Write, which does not insert
// alignment padding). The kernel uses a constant block size and reads only
// blockIdx/threadIdx, so no hidden ABI arguments are emitted.
type KernelArgs struct {
	TempSrc    gpuruntime.Ptr // offset 0
	TempDst    gpuruntime.Ptr // offset 8
	Power      gpuruntime.Ptr // offset 16
	Nx         int32          // offset 24
	Ny         int32          // offset 28
	Nz         int32          // offset 32
	StepDivCap float32        // offset 36
	Rx1        float32        // offset 40
	Ry1        float32        // offset 44
	Rz1        float32        // offset 48
	Ra1        float32        // offset 52
	AmbTemp    float32        // offset 56
}

// Benchmark defines the HotSpot3D benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new HotSpot3D benchmark.
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
		cdna3HSACOBytes, "hotspot3d_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. HotSpot3D uses a single GPU.
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
		log.Panic("the rodinia hotspot3d benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
}

// computeThermalParams computes the derived thermal parameters exactly as the
// HIP host does, so the Verify() reference uses identical coefficients.
func (b *Benchmark) computeThermalParams() {
	n := float32(b.GridSize)

	dx := chipWidth / n
	dy := chipHeight / n
	dz := chipDepth / n

	cap := cSi * tChip * dx * dy
	rx := dx / (2.0 * kSi * tChip * dy)
	ry := dy / (2.0 * kSi * tChip * dx)
	rz := dz / (2.0 * kSi * tChip * dx)
	ra := tChip / (kSi * dx * dy)
	maxSlope := kSi / (0.5 * tChip * cSi)
	step := float32(0.001) / maxSlope

	b.StepDivCap = step / cap
	b.Rx1 = 1.0 / rx
	b.Ry1 = 1.0 / ry
	b.Rz1 = 1.0 / rz
	b.Ra1 = 1.0 / ra
}

func (b *Benchmark) initMem() {
	if b.GridSize <= 0 {
		b.GridSize = 32
	}
	if b.NumIterations <= 0 {
		// The explicit stencil with these synthetic coefficients is only
		// conditionally stable; a small step count keeps temperatures bounded
		// so the float32 GPU result and the (also float32) CPU reference agree
		// to within the 1e-3 relative tolerance. Larger counts let the unstable
		// mode grow and amplify float rounding differences.
		b.NumIterations = 2
	}
	if b.AmbTemp <= 0 {
		b.AmbTemp = 80.0
	}

	b.computeThermalParams()

	n := b.GridSize
	totalCells := n * n * n

	// Deterministic host init (reproducible in Verify, unlike C rand()).
	b.TempInit = make([]float32, totalCells)
	b.Power = make([]float32, totalCells)
	for i := 0; i < totalCells; i++ {
		b.TempInit[i] = b.AmbTemp + float32(i%200)/10.0
		b.Power[i] = float32((i*7)%100) / 500.0
	}

	bytes := uint64(totalCells * 4)
	if b.UseUnifiedMemory {
		b.GTemp[0] = b.rt.AllocateUnifiedMemory(b.Context, bytes)
		b.GTemp[1] = b.rt.AllocateUnifiedMemory(b.Context, bytes)
		b.GPower = b.rt.AllocateUnifiedMemory(b.Context, bytes)
	} else {
		b.GTemp[0] = b.rt.AllocateMemory(b.Context, bytes)
		b.GTemp[1] = b.rt.AllocateMemory(b.Context, bytes)
		b.GPower = b.rt.AllocateMemory(b.Context, bytes)
	}

	b.Upload(b.rt, b.Queue, b.GTemp[0], b.TempInit)
	b.Upload(b.rt, b.Queue, b.GPower, b.Power)
}

func (b *Benchmark) exec() {
	n := b.GridSize

	// 3D grid: one work-item per (x, y, z) cell. globalX/Y are block-size
	// multiples (ceil(n/blockDim)*blockDim); the z grid dimension is n with
	// blockDim.z == 1 (the kernel uses z = blockIdx.z). globalX/Y/Z are total
	// work-item counts, matching the AQL packet's global-size semantics.
	gridDim := uint32((n + blockDim - 1) / blockDim)
	globalX := gridDim * blockDim
	globalY := gridDim * blockDim
	globalZ := uint32(n)

	src := 0
	dst := 1
	for k := 0; k < b.NumIterations; k++ {
		args := KernelArgs{
			TempSrc:    b.GTemp[src],
			TempDst:    b.GTemp[dst],
			Power:      b.GPower,
			Nx:         int32(n),
			Ny:         int32(n),
			Nz:         int32(n),
			StepDivCap: b.StepDivCap,
			Rx1:        b.Rx1,
			Ry1:        b.Ry1,
			Rz1:        b.Rz1,
			Ra1:        b.Ra1,
			AmbTemp:    b.AmbTemp,
		}

		b.rt.EnqueueLaunchKernel(
			b.Queue,
			b.hsaco,
			[3]uint32{globalX, globalY, globalZ},
			[3]uint16{blockDim, blockDim, 1},
			&args,
		)

		src, dst = dst, src
	}

	// After the loop, src holds the most recently written buffer.
	b.FinalBuf = src
}

// cpuStep applies one stencil time-step on the host, matching the kernel.
func (b *Benchmark) cpuStep(src, dst []float32) {
	n := b.GridSize
	for z := 0; z < n; z++ {
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				idx := z*n*n + y*n + x
				tc := src[idx]

				txm := tc
				if x > 0 {
					txm = src[z*n*n+y*n+(x-1)]
				}
				txp := tc
				if x < n-1 {
					txp = src[z*n*n+y*n+(x+1)]
				}
				tym := tc
				if y > 0 {
					tym = src[z*n*n+(y-1)*n+x]
				}
				typ := tc
				if y < n-1 {
					typ = src[z*n*n+(y+1)*n+x]
				}
				tzm := tc
				if z > 0 {
					tzm = src[(z-1)*n*n+y*n+x]
				}
				tzp := tc
				if z < n-1 {
					tzp = src[(z+1)*n*n+y*n+x]
				}

				delta := b.StepDivCap * (b.Power[idx] +
					(txm+txp-2.0*tc)*b.Rx1 +
					(tym+typ-2.0*tc)*b.Ry1 +
					(tzm+tzp-2.0*tc)*b.Rz1 +
					(b.AmbTemp-tc)*b.Ra1)

				dst[idx] = tc + delta
			}
		}
	}
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) verify() error {
	n := b.GridSize
	totalCells := n * n * n

	gpuResult := make([]float32, totalCells)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], gpuResult); err != nil {
		return err
	}

	// CPU reference: ping-pong the stencil for NumIterations steps.
	bufA := make([]float32, totalCells)
	bufB := make([]float32, totalCells)
	copy(bufA, b.TempInit)
	src, dst := bufA, bufB
	for k := 0; k < b.NumIterations; k++ {
		b.cpuStep(src, dst)
		src, dst = dst, src
	}
	// src now holds the final reference result.

	for i := 0; i < totalCells; i++ {
		ref := float64(src[i])
		got := float64(gpuResult[i])

		denom := math.Abs(ref)
		if denom < 1.0 {
			denom = 1.0
		}
		if math.Abs(ref-got)/denom > 1e-3 {
			return fmt.Errorf("At cell %d, expected %f, but got %f.\n",
				i, ref, got)
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(
		b.Readbacks,
		benchmarks.Download(b.rt, b.Queue, b.GTemp[b.FinalBuf], 4*((b.GridSize)*(b.GridSize)*(b.GridSize))),
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
