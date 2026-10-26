// Package memorybandwidth implements the memory_bandwidth microbenchmark,
// ported from sarchlab/gpu_benchmarks (tier1/memory_bandwidth) for the
// MGPUSim MI300X (CDNA3 / gfx942) model.
//
// The original HIP benchmark measures memory bandwidth using host-side
// hipMemcpy in three directions (H2D, D2H, D2D). Only the device-to-device
// (D2D) path performs GPU work: it streams every element of a source buffer
// into a destination buffer. This port models that path with an explicit
// element-wise copy kernel (dst[i] = src[i]), which is the on-device
// equivalent of a device-to-device memcpy and produces a verifiable result.
//
// The kernel binary is compiled for gfx942 only (see native/), so the
// benchmark must be run with `-arch cdna3` (the MI300X configuration).
package memorybandwidth

import (
	// embed hsaco files
	_ "embed"
	"fmt"
	"log"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
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
	*State
	benchmarks.Saved[checkpointConfig, State]

	driver *driver.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new memory_bandwidth benchmark.
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
		cdna3HSACOBytes, "memcpy_d2d_kernel")
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
		log.Panic("the memory_bandwidth benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.driver.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.NumElements <= 0 {
		b.NumElements = 4096
	}

	n := b.NumElements

	b.Src = make([]float32, n)
	for i := 0; i < n; i++ {
		b.Src[i] = float32(i%1000) * 0.5
	}

	if b.UseUnifiedMemory {
		b.GSrc = b.driver.AllocateUnifiedMemory(b.Context, uint64(n*4))
		b.GDst = b.driver.AllocateUnifiedMemory(b.Context, uint64(n*4))
	} else {
		b.GSrc = b.driver.AllocateMemory(b.Context, uint64(n*4))
		b.GDst = b.driver.AllocateMemory(b.Context, uint64(n*4))
	}

	b.Upload(b.driver, b.Queue, b.GSrc, b.Src)
}

func (b *Benchmark) exec() {
	n := b.NumElements
	gridSize := uint32((n + blockSize - 1) / blockSize)
	globalX := gridSize * blockSize

	args := KernelArgs{
		Src:         b.GSrc,
		Dst:         b.GDst,
		NumElements: int32(n),
	}

	b.driver.EnqueueLaunchKernel(
		b.Queue,
		b.hsaco,
		[3]uint32{globalX, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&args,
	)
}

// Verify checks the GPU result against the source buffer. A device-to-device
// copy must reproduce the source exactly.
func (b *Benchmark) verify() error {
	n := b.NumElements
	gpuDst := make([]float32, n)
	if err := benchmarks.Read(b.driver, b.Readbacks[0], gpuDst); err != nil {
		return err
	}

	for i := 0; i < n; i++ {
		if gpuDst[i] != b.Src[i] {
			return fmt.Errorf("At index %d, expected %f, but got %f.\n",
				i, b.Src[i], gpuDst[i])
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.driver, b.Queue, b.GDst, 4*(b.NumElements)))
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
