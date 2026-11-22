// Package conv2d implements the PolyBench 2D Convolution benchmark, ported
// from sarchlab/gpu_benchmarks (tier2/polybench_2dconv) for the MGPUSim
// MI300X (CDNA3 / gfx942) model.
//
// It applies a fixed 3x3 PolyBench coefficient stencil to an NI x NJ matrix
// A, producing output B, with one work-item per output element. The kernel
// binary is compiled for gfx942 only (see native/), so the benchmark must be
// run with `-arch cdna3` (the MI300X configuration).
package conv2d

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

// blockSize is the constant work-group dimension (16x16) baked into the
// kernel. Using a compile-time constant in the kernel avoids hidden ABI args.
const blockSize = 16

// KernelArgs encodes explicit kernel arguments and gfx942 launch geometry.
type KernelArgs struct {
	A      gpuruntime.Ptr // offset 0
	B      gpuruntime.Ptr // offset 8
	NI     int32          // offset 16
	NJ     int32          // offset 20
	Launch benchmarks.GFX942LaunchDimensions
}

// Benchmark defines the 2D convolution benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new 2D convolution benchmark.
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
		cdna3HSACOBytes, "convolution2D_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. This benchmark uses a single GPU.
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
		log.Panic("the polybench 2dconv benchmark ships only a gfx942 " +
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
		b.N = 64
	}

	n := b.N
	numElem := n * n

	// Deterministic host init that Verify() reproduces exactly.
	b.A = make([]float32, numElem)
	rand := benchmarks.NewGlibcRand(42)
	for i := range b.A {
		b.A[i] = float32(rand.Next()%100) / 10.0
	}

	if b.UseUnifiedMemory {
		b.GA = b.rt.AllocateUnifiedMemory(b.Context, uint64(numElem*4))
		b.GB = b.rt.AllocateUnifiedMemory(b.Context, uint64(numElem*4))
	} else {
		b.GA = b.rt.AllocateMemory(b.Context, uint64(numElem*4))
		b.GB = b.rt.AllocateMemory(b.Context, uint64(numElem*4))
	}

	// Initialize B to zero so untouched border elements match the reference.
	zeros := make([]float32, numElem)
	b.Upload(b.rt, b.Queue, b.GA, b.A)
	b.Upload(b.rt, b.Queue, b.GB, zeros)
}

func (b *Benchmark) exec() {
	n := b.N
	gridDim := uint32((n + blockSize - 1) / blockSize)
	globalX := gridDim * blockSize
	globalY := gridDim * blockSize

	args := KernelArgs{
		A:      b.GA,
		B:      b.GB,
		NI:     int32(n),
		NJ:     int32(n),
		Launch: benchmarks.NewGFX942LaunchDimensions([3]uint32{globalX, globalY, 1}, [3]uint16{blockSize, blockSize, 1}),
	}

	b.rt.EnqueueLaunchKernel(
		b.Queue,
		b.hsaco,
		[3]uint32{globalX, globalY, 1},
		[3]uint16{blockSize, blockSize, 1},
		&args,
	)
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) verify() error {
	n := b.N
	gpuB := make([]float32, n*n)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], gpuB); err != nil {
		return err
	}

	weights := [3][3]float64{{0.2, 0.5, -0.8}, {-0.3, 0.6, -0.9}, {0.4, 0.7, 0.1}}
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			var want, tolerance float64
			if i > 0 && i < n-1 && j > 0 && j < n-1 {
				for dy, row := range weights {
					for dx, weight := range row {
						want += weight * float64(b.A[(i+dy-1)*n+j+dx-1])
					}
				}
				tolerance = 1e-5 * math.Max(1, math.Abs(want))
			}
			got := float64(gpuB[i*n+j])
			if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(want-got) > tolerance {
				return fmt.Errorf("convolution[%d,%d] got %g, expected %g within %g", i, j, got, want, tolerance)
			}
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GB, 4*((b.N)*(b.N))))
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
