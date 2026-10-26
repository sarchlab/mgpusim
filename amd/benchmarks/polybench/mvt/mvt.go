// Package mvt implements the PolyBench MVT benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/polybench_mvt) for the MGPUSim MI300X
// (CDNA3 / gfx942) model.
//
// MVT (matrix-vector-product and transpose) computes:
//
//	x1 = A   * y1    (x1[i] = sum_j A[i,j] * y1[j])
//	x2 = A^T * y2    (x2[j] = sum_i A[i,j] * y2[i])
//
// for an N x N matrix A and N-vectors x1, x2, y1, y2. Two 1D kernels are
// launched, one output element per thread. The kernel binary is compiled
// for gfx942 only (see native/), so the benchmark must be run with
// `-arch cdna3` (the MI300X configuration).
package mvt

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

// blockSize matches the constant BLOCK_SIZE compiled into the gfx942
// kernels. Because the kernels use this constant (not blockDim.x), the
// compiler emits no hidden ABI arguments.
const blockSize = 256

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernels.
//
// The layout is verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 28): three 8-byte global_buffer pointers
// followed by one 4-byte by_value scalar, packed with no padding
// (mgpusim serializes args with binary.Write, which inserts no alignment
// padding). The kernels read only blockIdx/threadIdx, so no hidden ABI
// arguments are emitted. The same layout is used by both mvt_kernel1
// (A, y1, x1, n) and mvt_kernel2 (A, y2, x2, n).
type KernelArgs struct {
	A driver.Ptr // offset 0
	Y driver.Ptr // offset 8  (y1 for kernel1, y2 for kernel2)
	X driver.Ptr // offset 16 (x1 for kernel1, x2 for kernel2)
	N int32      // offset 24
}

// Benchmark defines the MVT benchmark.
type Benchmark struct {
	*State
	benchmarks.Saved[checkpointConfig, State]

	driver *driver.Runtime

	kernel1 *insts.KernelCodeObject
	kernel2 *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new MVT benchmark.
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Saved = benchmarks.NewSaved(b.State, b.checkpointConfig, b.bind)

	b.driver = driver
	b.Context = b.driver.Init()

	return b
}

func (b *Benchmark) loadProgram() {
	b.kernel1 = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "mvt_kernel1")
	if b.kernel1 == nil {
		log.Panic("Failed to load kernel binary (mvt_kernel1)")
	}

	b.kernel2 = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "mvt_kernel2")
	if b.kernel2 == nil {
		log.Panic("Failed to load kernel binary (mvt_kernel2)")
	}
}

// SelectGPU selects the GPUs to run on. MVT uses a single GPU.
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
		log.Panic("the polybench mvt benchmark ships only a gfx942 " +
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
		b.N = 128
	}

	n := b.N
	numElem := n * n

	b.A = make([]float32, numElem)
	b.Y1 = make([]float32, n)
	b.Y2 = make([]float32, n)

	// Deterministic host init, reproduced exactly in Verify().
	for i := 0; i < numElem; i++ {
		b.A[i] = float32(i%100) / 10.0
	}
	for j := 0; j < n; j++ {
		b.Y1[j] = float32((j*3)%100) / 10.0
		b.Y2[j] = float32((j*7)%100) / 10.0
	}

	if b.UseUnifiedMemory {
		b.GA = b.driver.AllocateUnifiedMemory(b.Context, uint64(numElem*4))
		b.GY1 = b.driver.AllocateUnifiedMemory(b.Context, uint64(n*4))
		b.GY2 = b.driver.AllocateUnifiedMemory(b.Context, uint64(n*4))
		b.GX1 = b.driver.AllocateUnifiedMemory(b.Context, uint64(n*4))
		b.GX2 = b.driver.AllocateUnifiedMemory(b.Context, uint64(n*4))
	} else {
		b.GA = b.driver.AllocateMemory(b.Context, uint64(numElem*4))
		b.GY1 = b.driver.AllocateMemory(b.Context, uint64(n*4))
		b.GY2 = b.driver.AllocateMemory(b.Context, uint64(n*4))
		b.GX1 = b.driver.AllocateMemory(b.Context, uint64(n*4))
		b.GX2 = b.driver.AllocateMemory(b.Context, uint64(n*4))
	}

	b.driver.EnqueueMemCopyH2D(b.Queue, b.GA, b.A)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GY1, b.Y1)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GY2, b.Y2)
}

func (b *Benchmark) exec() {
	n := b.N
	gridDim := uint32((n + blockSize - 1) / blockSize)
	globalX := gridDim * blockSize

	// Kernel 1: x1 = A * y1
	args1 := KernelArgs{
		A: b.GA,
		Y: b.GY1,
		X: b.GX1,
		N: int32(n),
	}
	b.driver.EnqueueLaunchKernel(
		b.Queue,
		b.kernel1,
		[3]uint32{globalX, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&args1,
	)

	// Kernel 2: x2 = A^T * y2
	args2 := KernelArgs{
		A: b.GA,
		Y: b.GY2,
		X: b.GX2,
		N: int32(n),
	}
	b.driver.EnqueueLaunchKernel(
		b.Queue,
		b.kernel2,
		[3]uint32{globalX, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&args2,
	)
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) verify() error {
	n := b.N

	gpuX1 := make([]float32, n)
	gpuX2 := make([]float32, n)
	if err := b.driver.ReadResult(b.Readbacks[0], gpuX1); err != nil {
		return err
	}
	if err := b.driver.ReadResult(b.Readbacks[1], gpuX2); err != nil {
		return err
	}

	// x1[i] = sum_j A[i,j] * y1[j]
	for i := 0; i < n; i++ {
		var sum float64
		for j := 0; j < n; j++ {
			sum += float64(b.A[i*n+j]) * float64(b.Y1[j])
		}
		got := float64(gpuX1[i])
		if relErr(sum, got) > 1e-3 {
			return fmt.Errorf("x1 mismatch at %d: expected %f, but got %f.\n",
				i, sum, got)
		}
	}

	// x2[j] = sum_i A[i,j] * y2[i]
	for j := 0; j < n; j++ {
		var sum float64
		for i := 0; i < n; i++ {
			sum += float64(b.A[i*n+j]) * float64(b.Y2[i])
		}
		got := float64(gpuX2[j])
		if relErr(sum, got) > 1e-3 {
			return fmt.Errorf("x2 mismatch at %d: expected %f, but got %f.\n",
				j, sum, got)
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, b.driver.EnqueueMemCopyD2H(b.Queue, b.GX1, 4*(b.N)))
	b.Readbacks = append(b.Readbacks, b.driver.EnqueueMemCopyD2H(b.Queue, b.GX2, 4*(b.N)))
}

func relErr(ref, got float64) float64 {
	denom := math.Abs(ref)
	if denom < 1.0 {
		denom = 1.0
	}
	return math.Abs(ref-got) / denom
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
