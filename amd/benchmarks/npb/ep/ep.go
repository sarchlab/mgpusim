// Package ep implements the NAS Parallel Benchmarks Embarrassingly Parallel
// (EP) benchmark, ported from sarchlab/gpu_benchmarks (tier2/npb_ep) for the
// MGPUSim MI300A (CDNA3 / gfx942) model.
//
// Each work-item generates one pair of uniform random deviates with a
// per-thread integer LCG, applies the Box-Muller transform, and computes an
// annular bin index in [0, NUM_BINS). The original benchmark uses a global
// atomicAdd to accumulate bin counts; the CDNA3 functional emulator does not
// implement global/FLAT atomics, so the ported kernel instead writes each
// work-item's bin index to a per-thread output array. The host (Verify)
// reproduces the exact per-thread computation and bins the results, yielding
// the same bin histogram as the original benchmark.
//
// The kernel binary is compiled for gfx942 only (see native/), so the
// benchmark must be run with `-arch cdna3` (the MI300A configuration).
package ep

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// numBins is the number of annular bins (matches NUM_BINS in the kernel).
const numBins = 10

// blockSize is the work-group size baked into the kernel as a constant.
const blockSize = 256

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout below is verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 16): a 4-byte by_value int at offset 0, then a
// 4-byte alignment gap, then an 8-byte global_buffer pointer at offset 8.
// mgpusim serializes args with binary.Write, which does not insert alignment
// padding, so the gap is represented by an explicit pad field. The kernel
// uses a constant block size (no blockDim reads), so no hidden ABI arguments
// are emitted.
type KernelArgs struct {
	N      int32      // offset 0
	Pad    uint32     // offset 4 - alignment padding before the pointer
	BinOut driver.Ptr // offset 8
}

// Benchmark defines the NPB EP benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch arch.Type
	N    int

	gBinOut driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new NPB EP benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, "ep_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. EP uses a single GPU.
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
		log.Panic("the npb ep benchmark ships only a gfx942 kernel; " +
			"run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.N <= 0 {
		b.N = 4096
	}

	if b.useUnifiedMemory {
		b.gBinOut = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.N*4))
	} else {
		b.gBinOut = b.driver.AllocateMemory(
			b.context, uint64(b.N*4))
	}
}

func (b *Benchmark) exec() {
	n := b.N
	numWG := uint32((n + blockSize - 1) / blockSize)
	globalX := numWG * blockSize

	args := KernelArgs{
		N:      int32(n),
		BinOut: b.gBinOut,
	}

	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsaco,
		[3]uint32{globalX, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&args,
	)

	b.driver.DrainCommandQueue(b.queue)
}

// lcgNext mirrors the device-side ANSI C LCG (mod 2^32 by overflow).
func lcgNext(seed uint32) uint32 {
	return seed*1103515245 + 12345
}

// cpuBin reproduces, in float32 arithmetic, the exact bin index that the GPU
// kernel computes for a given work-item index.
func cpuBin(idx int) int32 {
	seed := uint32(idx + 1)
	seed = lcgNext(seed)
	seed = lcgNext(seed)

	seed = lcgNext(seed)
	u1 := float32(seed) / 4294967296.0
	seed = lcgNext(seed)
	u2 := float32(seed) / 4294967296.0

	if u1 < 1e-10 {
		u1 = 1e-10
	}

	r := float32(math.Sqrt(float64(-2.0 * float32(math.Log(float64(u1))))))
	theta := 2.0 * float32(math.Pi) * u2
	x1 := r * float32(math.Cos(float64(theta)))
	x2 := r * float32(math.Sin(float64(theta)))

	t := x1*x1 + x2*x2

	bin := int32(float32(math.Sqrt(float64(t))))
	if bin >= numBins {
		bin = numBins - 1
	}
	if bin < 0 {
		bin = 0
	}
	return bin
}

// Verify checks the GPU per-thread bin indices against a CPU reference and
// confirms the resulting bin histogram matches.
func (b *Benchmark) Verify() {
	n := b.N
	gpuBins := make([]int32, n)
	b.driver.MemCopyD2H(b.context, gpuBins, b.gBinOut)

	cpuHist := make([]int64, numBins)
	gpuHist := make([]int64, numBins)

	for i := 0; i < n; i++ {
		expected := cpuBin(i)
		got := gpuBins[i]

		if got < 0 || got >= numBins {
			log.Fatalf("At index %d, GPU produced out-of-range bin %d.\n",
				i, got)
		}

		if expected != got {
			log.Fatalf("At index %d, expected bin %d, but got %d.\n",
				i, expected, got)
		}

		cpuHist[expected]++
		gpuHist[got]++
	}

	for i := 0; i < numBins; i++ {
		if cpuHist[i] != gpuHist[i] {
			log.Fatalf("Bin %d histogram mismatch: CPU=%d, GPU=%d.\n",
				i, cpuHist[i], gpuHist[i])
		}
	}

	log.Printf("Passed!\n")
}
