// Package twomm implements the PolyBench 2MM benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/polybench_2mm) for the MGPUSim MI300X
// (CDNA3 / gfx942) model.
//
// 2MM computes two chained matrix multiplications:
//
//	tmp = alpha*A*B
//	D = tmp*C + beta*D
//
// for NxN square matrices. Both multiplications use the same shared-memory
// tiled GEMM kernel (mm_kernel, TILE_SIZE = 16), launched twice. The kernel
// binary is compiled for gfx942 only (see native/), so the benchmark must be
// run with `-arch cdna3` (the MI300X configuration).
package twomm

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

const tileSize = 16

const (
	alpha float32 = 1.5
	beta  float32 = 1.2
)

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) mm_kernel.
//
// The layout below is verified against the compiled kernel's AMDGPU
// metadata (kernarg_segment_size = 36): three 8-byte global_buffer
// pointers followed by three 4-byte by_value scalars, packed with no
// padding (mgpusim serializes args with binary.Write, which does not
// insert alignment padding). The kernel reads only blockIdx/threadIdx,
// so no hidden ABI arguments are emitted.
type KernelArgs struct {
	P     gpuruntime.Ptr // offset 0
	Q     gpuruntime.Ptr // offset 8
	Out   gpuruntime.Ptr // offset 16
	N     int32          // offset 24
	Alpha float32        // offset 28
	Beta  float32        // offset 32
}

// Benchmark defines the 2MM benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	firstKernel, secondKernel *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new 2MM benchmark.
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)

	b.rt = rt
	b.Context = b.rt.Init()

	return b
}

func (b *Benchmark) loadProgram() {
	b.firstKernel = insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, "_Z9mm_kernelILb0EEvPKfS1_Pfiff")
	b.secondKernel = insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, "_Z9mm_kernelILb1EEvPKfS1_Pfiff")
	if b.firstKernel == nil || b.secondKernel == nil {
		log.Panic("could not load PolyBench 2MM kernels")
	}
}

// SelectGPU selects the GPUs to run on. 2MM uses a single GPU.
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
		log.Panic("the polybench 2mm benchmark ships only a gfx942 " +
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
		b.N = 128
	}

	n := b.N
	numElem := n * n

	b.A = make([]float32, numElem)
	b.B = make([]float32, numElem)
	b.C = make([]float32, numElem)
	b.DInit = make([]float32, numElem)
	rand := benchmarks.NewGlibcRand(42)
	for i := range b.A {
		b.A[i] = float32(rand.Next()%100) / 10.0
		b.B[i] = float32(rand.Next()%100) / 10.0
		b.C[i] = float32(rand.Next()%100) / 10.0
		b.DInit[i] = float32(rand.Next()%100) / 10.0
	}

	if b.UseUnifiedMemory {
		b.GA = b.rt.AllocateUnifiedMemory(b.Context, uint64(numElem*4))
		b.GB = b.rt.AllocateUnifiedMemory(b.Context, uint64(numElem*4))
		b.GC = b.rt.AllocateUnifiedMemory(b.Context, uint64(numElem*4))
		b.GD = b.rt.AllocateUnifiedMemory(b.Context, uint64(numElem*4))
		b.GE = b.rt.AllocateUnifiedMemory(b.Context, uint64(numElem*4))
	} else {
		b.GA = b.rt.AllocateMemory(b.Context, uint64(numElem*4))
		b.GB = b.rt.AllocateMemory(b.Context, uint64(numElem*4))
		b.GC = b.rt.AllocateMemory(b.Context, uint64(numElem*4))
		b.GD = b.rt.AllocateMemory(b.Context, uint64(numElem*4))
		b.GE = b.rt.AllocateMemory(b.Context, uint64(numElem*4))
	}

	b.Upload(b.rt, b.Queue, b.GA, b.A)
	b.Upload(b.rt, b.Queue, b.GB, b.B)
	b.Upload(b.rt, b.Queue, b.GC, b.C)
	b.Upload(b.rt, b.Queue, b.GD, b.DInit)
}

func (b *Benchmark) exec() {
	n := b.N
	gridDim := uint32((n + tileSize - 1) / tileSize)
	globalX := gridDim * tileSize
	globalY := gridDim * tileSize

	// First form tmp = alpha*A*B, then D = tmp*C + beta*D.
	grid := [3]uint32{globalX, globalY, 1}
	group := [3]uint16{tileSize, tileSize, 1}
	first := KernelArgs{P: b.GA, Q: b.GB, Out: b.GE, N: int32(n), Alpha: alpha}
	second := KernelArgs{P: b.GE, Q: b.GC, Out: b.GD, N: int32(n), Alpha: 1, Beta: beta}
	b.rt.EnqueueLaunchKernel(b.Queue, b.firstKernel, grid, group, &first)
	b.rt.EnqueueLaunchKernel(b.Queue, b.secondKernel, grid, group, &second)
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) verify() error {
	n := b.N

	// Compute both products independently in double precision.
	refTmp := make([]float64, n*n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			var sum float64
			for k := 0; k < n; k++ {
				sum += float64(b.A[i*n+k]) * float64(b.B[k*n+j])
			}
			refTmp[i*n+j] = float64(alpha) * sum
		}
	}
	refD := make([]float64, n*n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			var sum float64
			for k := 0; k < n; k++ {
				sum += refTmp[i*n+k] * float64(b.C[k*n+j])
			}
			refD[i*n+j] = sum + float64(beta)*float64(b.DInit[i*n+j])
		}
	}
	actual := make([]float32, n*n)
	for stage, expected := range [][]float64{refTmp, refD} {
		if err := benchmarks.Read(b.rt, b.Readbacks[stage], actual); err != nil {
			return err
		}
		for i, reference := range expected {
			got := float64(actual[i])
			if math.IsNaN(got) || math.IsInf(got, 0) ||
				math.Abs(reference-got) > 5e-4*math.Max(1, math.Abs(reference)) {
				return fmt.Errorf("product %d at (%d,%d): got %g, expected %g",
					stage+1, i/n, i%n, got, reference)
			}
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GE, 4*b.N*b.N))
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GD, 4*b.N*b.N))
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
