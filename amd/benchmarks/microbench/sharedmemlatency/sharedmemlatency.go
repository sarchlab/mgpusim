// Package sharedmemlatency implements the shared-memory (LDS) latency
// microbenchmark, ported from sarchlab/gpu_benchmarks
// (tier1/shared_mem_latency) for the MGPUSim MI300X (CDNA3 / gfx942) model.
//
// Two kernels exercise a block-local __shared__ buffer:
//   - smem_lat_no_conflict : stride-1 per lane (bank-conflict free)
//   - smem_lat_conflict    : stride-32 between lanes (intra-warp conflicts)
//
// Each thread accumulates a running sum while reading/writing its cells;
// thread 0 of each block writes that accumulator to d_sink[blockIdx.x].
//
// The shared-buffer length (SMEM_FLOATS = 12288 floats / 48 KB) matches the
// real gpu_benchmarks kernel, and the block size is a RUNTIME kernel argument
// (the CDNA3 emulator does not populate blockDim.x), so the benchmark sweeps
// the same work and the same block_size / access_pattern the hardware does.
// The benchmark must be run with `-arch cdna3` (the MI300X configuration).
//
// Verify() reproduces the deterministic smem_lat_no_conflict result on the
// CPU. In that pattern each thread owns a disjoint set of cells, so the
// final accumulator is independent of warp scheduling and exactly
// reproducible. The conflict kernel's values are scheduling-dependent and
// are not asserted.
package sharedmemlatency

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

const (
	// defaultBlockSize matches the gpu_benchmarks default block_size; the
	// block size is now a runtime kernel argument, not a compile-time constant.
	defaultBlockSize = 256
	// smemFloats mirrors the SMEM_FLOATS compile-time constant baked into the
	// gfx942 kernel (native/shared_mem_latency.cpp) -- 48 KB, matching the HW.
	smemFloats = 12288 // SMEM_FLOATS
)

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernels.
//
// Verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 16): one 8-byte global_buffer pointer followed by
// two 4-byte by_value ints (inner_iters, block_size), packed with no padding
// (mgpusim serializes args with binary.Write, which inserts no alignment
// padding). Both kernels share this identical layout.
type KernelArgs struct {
	Sink       gpuruntime.Ptr // offset 0
	InnerIters int32          // offset 8
	BlockSize  int32          // offset 12
}

// Benchmark defines the shared-memory latency benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
	conf  *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new shared-memory latency benchmark.
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
		cdna3HSACOBytes, "smem_lat_no_conflict")
	if b.hsaco == nil {
		log.Panic("Failed to load smem_lat_no_conflict kernel binary")
	}

	b.conf = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "smem_lat_conflict")
	if b.conf == nil {
		log.Panic("Failed to load smem_lat_conflict kernel binary")
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
func (b *Benchmark) setup() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the shared_mem_latency benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.initMem()
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

	if b.UseUnifiedMemory {
		b.GSink = b.rt.AllocateUnifiedMemory(
			b.Context, uint64(b.NumBlocks*4))
	} else {
		b.GSink = b.rt.AllocateMemory(
			b.Context, uint64(b.NumBlocks*4))
	}

	// Zero-initialise the sink so unwritten lanes are well defined.
	zeros := make([]float32, b.NumBlocks)
	b.Upload(b.rt, b.Queue, b.GSink, zeros)
}

// launch runs one kernel (no_conflict or conflict) with the configured
// block size as both the work-group dimension and an explicit kernel arg.
func (b *Benchmark) launch(kernel *insts.KernelCodeObject) {
	args := KernelArgs{
		Sink:       b.GSink,
		InnerIters: int32(b.InnerIters),
		BlockSize:  int32(b.BlockSize),
	}
	b.rt.EnqueueLaunchKernel(
		b.Queue,
		kernel,
		[3]uint32{uint32(b.NumBlocks * b.BlockSize), 1, 1},
		[3]uint16{uint16(b.BlockSize), 1, 1},
		&args,
	)
}

// computeNoConflictAcc reproduces, on the CPU, the accumulator that thread 0
// of every block produces in the smem_lat_no_conflict kernel.
//
// In that pattern thread tid touches the disjoint index set
// {tid, tid+n, tid+2n, ...}. For thread 0 with stride n=BlockSize the cells
// are 0, n, 2n, ... < smemFloats, each starting at 1.0. The inner sequence
//
//	acc += smem[i]; smem[i] = acc;
//
// over the cells in order, repeated InnerIters times, is order-independent
// across threads, so it is exactly reproducible here.
func (b *Benchmark) computeNoConflictAcc() float32 {
	n := b.BlockSize

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
// It validates the deterministic smem_lat_no_conflict result (every block's
// thread 0 produces the same accumulator) snapshotted before any conflict
// kernel ran. When the no-conflict kernel was not run (access_pattern =
// "conflict"), there is nothing deterministic to check.
func (b *Benchmark) verify() error {
	if b.NoConflictResult == nil {
		log.Printf("Verify skipped (no_conflict kernel not run).\n")
		return nil
	}

	ref := b.computeNoConflictAcc()

	for blk := 0; blk < b.NumBlocks; blk++ {
		got := b.NoConflictResult[blk]

		denom := math.Abs(float64(ref))
		if denom < 1.0 {
			denom = 1.0
		}
		if math.Abs(float64(ref-got))/denom > 1e-3 {
			return fmt.Errorf("Block %d: expected sink %f, but got %f.\n",
				blk, ref, got)
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if err := b.ReleaseUploads(b.rt); err != nil {
		return err
	}

	switch b.Phase {
	case 0:
		b.setup()
		if b.AccessPattern != "conflict" {
			b.launch(b.hsaco)
		}
		b.Phase = 1
	case 1:
		if b.AccessPattern != "conflict" {
			b.NoConflictResult = make([]float32, b.NumBlocks)
			b.Readback = benchmarks.Download(b.rt, b.Queue, b.GSink, b.NumBlocks*4)
		}
		b.Phase = 2
	case 2:
		if b.Readback.Address != 0 {
			if err := benchmarks.Read(b.rt, b.Readback, b.NoConflictResult); err != nil {
				return err
			}
			b.Readback = gpuruntime.HostBuffer{}
		}
		if b.AccessPattern == "conflict" || b.AccessPattern == "both" {
			b.launch(b.conf)
		}
		b.Phase = 3
	case 3:
		if b.Verification {
			if err := b.verify(); err != nil {
				return err
			}
		}
		b.Ended = true
	}
	return nil
}
