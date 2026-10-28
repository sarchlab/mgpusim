// Package syr2k implements the PolyBench SYR2K benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/polybench_syr2k) for the MGPUSim MI300X
// (CDNA3 / gfx942) model.
//
// It computes the symmetric rank-2k update
//
//	C = alpha*A*B^T + alpha*B*A^T + beta*C
//
// where A and B are NxM matrices and C is an NxN matrix, using a
// shared-memory tiled kernel (TILE_SIZE = 16). The kernel binary is
// compiled for gfx942 only (see native/), so the benchmark must be run
// with `-arch cdna3` (the MI300X configuration).
package syr2k

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

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout below is verified against the compiled kernel's AMDGPU
// metadata (kernarg_segment_size = 40): three 8-byte global_buffer
// pointers (A, B, C) followed by four 4-byte by_value scalars
// (N, M, alpha, beta), packed with no padding (mgpusim serializes args
// with binary.Write, which does not insert alignment padding). The kernel
// reads only blockIdx/threadIdx with a constant block size, so no hidden
// ABI arguments are emitted.
type KernelArgs struct {
	A     gpuruntime.Ptr // offset 0
	B     gpuruntime.Ptr // offset 8
	C     gpuruntime.Ptr // offset 16
	N     int32          // offset 24
	M     int32          // offset 28
	Alpha float32        // offset 32
	Beta  float32        // offset 36
}

// Benchmark defines the SYR2K benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new SYR2K benchmark.
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
		cdna3HSACOBytes, "polybench_syr2k_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. SYR2K uses a single GPU.
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
		log.Panic("the polybench syr2k benchmark ships only a gfx942 " +
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
	if b.M <= 0 {
		b.M = 64
	}

	n := b.N
	m := b.M

	b.A = make([]float32, n*m)
	b.B = make([]float32, n*m)
	b.CInit = make([]float32, n*n)
	for i := 0; i < n*m; i++ {
		b.A[i] = float32(i%100) / 100.0
		b.B[i] = float32((i*2)%100) / 100.0
	}
	for i := 0; i < n*n; i++ {
		b.CInit[i] = float32((i*3)%100) / 100.0
	}

	if b.UseUnifiedMemory {
		b.GA = b.rt.AllocateUnifiedMemory(b.Context, uint64(n*m*4))
		b.GB = b.rt.AllocateUnifiedMemory(b.Context, uint64(n*m*4))
		b.GC = b.rt.AllocateUnifiedMemory(b.Context, uint64(n*n*4))
	} else {
		b.GA = b.rt.AllocateMemory(b.Context, uint64(n*m*4))
		b.GB = b.rt.AllocateMemory(b.Context, uint64(n*m*4))
		b.GC = b.rt.AllocateMemory(b.Context, uint64(n*n*4))
	}

	b.Upload(b.rt, b.Queue, b.GA, b.A)
	b.Upload(b.rt, b.Queue, b.GB, b.B)
	b.Upload(b.rt, b.Queue, b.GC, b.CInit)
}

func (b *Benchmark) exec() {
	n := b.N
	gridDim := uint32((n + tileSize - 1) / tileSize)
	global := gridDim * tileSize

	args := KernelArgs{
		A:     b.GA,
		B:     b.GB,
		C:     b.GC,
		N:     int32(n),
		M:     int32(b.M),
		Alpha: alpha,
		Beta:  beta,
	}

	b.rt.EnqueueLaunchKernel(
		b.Queue,
		b.hsaco,
		[3]uint32{global, global, 1},
		[3]uint16{tileSize, tileSize, 1},
		&args,
	)
}

// Verify checks the GPU result against a CPU reference computation that
// mirrors the kernel exactly:
//
//	C[i][j] = alpha * sum_k( A[i][k]*B[j][k] + B[i][k]*A[j][k] )
//	        + beta  * C_init[i][j]
func (b *Benchmark) verify() error {
	n := b.N
	m := b.M
	gpuC := make([]float32, n*n)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], gpuC); err != nil {
		return err
	}

	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			var sum float64
			for k := 0; k < m; k++ {
				sum += float64(b.A[i*m+k])*float64(b.B[j*m+k]) +
					float64(b.B[i*m+k])*float64(b.A[j*m+k])
			}
			ref := float64(alpha)*sum + float64(beta)*float64(b.CInit[i*n+j])
			got := float64(gpuC[i*n+j])

			denom := math.Abs(ref)
			if denom < 1.0 {
				denom = 1.0
			}
			if math.Abs(ref-got)/denom > 1e-3 {
				return fmt.Errorf("At (%d,%d), expected %f, but got %f.\n",
					i, j, ref, got)
			}
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GC, 4*((b.N)*(b.N))))
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
