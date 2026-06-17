// Package hotspot implements the Rodinia Hotspot benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/rodinia_hotspot) for the MGPUSim MI300A
// (CDNA3 / gfx942) model.
//
// Hotspot is an iterative 2D stencil thermal simulation: each grid cell's
// temperature is updated from its four neighbors (N/S/E/W), the local power
// density, and thermal resistances. The kernel is launched NumIterations
// times with ping-pong source/destination buffers. The kernel binary is
// compiled for gfx942 only (see native/), so the benchmark must be run with
// `-arch cdna3` (the MI300A configuration).
package hotspot

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
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
	TempSrc    driver.Ptr // offset 0
	TempDst    driver.Ptr // offset 8
	Power      driver.Ptr // offset 16
	GridCols   int32      // offset 24
	GridRows   int32      // offset 28
	StepDivCap float32    // offset 32
	Rx1        float32    // offset 36
	Ry1        float32    // offset 40
	Rz1        float32    // offset 44
}

// Benchmark defines the Hotspot benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch          arch.Type
	GridSize      int // grid is GridSize x GridSize
	NumIterations int // stencil time-steps per run

	temp  []float32 // initial temperature grid
	power []float32 // power density grid

	gTempA driver.Ptr
	gTempB driver.Ptr
	gPower driver.Ptr

	// which buffer holds the final result after the iteration loop
	finalSrc driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new Hotspot benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

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
	b.gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// Run runs the benchmark.
func (b *Benchmark) Run() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the rodinia hotspot benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
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

	b.temp = make([]float32, total)
	b.power = make([]float32, total)
	for i := 0; i < total; i++ {
		// Deterministic synthetic init (reproducible in Verify).
		b.temp[i] = float32(ambTemp) + float32(i%200)/10.0
		b.power[i] = float32(i%100) / 500.0
	}

	if b.useUnifiedMemory {
		b.gTempA = b.driver.AllocateUnifiedMemory(b.context, uint64(total*4))
		b.gTempB = b.driver.AllocateUnifiedMemory(b.context, uint64(total*4))
		b.gPower = b.driver.AllocateUnifiedMemory(b.context, uint64(total*4))
	} else {
		b.gTempA = b.driver.AllocateMemory(b.context, uint64(total*4))
		b.gTempB = b.driver.AllocateMemory(b.context, uint64(total*4))
		b.gPower = b.driver.AllocateMemory(b.context, uint64(total*4))
	}

	b.driver.MemCopyH2D(b.context, b.gTempA, b.temp)
	b.driver.MemCopyH2D(b.context, b.gPower, b.power)
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

	src := b.gTempA
	dst := b.gTempB

	for k := 0; k < b.NumIterations; k++ {
		args := KernelArgs{
			TempSrc:    src,
			TempDst:    dst,
			Power:      b.gPower,
			GridCols:   int32(n),
			GridRows:   int32(n),
			StepDivCap: stepDivCap,
			Rx1:        rx1,
			Ry1:        ry1,
			Rz1:        rz1,
		}

		b.driver.EnqueueLaunchKernel(
			b.queue,
			b.hsaco,
			[3]uint32{globalX, globalY, 1},
			[3]uint16{blockSize, blockSize, 1},
			&args,
		)
		b.driver.DrainCommandQueue(b.queue)

		src, dst = dst, src
	}

	// After the loop, src holds the most recent result.
	b.finalSrc = src
}

// Verify checks the GPU result against a CPU reference computation that
// reproduces the same iterative stencil exactly.
func (b *Benchmark) Verify() { //nolint:funlen,gocognit
	n := b.GridSize
	total := n * n

	gpu := make([]float32, total)
	b.driver.MemCopyD2H(b.context, gpu, b.finalSrc)

	stepDivCap, rx1, ry1, rz1 := b.thermalParams()

	cur := make([]float32, total)
	next := make([]float32, total)
	copy(cur, b.temp)

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
					(b.power[idx] +
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
			log.Fatalf("At cell %d (row %d, col %d), expected %f, but got %f.\n",
				i, i/n, i%n, ref, got)
		}
	}

	log.Printf("Passed!\n")
}
