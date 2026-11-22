// Package conv3d implements the PolyBench 3D Convolution benchmark, ported
// from sarchlab/gpu_benchmarks (tier2/polybench_3dconv) for the MGPUSim
// MI300X (CDNA3 / gfx942) model.
//
// It convolves an NxNxN float volume with a small
// filter_size x filter_size x filter_size 3D filter, one work-item per
// output element. The kernel binary is compiled for gfx942 only (see
// native/), so the benchmark must be run with `-arch cdna3` (the MI300X
// configuration).
package conv3d

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

// blockSize is the constant work-group edge length, matching BLOCK_SIZE in
// native/polybench_3dconv.cpp. Because the kernel uses this compile-time
// constant instead of blockDim, the compiler emits no hidden ABI arguments.
const blockSize = 8

// KernelArgs is the v1.1 fixed stencil ABI.
type KernelArgs struct {
	Input, Output gpuruntime.Ptr
	N             int32
	_             uint32
	Launch        benchmarks.GFX942LaunchDimensions
}

// Benchmark defines the 3D convolution benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new 3D convolution benchmark.
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
		cdna3HSACOBytes, "conv3d_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. conv3d uses a single GPU.
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
		log.Panic("the polybench 3dconv benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.N <= 0 {
		b.N = 32
	}
	n := b.N
	if n < 3 {
		panic("volume must have at least three elements per side")
	}
	if b.FilterSize != 0 && b.FilterSize != 3 {
		panic("the v1.1 convolution uses a fixed three-point spatial extent")
	}
	volume := n * n * n
	b.Input = make([]float32, volume)
	rand := benchmarks.NewGlibcRand(42)
	for i := range b.Input {
		b.Input[i] = float32(rand.Next()%100) / 100.0
	}
	alloc := b.rt.AllocateMemory
	if b.UseUnifiedMemory {
		alloc = b.rt.AllocateUnifiedMemory
	}
	b.GInput = alloc(b.Context, uint64(volume*4))
	b.GOutput = alloc(b.Context, uint64(volume*4))
	b.Upload(b.rt, b.Queue, b.GInput, b.Input)
}

func (b *Benchmark) exec() {
	edge := b.BlockSize
	if edge == 0 {
		edge = blockSize
	}
	if edge < 1 || edge > 10 {
		panic("convolution block edge must be between 1 and 10")
	}
	global := uint32((b.N + edge - 1) / edge * edge)
	grid := [3]uint32{global, global, global}
	group := [3]uint16{uint16(edge), uint16(edge), uint16(edge)}
	args := KernelArgs{Input: b.GInput, Output: b.GOutput, N: int32(b.N),
		Launch: benchmarks.NewGFX942LaunchDimensions(grid, group)}
	b.rt.EnqueueLaunchKernel(b.Queue, b.hsaco, grid, group, &args)
}

// Verify uses independent coefficients with duplicate source offsets combined.
func (b *Benchmark) verify() error {
	n := b.N
	output := make([]float32, n*n*n)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], output); err != nil {
		return err
	}
	offsets := [][3]int{{-1, -1, -1}, {1, -1, -1}, {0, -1, 0}, {0, 0, 0}, {0, 1, 0},
		{-1, -1, 1}, {1, -1, 1}, {-1, 0, 1}, {1, 0, 1}, {-1, 1, 1}, {1, 1, 1}}
	weights := []float64{-1, 21, -3, 6, -9, 2, 4, 5, 7, -8, 10}
	magnitude := []float64{15, 21, 3, 6, 9, 2, 4, 5, 7, 8, 10}
	for idx, actual := range output {
		i, j, k := idx/(n*n), idx/n%n, idx%n
		var expected, tol float64
		if i > 0 && j > 0 && k > 0 && i < n-1 && j < n-1 && k < n-1 {
			var scale float64
			for t, offset := range offsets {
				at := ((i+offset[0])*n+j+offset[1])*n + k + offset[2]
				x := float64(b.Input[at])
				expected += weights[t] * x
				scale += magnitude[t] * math.Abs(x)
			}
			tol = 2e-6 * math.Max(1, scale)
		}
		got := float64(actual)
		if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got-expected) > tol {
			return fmt.Errorf("at (%d,%d,%d): got %g, expected %g (tolerance %g)",
				i, j, k, got, expected, tol)
		}
	}
	log.Printf("Passed!\n")
	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GOutput, 4*((b.N)*(b.N)*(b.N))))
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
