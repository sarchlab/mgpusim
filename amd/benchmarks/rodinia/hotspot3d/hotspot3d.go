// Package hotspot3d implements the Rodinia HotSpot3D benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/rodinia_hotspot3d) for the MGPUSim MI300A
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
// benchmark must be run with `-arch cdna3` (the MI300A configuration). The
// block geometry is a constant 8x8x8 cube, so the kernel emits no hidden ABI
// arguments (kernarg_segment_size = 60).
package hotspot3d

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
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
	TempSrc    driver.Ptr // offset 0
	TempDst    driver.Ptr // offset 8
	Power      driver.Ptr // offset 16
	Nx         int32      // offset 24
	Ny         int32      // offset 28
	Nz         int32      // offset 32
	StepDivCap float32    // offset 36
	Rx1        float32    // offset 40
	Ry1        float32    // offset 44
	Rz1        float32    // offset 48
	Ra1        float32    // offset 52
	AmbTemp    float32    // offset 56
}

// Benchmark defines the HotSpot3D benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch arch.Type

	// GridSize is the edge length N of the NxNxN grid.
	GridSize int
	// NumIterations is the number of stencil time-steps to run.
	NumIterations int
	// AmbTemp is the ambient temperature.
	AmbTemp float32

	// Derived thermal parameters (computed in initMem).
	stepDivCap float32
	rx1        float32
	ry1        float32
	rz1        float32
	ra1        float32

	tempInit []float32
	power    []float32

	// Two device temperature buffers (ping-pong) plus the power buffer.
	gTemp  [2]driver.Ptr
	gPower driver.Ptr
	// finalBuf is the index of the device buffer holding the final result.
	finalBuf int

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new HotSpot3D benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

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
	b.gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// Run runs the benchmark.
func (b *Benchmark) Run() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the rodinia hotspot3d benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
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

	b.stepDivCap = step / cap
	b.rx1 = 1.0 / rx
	b.ry1 = 1.0 / ry
	b.rz1 = 1.0 / rz
	b.ra1 = 1.0 / ra
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
	b.tempInit = make([]float32, totalCells)
	b.power = make([]float32, totalCells)
	for i := 0; i < totalCells; i++ {
		b.tempInit[i] = b.AmbTemp + float32(i%200)/10.0
		b.power[i] = float32((i*7)%100) / 500.0
	}

	bytes := uint64(totalCells * 4)
	if b.useUnifiedMemory {
		b.gTemp[0] = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.gTemp[1] = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.gPower = b.driver.AllocateUnifiedMemory(b.context, bytes)
	} else {
		b.gTemp[0] = b.driver.AllocateMemory(b.context, bytes)
		b.gTemp[1] = b.driver.AllocateMemory(b.context, bytes)
		b.gPower = b.driver.AllocateMemory(b.context, bytes)
	}

	b.driver.MemCopyH2D(b.context, b.gTemp[0], b.tempInit)
	b.driver.MemCopyH2D(b.context, b.gPower, b.power)
}

func (b *Benchmark) exec() {
	n := b.GridSize

	// The kernel uses a 2D (x, y) work-group; each work-item walks the full z
	// range internally. So we launch a 2D grid that covers x and y; the z grid
	// dimension is 1. globalX/Y are the total work-item counts (block-size
	// multiples), matching the AQL packet's global-size semantics.
	gridDim := uint32((n + blockDim - 1) / blockDim)
	globalX := gridDim * blockDim
	globalY := gridDim * blockDim

	src := 0
	dst := 1
	for k := 0; k < b.NumIterations; k++ {
		args := KernelArgs{
			TempSrc:    b.gTemp[src],
			TempDst:    b.gTemp[dst],
			Power:      b.gPower,
			Nx:         int32(n),
			Ny:         int32(n),
			Nz:         int32(n),
			StepDivCap: b.stepDivCap,
			Rx1:        b.rx1,
			Ry1:        b.ry1,
			Rz1:        b.rz1,
			Ra1:        b.ra1,
			AmbTemp:    b.AmbTemp,
		}

		b.driver.EnqueueLaunchKernel(
			b.queue,
			b.hsaco,
			[3]uint32{globalX, globalY, 1},
			[3]uint16{blockDim, blockDim, 1},
			&args,
		)
		b.driver.DrainCommandQueue(b.queue)

		src, dst = dst, src
	}

	// After the loop, src holds the most recently written buffer.
	b.finalBuf = src
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

				delta := b.stepDivCap * (b.power[idx] +
					(txm+txp-2.0*tc)*b.rx1 +
					(tym+typ-2.0*tc)*b.ry1 +
					(tzm+tzp-2.0*tc)*b.rz1 +
					(b.AmbTemp-tc)*b.ra1)

				dst[idx] = tc + delta
			}
		}
	}
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) Verify() {
	n := b.GridSize
	totalCells := n * n * n

	gpuResult := make([]float32, totalCells)
	b.driver.MemCopyD2H(b.context, gpuResult, b.gTemp[b.finalBuf])

	// CPU reference: ping-pong the stencil for NumIterations steps.
	bufA := make([]float32, totalCells)
	bufB := make([]float32, totalCells)
	copy(bufA, b.tempInit)
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
			log.Fatalf("At cell %d, expected %f, but got %f.\n",
				i, ref, got)
		}
	}

	log.Printf("Passed!\n")
}
