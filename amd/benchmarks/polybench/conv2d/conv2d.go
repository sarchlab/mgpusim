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
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// blockSize is the constant work-group dimension (16x16) baked into the
// kernel. Using a compile-time constant in the kernel avoids hidden ABI args.
const blockSize = 16

// PolyBench fixed 3x3 convolution coefficients.
const (
	c00 float32 = 0.8
	c01 float32 = 0.2
	c02 float32 = 0.3
	c10 float32 = 0.2
	c11 float32 = 0.7
	c12 float32 = 0.4
	c20 float32 = 0.1
	c21 float32 = 0.2
	c22 float32 = 0.5
)

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout below is verified against the compiled kernel's AMDGPU
// metadata (kernarg_segment_size = 24): two 8-byte global_buffer pointers
// followed by two 4-byte by_value scalars, packed with no padding (mgpusim
// serializes args with binary.Write, which does not insert alignment
// padding). The kernel uses constant block dimensions, so no hidden ABI
// arguments are emitted.
type KernelArgs struct {
	A  driver.Ptr // offset 0
	B  driver.Ptr // offset 8
	NI int32      // offset 16
	NJ int32      // offset 20
}

// Benchmark defines the 2D convolution benchmark.
type Benchmark struct {
	*State
	benchmarks.Saved[checkpointConfig, State]

	driver *driver.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new 2D convolution benchmark.
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Saved = benchmarks.NewSaved(b.State, b.checkpointConfig, b.bind)

	b.driver = driver
	b.Context = b.driver.Init()

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

	b.driver.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.driver.CreateCommandQueue(b.Context)
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
	for i := 0; i < numElem; i++ {
		b.A[i] = float32(i%100) / 10.0
	}

	if b.UseUnifiedMemory {
		b.GA = b.driver.AllocateUnifiedMemory(b.Context, uint64(numElem*4))
		b.GB = b.driver.AllocateUnifiedMemory(b.Context, uint64(numElem*4))
	} else {
		b.GA = b.driver.AllocateMemory(b.Context, uint64(numElem*4))
		b.GB = b.driver.AllocateMemory(b.Context, uint64(numElem*4))
	}

	// Initialize B to zero so untouched border elements match the reference.
	zeros := make([]float32, numElem)
	b.Upload(b.driver, b.Queue, b.GA, b.A)
	b.Upload(b.driver, b.Queue, b.GB, zeros)
}

func (b *Benchmark) exec() {
	n := b.N
	gridDim := uint32((n + blockSize - 1) / blockSize)
	globalX := gridDim * blockSize
	globalY := gridDim * blockSize

	args := KernelArgs{
		A:  b.GA,
		B:  b.GB,
		NI: int32(n),
		NJ: int32(n),
	}

	b.driver.EnqueueLaunchKernel(
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
	if err := benchmarks.Read(b.driver, b.Readbacks[0], gpuB); err != nil {
		return err
	}

	ref := make([]float32, n*n)
	for i := 1; i < n-1; i++ {
		for j := 1; j < n-1; j++ {
			ref[i*n+j] =
				c00*b.A[(i-1)*n+(j-1)] +
					c01*b.A[(i-1)*n+j] +
					c02*b.A[(i-1)*n+(j+1)] +
					c10*b.A[i*n+(j-1)] +
					c11*b.A[i*n+j] +
					c12*b.A[i*n+(j+1)] +
					c20*b.A[(i+1)*n+(j-1)] +
					c21*b.A[(i+1)*n+j] +
					c22*b.A[(i+1)*n+(j+1)]
		}
	}

	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			want := float64(ref[i*n+j])
			got := float64(gpuB[i*n+j])

			denom := math.Abs(want)
			if denom < 1.0 {
				denom = 1.0
			}
			if math.Abs(want-got)/denom > 1e-3 {
				return fmt.Errorf("At (%d,%d), expected %f, but got %f.\n",
					i, j, want, got)
			}
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.driver, b.Queue, b.GB, 4*((b.N)*(b.N))))
}

// Step submits computation, then verification readback, then consumes results.
func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if err := b.ReleaseUploads(b.driver); err != nil {
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
