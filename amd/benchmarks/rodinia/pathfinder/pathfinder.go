// Package pathfinder implements the Rodinia PathFinder benchmark, ported
// from sarchlab/gpu_benchmarks (tier2/rodinia_pathfinder) for the MGPUSim
// MI300X (CDNA3 / gfx942) model.
//
// PathFinder is a dynamic-programming sweep that finds the minimum-cost path
// through a 2D grid of weights from the top row to the bottom row. Each row
// is processed by one kernel launch using the previous row's costs; the
// source/destination row buffers are double-buffered (ping-pong) across the
// Rows iterations. The kernel binary is compiled for gfx942 only (see
// native/), so the benchmark must be run with `-arch cdna3` (MI300X).
package pathfinder

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
	GPUWall gpuruntime.Ptr // offset 0
	GPUSrc  gpuruntime.Ptr // offset 8
	GPUDst  gpuruntime.Ptr // offset 16
	Cols    int32          // offset 24
	T       int32          // offset 28
}

// Benchmark defines the PathFinder benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[checkpointConfig, State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new PathFinder benchmark.
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
		cdna3HSACOBytes, "dynproc_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. PathFinder uses a single GPU.
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
		log.Panic("the rodinia pathfinder benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
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

	b.Wall = genWall(rows, cols)

	wallBytes := uint64(rows * cols * 4)
	rowBytes := uint64(cols * 4)

	if b.UseUnifiedMemory {
		b.GWall = b.rt.AllocateUnifiedMemory(b.Context, wallBytes)
		b.GBuf[0] = b.rt.AllocateUnifiedMemory(b.Context, rowBytes)
		b.GBuf[1] = b.rt.AllocateUnifiedMemory(b.Context, rowBytes)
	} else {
		b.GWall = b.rt.AllocateMemory(b.Context, wallBytes)
		b.GBuf[0] = b.rt.AllocateMemory(b.Context, rowBytes)
		b.GBuf[1] = b.rt.AllocateMemory(b.Context, rowBytes)
	}

	b.Upload(b.rt, b.Queue, b.GWall, b.Wall)

	// Initialize src buffer (gBuf[0]) with row 0 of the wall.
	row0 := make([]int32, cols)
	copy(row0, b.Wall[:cols])
	b.Upload(b.rt, b.Queue, b.GBuf[0], row0)
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
			GPUWall: b.GWall,
			GPUSrc:  b.GBuf[src],
			GPUDst:  b.GBuf[dst],
			Cols:    int32(cols),
			T:       int32(t),
		}

		b.rt.EnqueueLaunchKernel(
			b.Queue,
			b.hsaco,
			[3]uint32{globalX, 1, 1},
			[3]uint16{blockSize, 1, 1},
			&args,
		)

		src = dst
	}

	// After the loop, the most recent result is in b.gBuf[src].
	b.FinalGBuf = b.GBuf[src]
}

// Verify checks the GPU result against a CPU reference computation that
// reproduces the exact double-buffered sweep performed on the GPU.
func (b *Benchmark) verify() error {
	cols := b.Cols
	rows := b.Rows

	gpuResult := make([]int32, cols)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], gpuResult); err != nil {
		return err
	}

	src := make([]int32, cols)
	dst := make([]int32, cols)
	copy(src, b.Wall[:cols])

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

			dst[c] = b.Wall[t*cols+c] + min3
		}
		src, dst = dst, src
	}

	for c := 0; c < cols; c++ {
		if gpuResult[c] != src[c] {
			return fmt.Errorf("At column %d, expected %d, but got %d.\n",
				c, src[c], gpuResult[c])
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.FinalGBuf, 4*(b.Cols)))
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
