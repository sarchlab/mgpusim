// Package stencil implements the Parboil 7-point 3D Jacobi stencil benchmark,
// ported from sarchlab/gpu_benchmarks (tier2/parboil_stencil) for the MGPUSim
// MI300A (CDNA3 / gfx942) model.
//
// A 7-point stencil is applied over an N*N*N grid using two ping-pong buffers.
// Each kernel launch performs one time-step; the benchmark runs a small fixed
// number of time-steps. The kernel binary is compiled for gfx942 only (see
// native/), so the benchmark must be run with `-arch cdna3` (the MI300A
// configuration).
//
// Stencil:
//
//	out[i] = c0*in[i] + c1*(in[i-1] + in[i+1] + in[i-nx] + in[i+nx]
//	                        + in[i-nx*ny] + in[i+nx*ny])
//
// Interior points are updated; the 1-cell boundary remains fixed.
package stencil

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

const blockSize = 16

const (
	c0 float32 = 0.6
	c1 float32 = (1.0 - 0.6) / 6.0
)

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout below is verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 36): two 8-byte global_buffer pointers followed by
// five 4-byte by_value scalars, packed with no padding (mgpusim serializes
// args with binary.Write, which does not insert alignment padding). The kernel
// uses constant block dimensions, so no hidden ABI arguments are emitted.
type KernelArgs struct {
	In  driver.Ptr // offset 0
	Out driver.Ptr // offset 8
	Nx  int32      // offset 16
	Ny  int32      // offset 20
	Nz  int32      // offset 24
	C0  float32    // offset 28
	C1  float32    // offset 32
}

// Benchmark defines the stencil benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch         arch.Type
	N            int // grid dimension (N*N*N)
	NumTimesteps int

	hData []float32 // initial host data
	gA    driver.Ptr
	gB    driver.Ptr

	// finalSrc tracks which device buffer holds the final result after the
	// ping-pong iterations.
	finalSrc driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new stencil benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "stencil3d")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. Stencil uses a single GPU.
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
		log.Panic("the parboil stencil benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.N <= 0 {
		b.N = 32
	}
	if b.NumTimesteps <= 0 {
		b.NumTimesteps = 4
	}

	n := b.N
	total := n * n * n

	// Deterministic host init that is reproduced exactly in Verify().
	b.hData = make([]float32, total)
	for i := 0; i < total; i++ {
		b.hData[i] = float32((i*7919)%1000) / 1000.0
	}

	if b.useUnifiedMemory {
		b.gA = b.driver.AllocateUnifiedMemory(b.context, uint64(total*4))
		b.gB = b.driver.AllocateUnifiedMemory(b.context, uint64(total*4))
	} else {
		b.gA = b.driver.AllocateMemory(b.context, uint64(total*4))
		b.gB = b.driver.AllocateMemory(b.context, uint64(total*4))
	}

	// Both ping-pong buffers start identical (so fixed boundary cells match).
	b.driver.MemCopyH2D(b.context, b.gA, b.hData)
	b.driver.MemCopyH2D(b.context, b.gB, b.hData)
}

func (b *Benchmark) exec() {
	n := b.N
	gridDimX := uint32((n + blockSize - 1) / blockSize)
	gridDimY := uint32((n + blockSize - 1) / blockSize)
	globalX := gridDimX * blockSize
	globalY := gridDimY * blockSize

	// The kernel uses a 2D (x,y) grid and loops over z internally, so the
	// grid's z-dimension is 1 (see native/parboil_stencil.cpp for the reason).
	src := b.gA
	dst := b.gB
	for t := 0; t < b.NumTimesteps; t++ {
		args := KernelArgs{
			In:  src,
			Out: dst,
			Nx:  int32(n),
			Ny:  int32(n),
			Nz:  int32(n),
			C0:  c0,
			C1:  c1,
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

	// After the loop, src holds the most recently written result.
	b.finalSrc = src
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) Verify() {
	n := b.N
	total := n * n * n

	gpuOut := make([]float32, total)
	b.driver.MemCopyD2H(b.context, gpuOut, b.finalSrc)

	// CPU reference: same ping-pong stencil with the same init.
	cur := make([]float32, total)
	next := make([]float32, total)
	copy(cur, b.hData)
	copy(next, b.hData)

	for t := 0; t < b.NumTimesteps; t++ {
		for iz := 0; iz < n; iz++ {
			for iy := 0; iy < n; iy++ {
				for ix := 0; ix < n; ix++ {
					idx := iz*n*n + iy*n + ix
					if ix >= 1 && ix < n-1 &&
						iy >= 1 && iy < n-1 &&
						iz >= 1 && iz < n-1 {
						next[idx] = c0*cur[idx] +
							c1*(cur[idx-1]+
								cur[idx+1]+
								cur[idx-n]+
								cur[idx+n]+
								cur[idx-n*n]+
								cur[idx+n*n])
					}
					// boundary cells: keep the value already in next
				}
			}
		}
		cur, next = next, cur
	}

	for i := 0; i < total; i++ {
		ref := float64(cur[i])
		got := float64(gpuOut[i])

		denom := math.Abs(ref)
		if denom < 1.0 {
			denom = 1.0
		}
		if math.Abs(ref-got)/denom > 1e-3 {
			log.Fatalf("At index %d, expected %f, but got %f.\n",
				i, ref, got)
		}
	}

	log.Printf("Passed!\n")
}
