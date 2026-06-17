// Package pathfinder implements the Rodinia PathFinder benchmark, ported
// from sarchlab/gpu_benchmarks (tier2/rodinia_pathfinder) for the MGPUSim
// MI300A (CDNA3 / gfx942) model.
//
// PathFinder is a dynamic-programming sweep that finds the minimum-cost path
// through a 2D grid of weights from the top row to the bottom row. Each row
// is processed by one kernel launch using the previous row's costs; the
// source/destination row buffers are double-buffered (ping-pong) across the
// Rows iterations. The kernel binary is compiled for gfx942 only (see
// native/), so the benchmark must be run with `-arch cdna3` (MI300A).
package pathfinder

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// blockSize matches the constant BLOCK_SIZE in native/rodinia_pathfinder.cpp.
const blockSize = 256

// intMax mirrors INT_MAX used as the out-of-bounds sentinel in the kernel.
const intMax int32 = math.MaxInt32

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout is verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 32): three 8-byte global_buffer pointers followed
// by two 4-byte by_value scalars, packed with no padding (mgpusim serializes
// args with binary.Write, which does not insert alignment padding). The
// kernel uses a constant block size and reads only blockIdx/threadIdx, so no
// hidden ABI arguments are emitted.
type KernelArgs struct {
	GPUWall driver.Ptr // offset 0
	GPUSrc  driver.Ptr // offset 8
	GPUDst  driver.Ptr // offset 16
	Cols    int32      // offset 24
	T       int32      // offset 28
}

// Benchmark defines the PathFinder benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch arch.Type
	Rows int
	Cols int

	wall      []int32
	gWall     driver.Ptr
	gBuf      [2]driver.Ptr
	finalGBuf driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new PathFinder benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "dynproc_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. PathFinder uses a single GPU.
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
		log.Panic("the rodinia pathfinder benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

// genWall produces a deterministic wall (cost grid) reproducible in Verify().
func genWall(rows, cols int) []int32 {
	wall := make([]int32, rows*cols)
	for i := range wall {
		// Simple deterministic LCG-style hash into [0, 10).
		wall[i] = int32((i*1103515245 + 12345) & 0x7fffffff % 10)
	}
	return wall
}

func (b *Benchmark) initMem() {
	if b.Rows <= 0 {
		b.Rows = 64
	}
	if b.Cols <= 0 {
		b.Cols = 128
	}

	rows := b.Rows
	cols := b.Cols

	b.wall = genWall(rows, cols)

	wallBytes := uint64(rows * cols * 4)
	rowBytes := uint64(cols * 4)

	if b.useUnifiedMemory {
		b.gWall = b.driver.AllocateUnifiedMemory(b.context, wallBytes)
		b.gBuf[0] = b.driver.AllocateUnifiedMemory(b.context, rowBytes)
		b.gBuf[1] = b.driver.AllocateUnifiedMemory(b.context, rowBytes)
	} else {
		b.gWall = b.driver.AllocateMemory(b.context, wallBytes)
		b.gBuf[0] = b.driver.AllocateMemory(b.context, rowBytes)
		b.gBuf[1] = b.driver.AllocateMemory(b.context, rowBytes)
	}

	b.driver.MemCopyH2D(b.context, b.gWall, b.wall)

	// Initialize src buffer (gBuf[0]) with row 0 of the wall.
	row0 := make([]int32, cols)
	copy(row0, b.wall[:cols])
	b.driver.MemCopyH2D(b.context, b.gBuf[0], row0)
}

func (b *Benchmark) exec() {
	cols := b.Cols
	rows := b.Rows

	gridDim := uint32((cols + blockSize - 1) / blockSize)
	globalX := gridDim * blockSize

	src := 0 // index into b.gBuf
	for t := 1; t < rows; t++ {
		dst := 1 - src

		args := KernelArgs{
			GPUWall: b.gWall,
			GPUSrc:  b.gBuf[src],
			GPUDst:  b.gBuf[dst],
			Cols:    int32(cols),
			T:       int32(t),
		}

		b.driver.EnqueueLaunchKernel(
			b.queue,
			b.hsaco,
			[3]uint32{globalX, 1, 1},
			[3]uint16{blockSize, 1, 1},
			&args,
		)

		src = dst
	}

	b.driver.DrainCommandQueue(b.queue)

	// After the loop, the most recent result is in b.gBuf[src].
	b.finalGBuf = b.gBuf[src]
}

// Verify checks the GPU result against a CPU reference computation that
// reproduces the exact double-buffered sweep performed on the GPU.
func (b *Benchmark) Verify() {
	cols := b.Cols
	rows := b.Rows

	gpuResult := make([]int32, cols)
	b.driver.MemCopyD2H(b.context, gpuResult, b.finalGBuf)

	src := make([]int32, cols)
	dst := make([]int32, cols)
	copy(src, b.wall[:cols])

	for t := 1; t < rows; t++ {
		for c := 0; c < cols; c++ {
			left := intMax
			if c > 0 {
				left = src[c-1]
			}
			above := src[c]
			right := intMax
			if c < cols-1 {
				right = src[c+1]
			}

			min3 := left
			if above < min3 {
				min3 = above
			}
			if right < min3 {
				min3 = right
			}

			dst[c] = b.wall[t*cols+c] + min3
		}
		src, dst = dst, src
	}

	for c := 0; c < cols; c++ {
		if gpuResult[c] != src[c] {
			log.Fatalf("At column %d, expected %d, but got %d.\n",
				c, src[c], gpuResult[c])
		}
	}

	log.Printf("Passed!\n")
}
