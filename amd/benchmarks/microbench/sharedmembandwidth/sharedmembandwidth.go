// Package sharedmembandwidth implements the shared-memory (LDS) bandwidth
// microbenchmark for the MGPUSim MI300X (CDNA3 / gfx942) model.
//
// Two kernels stream a block-local __shared__ buffer using UNROLL independent
// accumulators, so many LDS reads are in flight at once and the LDS unit can be
// saturated (bandwidth, not latency):
//   - smem_bw_no_conflict : consecutive lanes -> consecutive banks (conflict-free)
//   - smem_bw_conflict    : 32 lanes -> the same bank (32-way bank conflict)
//
// To stop the compiler from folding the loop-invariant, all-equal LDS reads
// away, the buffer is staged from a GLOBAL input (d_in, contents opaque to the
// compiler) and the read index depends on the loop counter (not hoistable).
// d_in is filled with 1.0 on the host, so each block's result is exactly
// UNROLL*InnerIters -- deterministic -- while the compiler still must emit
// InnerIters*UNROLL real ds_read ops. To actually measure bandwidth, launch
// with a full block and many blocks (>= number of CUs).
//
// The block size (256), shared-buffer length (2048 floats) and UNROLL (8) are
// compile-time constants in the kernel, so the gfx942 binary carries NO hidden
// ABI arguments (kernarg_segment_size = 20: two 8-byte pointers + one 4-byte
// by_value int). Run with `-arch cdna3` (the MI300X configuration).
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

// These mirror constants in the gfx942 kernel (native/shared_mem_bandwidth.cpp).
const (
	defaultBlockSize = 256  // work-group size used when BlockSize is unset
	smemFloats       = 8192 // SMEM_FLOATS: fixed 32 KB LDS footprint (matches HW)
	unroll           = 8    // UNROLL independent accumulators
)

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernels.
//
// Verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 24): two 8-byte global_buffer pointers followed by
// two 4-byte by_value ints (inner_iters, block_size), packed with no padding
// (mgpusim serializes args with binary.Write, which inserts no alignment
// padding). Both kernels share this layout.
type KernelArgs struct {
	In         driver.Ptr // offset 0  (opaque source data, staged into LDS)
	Sink       driver.Ptr // offset 8
	InnerIters int32      // offset 16
	BlockSize  int32      // offset 20
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
	// InnerIters is the number of streaming iterations the kernel runs.
	InnerIters int
	// BlockSize is the work-group size (threads per block), swept by the
	// calibration and passed to the kernel as an explicit argument.
	BlockSize int
	// AccessPattern selects which kernel(s) run: "no_conflict" (default),
	// "conflict", or "both".
	AccessPattern string

	gIn   driver.Ptr
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
			"kernel; run with -arch cdna3 -gpu mi300x")
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
	if b.BlockSize <= 0 {
		b.BlockSize = defaultBlockSize
	}
	if b.AccessPattern == "" {
		b.AccessPattern = "no_conflict"
	}

	// The kernel stages d_in[0 .. block_size*UNROLL) into LDS (one contiguous
	// chunk per thread); that is exactly the region the streaming reads mask to.
	inFloats := b.BlockSize * unroll

	if b.useUnifiedMemory {
		b.gIn = b.driver.AllocateUnifiedMemory(
			b.context, uint64(inFloats*4))
		b.gSink = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.NumBlocks*4))
	} else {
		b.gIn = b.driver.AllocateMemory(b.context, uint64(inFloats*4))
		b.gSink = b.driver.AllocateMemory(b.context, uint64(b.NumBlocks*4))
	}

	// Opaque source data: all 1.0, so each block's accumulator is exactly
	// unroll*InnerIters, but the compiler can't fold the LDS reads.
	in := make([]float32, inFloats)
	for i := range in {
		in[i] = 1.0
	}
	b.driver.MemCopyH2D(b.context, b.gIn, in)

	// Zero-initialise the sink so unwritten lanes are well defined.
	zeros := make([]float32, b.NumBlocks)
	b.driver.MemCopyH2D(b.context, b.gSink, zeros)
}

// launch runs one kernel (no_conflict or conflict) with the configured block
// size as both the work-group dimension and an explicit kernel argument.
func (b *Benchmark) launch(kernel *insts.KernelCodeObject) {
	args := KernelArgs{
		In:         b.gIn,
		Sink:       b.gSink,
		InnerIters: int32(b.InnerIters),
		BlockSize:  int32(b.BlockSize),
	}
	b.driver.EnqueueLaunchKernel(
		b.queue,
		kernel,
		[3]uint32{uint32(b.NumBlocks * b.BlockSize), 1, 1},
		[3]uint16{uint16(b.BlockSize), 1, 1},
		&args,
	)
	b.driver.DrainCommandQueue(b.queue)
}

func (b *Benchmark) exec() {
	runNoConflict := b.AccessPattern != "conflict"
	runConflict := b.AccessPattern == "conflict" || b.AccessPattern == "both"

	if runNoConflict {
		b.launch(b.hsaco)
		// Snapshot the deterministic no-conflict result before any conflict
		// launch overwrites the shared sink.
		b.noConflictResult = make([]float32, b.NumBlocks)
		b.driver.MemCopyD2H(b.context, b.noConflictResult, b.gSink)
	}

	if runConflict {
		b.launch(b.conf)
	}
}

// Verify checks the GPU result against the CPU reference. With d_in all 1.0,
// each block's thread 0 accumulates 1.0 once per (iteration, accumulator), so
// the result is exactly unroll*InnerIters and independent of scheduling.
func (b *Benchmark) Verify() {
	if b.noConflictResult == nil {
		log.Printf("Verify skipped (no_conflict kernel not run).\n")
		return
	}
	ref := float32(unroll * b.InnerIters)

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
