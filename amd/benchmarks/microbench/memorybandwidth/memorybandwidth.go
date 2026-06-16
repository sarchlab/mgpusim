// Package memorybandwidth implements the memory_bandwidth microbenchmark,
// ported from sarchlab/gpu_benchmarks (tier1/memory_bandwidth) for the
// MGPUSim MI300A (CDNA3 / gfx942) model.
//
// The original HIP benchmark measures memory bandwidth using host-side
// hipMemcpy in three directions (H2D, D2H, D2D). Only the device-to-device
// (D2D) path performs GPU work: it streams every element of a source buffer
// into a destination buffer. This port models that path with an explicit
// element-wise copy kernel (dst[i] = src[i]), which is the on-device
// equivalent of a device-to-device memcpy and produces a verifiable result.
//
// The kernel binary is compiled for gfx942 only (see native/), so the
// benchmark must be run with `-arch cdna3` (the MI300A configuration).
package memorybandwidth

import (
	"log"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// blockSize matches the constant BLOCK_SIZE in native/memory_bandwidth.cpp.
const blockSize = 256

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout below is verified against the compiled kernel's AMDGPU
// metadata (kernarg_segment_size = 20): two 8-byte global_buffer pointers
// (src at offset 0, dst at offset 8) followed by one 4-byte by_value int
// (num_elements at offset 16), packed with no padding (mgpusim serializes
// args with binary.Write, which does not insert alignment padding). The
// kernel uses a constant block size, so no hidden ABI arguments are emitted.
type KernelArgs struct {
	Src         driver.Ptr // offset 0
	Dst         driver.Ptr // offset 8
	NumElements int32      // offset 16
}

// Benchmark defines the memory_bandwidth benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch arch.Type
	// NumElements is the number of float32 elements copied device-to-device.
	NumElements int

	src  []float32
	gSrc driver.Ptr
	gDst driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new memory_bandwidth benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "memcpy_d2d_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
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
		log.Panic("the memory_bandwidth benchmark ships only a gfx942 " +
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

	n := b.NumElements

	b.src = make([]float32, n)
	for i := 0; i < n; i++ {
		b.src[i] = float32(i%1000) * 0.5
	}

	if b.useUnifiedMemory {
		b.gSrc = b.driver.AllocateUnifiedMemory(b.context, uint64(n*4))
		b.gDst = b.driver.AllocateUnifiedMemory(b.context, uint64(n*4))
	} else {
		b.gSrc = b.driver.AllocateMemory(b.context, uint64(n*4))
		b.gDst = b.driver.AllocateMemory(b.context, uint64(n*4))
	}

	b.driver.MemCopyH2D(b.context, b.gSrc, b.src)
}

func (b *Benchmark) exec() {
	n := b.NumElements
	gridSize := uint32((n + blockSize - 1) / blockSize)
	globalX := gridSize * blockSize

	args := KernelArgs{
		Src:         b.gSrc,
		Dst:         b.gDst,
		NumElements: int32(n),
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

// Verify checks the GPU result against the source buffer. A device-to-device
// copy must reproduce the source exactly.
func (b *Benchmark) Verify() {
	n := b.NumElements
	gpuDst := make([]float32, n)
	b.driver.MemCopyD2H(b.context, gpuDst, b.gDst)

	for i := 0; i < n; i++ {
		if gpuDst[i] != b.src[i] {
			log.Fatalf("At index %d, expected %f, but got %f.\n",
				i, b.src[i], gpuDst[i])
		}
	}

	log.Printf("Passed!\n")
}
