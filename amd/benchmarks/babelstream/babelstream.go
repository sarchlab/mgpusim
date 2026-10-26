// Package babelstream implements the BabelStream benchmark, ported from
// sarchlab/gpu_benchmarks (tier4/babelstream) for the MGPUSim MI300X
// (CDNA3 / gfx942) model.
//
// BabelStream measures memory bandwidth via four elementwise vector
// operations over float arrays of length N:
//
//	copy:   c[i] = a[i]
//	scale:  b[i] = s * c[i]
//	add:    c[i] = a[i] + b[i]
//	triad:  a[i] = b[i] + s * c[i]
//
// The four operations are run once each, in this order, exactly as the host
// loop in the original benchmark drives them. Verify() reproduces the same
// sequence on the CPU.
//
// The kernels are compiled for gfx942 only (see native/) with a constant
// block size of 256, so they emit no hidden ABI arguments. The benchmark
// must be run with `-arch cdna3` (the MI300X configuration).
package babelstream

import (
	// embed hsaco files
	_ "embed"
	"fmt"
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// blockSize is the constant work-group size baked into the kernels (see
// native/babelstream.cpp). It must match the BLOCK_SIZE literal there.
const blockSize = 256

// CopyKernelArgs matches copy_kernel (kernarg_segment_size = 20):
// a(ptr,0), c(ptr,8), n(i32,16).
type CopyKernelArgs struct {
	A driver.Ptr // offset 0
	C driver.Ptr // offset 8
	N int32      // offset 16
}

// ScaleKernelArgs matches scale_kernel (kernarg_segment_size = 24):
// c(ptr,0), b(ptr,8), s(f32,16), n(i32,20).
type ScaleKernelArgs struct {
	C driver.Ptr // offset 0
	B driver.Ptr // offset 8
	S float32    // offset 16
	N int32      // offset 20
}

// AddKernelArgs matches add_kernel (kernarg_segment_size = 28):
// a(ptr,0), b(ptr,8), c(ptr,16), n(i32,24).
type AddKernelArgs struct {
	A driver.Ptr // offset 0
	B driver.Ptr // offset 8
	C driver.Ptr // offset 16
	N int32      // offset 24
}

// TriadKernelArgs matches triad_kernel (kernarg_segment_size = 32):
// b(ptr,0), c(ptr,8), a(ptr,16), s(f32,24), n(i32,28).
type TriadKernelArgs struct {
	B driver.Ptr // offset 0
	C driver.Ptr // offset 8
	A driver.Ptr // offset 16
	S float32    // offset 24
	N int32      // offset 28
}

// Benchmark defines the BabelStream benchmark.
type Benchmark struct {
	*State
	benchmarks.Saved[checkpointConfig, State]

	driver *driver.Runtime

	copyKernel  *insts.KernelCodeObject
	scaleKernel *insts.KernelCodeObject
	addKernel   *insts.KernelCodeObject
	triadKernel *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new BabelStream benchmark.
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Saved = benchmarks.NewSaved(b.State,
		b.checkpointConfig, b.bind)

	b.driver = driver
	b.Context = b.driver.Init()

	return b
}

func (b *Benchmark) loadProgram() {
	b.copyKernel = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "copy_kernel")
	b.scaleKernel = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "scale_kernel")
	b.addKernel = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "add_kernel")
	b.triadKernel = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "triad_kernel")

	if b.copyKernel == nil || b.scaleKernel == nil ||
		b.addKernel == nil || b.triadKernel == nil {
		log.Panic("Failed to load one or more kernel binaries")
	}
}

// SelectGPU selects the GPUs to run on. BabelStream uses a single GPU.
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
		log.Panic("the babelstream benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.driver.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.N <= 0 {
		b.N = 4096
	}
	if b.Scalar == 0 {
		b.Scalar = 2.0
	}

	n := b.N

	b.A = make([]float32, n)
	b.B = make([]float32, n)
	b.C = make([]float32, n)
	// Deterministic, varied host init so the verification is meaningful.
	for i := 0; i < n; i++ {
		b.A[i] = float32(i%100) / 100.0
		b.B[i] = float32((i*2)%100) / 100.0
		b.C[i] = 0.0
	}

	if b.UseUnifiedMemory {
		b.GA = b.driver.AllocateUnifiedMemory(b.Context, uint64(n*4))
		b.GB = b.driver.AllocateUnifiedMemory(b.Context, uint64(n*4))
		b.GC = b.driver.AllocateUnifiedMemory(b.Context, uint64(n*4))
	} else {
		b.GA = b.driver.AllocateMemory(b.Context, uint64(n*4))
		b.GB = b.driver.AllocateMemory(b.Context, uint64(n*4))
		b.GC = b.driver.AllocateMemory(b.Context, uint64(n*4))
	}

	b.driver.EnqueueMemCopyH2D(b.Queue, b.GA, b.A)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GB, b.B)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GC, b.C)
}

