// Package pathfinder implements the Rodinia PathFinder benchmark, ported
// from sarchlab/gpu_benchmarks (tier2/rodinia_pathfinder) for the MGPUSim
// MI300X (CDNA3 / gfx942) model.
//
// PathFinder is a dynamic-programming sweep that finds the minimum-cost path
// through a 2D grid of weights from the top row to the bottom row. Work-groups use LDS to update a tile of
// multiple rows per launch, with overlapping halos between adjacent tiles. The kernel binary is compiled for gfx942 only (see
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

// KernelArgs is the ABI of the tiled pathfinder kernel.
type KernelArgs struct {
	Iterations                    int32
	_                             uint32
	GPUWall, GPUSrc, GPUDst       gpuruntime.Ptr
	Cols, Rows, StartStep, Border int32
}

// Benchmark defines the PathFinder benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new PathFinder benchmark.
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
	rand := benchmarks.NewGlibcRand(9)
	for i := range wall {
		wall[i] = int32(rand.Next() % 10)
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

	alloc := b.rt.AllocateMemory
	if b.UseUnifiedMemory {
		alloc = b.rt.AllocateUnifiedMemory
	}
	rowBytes := uint64(cols * 4)
	b.GBuf[0] = alloc(b.Context, rowBytes)
	b.GBuf[1] = alloc(b.Context, rowBytes)
	if rows > 1 {
		b.GWall = alloc(b.Context, uint64((rows-1)*cols*4))
		b.Upload(b.rt, b.Queue, b.GWall, b.Wall[cols:])
	}
	b.Upload(b.rt, b.Queue, b.GBuf[0], b.Wall[:cols])
}

func (b *Benchmark) exec() {
	height := b.PyramidHeight
	if height == 0 {
		height = 20
	}
	if height < 1 || height >= blockSize/2 {
		panic("pathfinder pyramid height must be positive and less than half the block size")
	}
	cols, rows := b.Cols, b.Rows
	tileWidth := blockSize - 2*height
	groups := (cols + tileWidth - 1) / tileWidth
	grid := [3]uint32{uint32(groups * blockSize), 1, 1}
	group := [3]uint16{blockSize, 1, 1}
	src, dst := 0, 1
	for t := 0; t < rows-1; t += height {
		args := KernelArgs{Iterations: int32(min(height, rows-1-t)),
			GPUWall: b.GWall, GPUSrc: b.GBuf[src], GPUDst: b.GBuf[dst],
			Cols: int32(cols), Rows: int32(rows), StartStep: int32(t), Border: int32(height)}
		b.rt.EnqueueLaunchKernel(b.Queue, b.hsaco, grid, group, &args)
		src, dst = dst, src
	}
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
