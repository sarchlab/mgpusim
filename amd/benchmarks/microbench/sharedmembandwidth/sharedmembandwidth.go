// Package sharedmembandwidth implements the shared-memory (LDS) bandwidth
// microbenchmark, ported from sarchlab/gpu_benchmarks
// (tier1/shared_mem_bandwidth) for the MGPUSim MI300A (CDNA3 / gfx942) model.
//
// Two kernels exercise a block-local __shared__ buffer:
//   - smem_bw_no_conflict : stride-1 per lane (bank-conflict free)
//   - smem_bw_conflict    : stride-32 between lanes (intra-warp conflicts)
//
// Each thread accumulates a running sum while reading/writing its cells;
// thread 0 of each block writes that accumulator to d_sink[blockIdx.x].
//
// The block size (64) and the shared-buffer length (512 floats) are
// compile-time constants in the kernel, so the gfx942 binary carries NO
// hidden ABI arguments (kernarg_segment_size = 12). The benchmark must be
// run with `-arch cdna3` (the MI300A configuration).
//
// Verify() reproduces the deterministic smem_bw_no_conflict result on the
// CPU. In that pattern each thread owns a disjoint set of cells, so the
// final accumulator is independent of warp scheduling and exactly
// reproducible. The conflict kernel is launched for faithfulness/timing but
// its values are scheduling-dependent and are not asserted.
package sharedmembandwidth

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// These mirror the compile-time constants baked into the gfx942 kernel
// (see native/shared_mem_bandwidth.cpp). Keep them in sync.
const (
	blockSize  = 64  // BLOCK_SIZE
	smemFloats = 512 // SMEM_FLOATS
)

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernels.
//
// Verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 12): one 8-byte global_buffer pointer followed by
// one 4-byte by_value int, packed with no padding (mgpusim serializes args
// with binary.Write, which inserts no alignment padding). Both kernels share
// this identical layout, and neither emits hidden ABI arguments because the
// block geometry is a compile-time constant.
type KernelArgs struct {
	Sink       driver.Ptr // offset 0
	InnerIters int32      // offset 8
}

// Benchmark defines the shared-memory bandwidth benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	conf    *insts.KernelCodeObject
	gpus    []int

	Arch arch.Type

	// NumBlocks is the number of work-groups (grid size in the X dimension).
	NumBlocks int
	// InnerIters is the number of outer timing iterations the kernel runs.
	InnerIters int

	gSink driver.Ptr

	// noConflictResult holds the d_sink contents after the no-conflict
	// kernel, snapshotted before the conflict kernel overwrites the sink.
	noConflictResult []float32

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new shared-memory bandwidth benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "smem_bw_no_conflict")
	if b.hsaco == nil {
		log.Panic("Failed to load smem_bw_no_conflict kernel binary")
	}

	b.conf = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "smem_bw_conflict")
	if b.conf == nil {
		log.Panic("Failed to load smem_bw_conflict kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. This benchmark uses a single GPU.
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
		log.Panic("the shared_mem_bandwidth benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.NumBlocks <= 0 {
		b.NumBlocks = 16
	}
	if b.InnerIters <= 0 {
		b.InnerIters = 8
	}

	if b.useUnifiedMemory {
		b.gSink = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.NumBlocks*4))
	} else {
		b.gSink = b.driver.AllocateMemory(
			b.context, uint64(b.NumBlocks*4))
	}

	// Zero-initialise the sink so unwritten lanes are well defined.
	zeros := make([]float32, b.NumBlocks)
	b.driver.MemCopyH2D(b.context, b.gSink, zeros)
}

func (b *Benchmark) exec() {
	args := KernelArgs{
		Sink:       b.gSink,
		InnerIters: int32(b.InnerIters),
	}

	// No-conflict kernel (the one Verify() checks).
	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsaco,
		[3]uint32{uint32(b.NumBlocks) * blockSize, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&args,
	)
	b.driver.DrainCommandQueue(b.queue)

	// Conflict kernel: launched for faithfulness/timing. Reuses the same
	// sink (overwriting the no-conflict result), so snapshot the no-conflict
	// result before running it.
	b.noConflictResult = make([]float32, b.NumBlocks)
	b.driver.MemCopyD2H(b.context, b.noConflictResult, b.gSink)

	confArgs := KernelArgs{
		Sink:       b.gSink,
		InnerIters: int32(b.InnerIters),
	}
	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.conf,
		[3]uint32{uint32(b.NumBlocks) * blockSize, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&confArgs,
	)
	b.driver.DrainCommandQueue(b.queue)
}

// computeNoConflictAcc reproduces, on the CPU, the accumulator that thread 0
// of every block produces in the smem_bw_no_conflict kernel.
//
// In that pattern thread tid touches the disjoint index set
// {tid, tid+n, tid+2n, ...}. For thread 0 with stride n=blockSize the cells
// are 0, n, 2n, ... < smemFloats, each starting at 1.0. The inner sequence
//
//	acc += smem[i]; smem[i] = acc;
//
// over the cells in order, repeated InnerIters times, is order-independent
// across threads, so it is exactly reproducible here.
func (b *Benchmark) computeNoConflictAcc() float32 {
	n := blockSize

	// Cells owned by thread 0.
	cells := make([]float32, 0, smemFloats/n+1)
	for i := 0; i < smemFloats; i += n {
		cells = append(cells, 1.0)
	}

	var acc float32
	for it := 0; it < b.InnerIters; it++ {
		for k := range cells {
			acc += cells[k]
			cells[k] = acc
		}
	}
	return acc
}

// Verify checks the GPU result against a CPU reference computation.
//
// It validates the deterministic smem_bw_no_conflict result (every block's
// thread 0 produces the same accumulator) snapshotted before the conflict
// kernel ran.
func (b *Benchmark) Verify() {
	ref := b.computeNoConflictAcc()

	for blk := 0; blk < b.NumBlocks; blk++ {
		got := b.noConflictResult[blk]

		denom := math.Abs(float64(ref))
		if denom < 1.0 {
			denom = 1.0
		}
		if math.Abs(float64(ref-got))/denom > 1e-3 {
			log.Fatalf("Block %d: expected sink %f, but got %f.\n",
				blk, ref, got)
		}
	}

	log.Printf("Passed!\n")
}