func (b *Benchmark) exec() {
	n := b.N
	gridDim := uint32((n + blockSize - 1) / blockSize)
	globalX := gridDim * blockSize

	// copy: c[i] = a[i]
	copyArgs := CopyKernelArgs{A: b.GA, C: b.GC, N: int32(n)}
	b.driver.EnqueueLaunchKernel(
		b.Queue, b.copyKernel,
		[3]uint32{globalX, 1, 1}, [3]uint16{blockSize, 1, 1},
		&copyArgs,
	)

	// scale: b[i] = s * c[i]
	scaleArgs := ScaleKernelArgs{C: b.GC, B: b.GB, S: b.Scalar, N: int32(n)}
	b.driver.EnqueueLaunchKernel(
		b.Queue, b.scaleKernel,
		[3]uint32{globalX, 1, 1}, [3]uint16{blockSize, 1, 1},
		&scaleArgs,
	)

	// add: c[i] = a[i] + b[i]
	addArgs := AddKernelArgs{A: b.GA, B: b.GB, C: b.GC, N: int32(n)}
	b.driver.EnqueueLaunchKernel(
		b.Queue, b.addKernel,
		[3]uint32{globalX, 1, 1}, [3]uint16{blockSize, 1, 1},
		&addArgs,
	)

	// triad: a[i] = b[i] + s * c[i]
	triadArgs := TriadKernelArgs{B: b.GB, C: b.GC, A: b.GA, S: b.Scalar, N: int32(n)}
	b.driver.EnqueueLaunchKernel(
		b.Queue, b.triadKernel,
		[3]uint32{globalX, 1, 1}, [3]uint16{blockSize, 1, 1},
		&triadArgs,
	)
}

// Verify checks the GPU result against a CPU reference computation that
// reproduces the same copy -> scale -> add -> triad sequence.
func (b *Benchmark) verify() error {
	n := b.N
	s := float64(b.Scalar)

	// CPU reference, mirroring exec() order on a copy of the init data.
	refA := make([]float64, n)
	refB := make([]float64, n)
	refC := make([]float64, n)
	for i := 0; i < n; i++ {
		refA[i] = float64(b.A[i])
		refB[i] = float64(b.B[i])
		refC[i] = 0.0
	}
	for i := 0; i < n; i++ {
		refC[i] = refA[i]             // copy
		refB[i] = s * refC[i]         // scale
		refC[i] = refA[i] + refB[i]   // add
		refA[i] = refB[i] + s*refC[i] // triad
	}

	gpuA := make([]float32, n)
	gpuB := make([]float32, n)
	gpuC := make([]float32, n)
	if err := b.driver.ReadResult(b.Readbacks[0], gpuA); err != nil {
		return err
	}
	if err := b.driver.ReadResult(b.Readbacks[1], gpuB); err != nil {
		return err
	}
	if err := b.driver.ReadResult(b.Readbacks[2], gpuC); err != nil {
		return err
	}

	check := func(name string, idx int, ref float64, got float32) error {
		denom := math.Abs(ref)
		if denom < 1.0 {
			denom = 1.0
		}
		if math.Abs(ref-float64(got))/denom > 1e-3 {
			return fmt.Errorf("%s mismatch at %d, expected %f, but got %f.\n",
				name, idx, ref, got)
		}

		return nil
	}

	for i := 0; i < n; i++ {
		if err := check("a", i, refA[i], gpuA[i]); err != nil {
			return err
		}
		if err := check("b", i, refB[i], gpuB[i]); err != nil {
			return err
		}
		if err := check("c", i, refC[i], gpuC[i]); err != nil {
			return err
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, b.driver.EnqueueMemCopyD2H(b.Queue, b.GA, 4*(b.N)))
	b.Readbacks = append(b.Readbacks, b.driver.EnqueueMemCopyD2H(b.Queue, b.GB, 4*(b.N)))
	b.Readbacks = append(b.Readbacks, b.driver.EnqueueMemCopyD2H(b.Queue, b.GC, 4*(b.N)))
}

// Step submits computation, then verification readback, then consumes results.
func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
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
