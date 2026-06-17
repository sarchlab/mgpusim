// Package mixbench implements the mixbench parametric roofline microbenchmark,
// ported from sarchlab/gpu_benchmarks (tier1/mixbench) for the MGPUSim MI300A
// (CDNA3 / gfx942) model.
//
// Each work-item loads one float from global memory, runs a dependency chain
// of NumFmas FP32 FMA operations (val = fmaf(val, mul, add)), and stores the
// result back. The kernel binary is compiled for gfx942 only (see native/),
// so the benchmark must be run with `-arch cdna3` (the MI300A configuration).
package mixbench

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// blockSize must match BLOCK_SIZE in native/mixbench.cpp.
const blockSize = 256

// FMA constants must match the kernel exactly.
const (
	fmaMul float32 = 1.0000001
	fmaAdd float32 = 0.0000001
)

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout below is verified against the compiled kernel's AMDGPU
// metadata (kernarg_segment_size = 16): one 8-byte global_buffer pointer
// followed by two 4-byte by_value int32 scalars, packed with no padding
// (mgpusim serializes args with binary.Write, which does not insert
// alignment padding). The kernel uses a constant BLOCK_SIZE instead of
// blockDim.x, so no hidden ABI arguments are emitted.
type KernelArgs struct {
	Data        driver.Ptr // offset 0
	NumElements int32      // offset 8
	NumFmas     int32      // offset 12
}

// Benchmark defines the mixbench benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch arch.Type
	// NumElements is the number of floats processed (one per work-item).
	NumElements int
	// NumFmas is the FP32 FMA-chain length per work-item.
	NumFmas int

	data  []float32
	gData driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new mixbench benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "mixbench_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. mixbench uses a single GPU.
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
		log.Panic("the mixbench benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.NumElements <= 0 {
		b.NumElements = 4096
	}
	if b.NumFmas <= 0 {
		b.NumFmas = 16
	}

	n := b.NumElements
	b.data = make([]float32, n)
	for i := 0; i < n; i++ {
		// Deterministic init, mirroring the HIP host: 1.0 + (i%1024)*0.001.
		b.data[i] = 1.0 + float32(i%1024)*0.001
	}

	if b.useUnifiedMemory {
		b.gData = b.driver.AllocateUnifiedMemory(b.context, uint64(n*4))
	} else {
		b.gData = b.driver.AllocateMemory(b.context, uint64(n*4))
	}

	b.driver.MemCopyH2D(b.context, b.gData, b.data)
}

func (b *Benchmark) exec() {
	n := b.NumElements
	numBlocks := uint32((n + blockSize - 1) / blockSize)
	globalX := numBlocks * blockSize

	args := KernelArgs{
		Data:        b.gData,
		NumElements: int32(n),
		NumFmas:     int32(b.NumFmas),
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

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) Verify() {
	n := b.NumElements
	gpuData := make([]float32, n)
	b.driver.MemCopyD2H(b.context, gpuData, b.gData)

	for i := 0; i < n; i++ {
		// Reproduce the kernel's FMA chain in float32 exactly.
		val := b.data[i]
		for f := 0; f < b.NumFmas; f++ {
			val = float32(math.FMA(
				float64(val), float64(fmaMul), float64(fmaAdd)))
		}
		ref := val
		got := gpuData[i]

		denom := math.Abs(float64(ref))
		if denom < 1.0 {
			denom = 1.0
		}
		if math.Abs(float64(ref-got))/denom > 1e-3 {
			log.Fatalf("At index %d, expected %f, but got %f.\n",
				i, ref, got)
		}
	}

	log.Printf("Passed!\n")
}
