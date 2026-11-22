// Package memorybandwidth implements the memory_bandwidth microbenchmark,
// ported from sarchlab/gpu_benchmarks (tier1/memory_bandwidth) for the
// MGPUSim MI300X (CDNA3 / gfx942) model.
//
// Measured device-to-device transfers use the ROCclr copy kernel. Host-link
// transfer cases measure HIP and CPU-link behavior outside this benchmark.
package memorybandwidth

import (
	// embed hsaco files
	"bytes"
	_ "embed"
	"fmt"
	"log"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

const (
	blockSize           = 512
	copyGroupLimit      = 304
	alignedElementBytes = 16
)

// KernelArgs is the explicit ABI of the ROCclr copy kernel.
type KernelArgs struct {
	Src           gpuruntime.Ptr
	Dst           gpuruntime.Ptr
	Size          uint64
	Remainder     uint32
	AlignedSize   uint32
	EndPtr        gpuruntime.Ptr
	NextChunk     uint32
	WorkgroupSize uint32
}

// Benchmark defines the memory_bandwidth benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new memory_bandwidth benchmark.
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
		cdna3HSACOBytes, "__amd_rocclr_copyBuffer")
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

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.NumElements <= 0 {
		b.NumElements = 4096
	}

	n := b.NumElements * 4
	b.Src = bytes.Repeat([]byte{0xab}, n)

	if b.UseUnifiedMemory {
		b.GSrc = b.rt.AllocateUnifiedMemory(b.Context, uint64(n))
		b.GDst = b.rt.AllocateUnifiedMemory(b.Context, uint64(n))
	} else {
		b.GSrc = b.rt.AllocateMemory(b.Context, uint64(n))
		b.GDst = b.rt.AllocateMemory(b.Context, uint64(n))
	}

	b.Upload(b.rt, b.Queue, b.GSrc, b.Src)
}

func (b *Benchmark) exec() {
	n := uint64(b.NumElements) * 4
	items := (n + alignedElementBytes - 1) / alignedElementBytes
	groups := min((items+blockSize-1)/blockSize, copyGroupLimit)
	globalX := uint32(groups * blockSize)
	remainder := n % alignedElementBytes

	args := KernelArgs{
		Src:           b.GSrc,
		Dst:           b.GDst,
		Size:          n,
		Remainder:     uint32(remainder),
		AlignedSize:   alignedElementBytes,
		EndPtr:        b.GDst + gpuruntime.Ptr(n-remainder),
		NextChunk:     globalX,
		WorkgroupSize: blockSize,
	}

	b.rt.EnqueueLaunchKernel(
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
	n := b.NumElements * 4
	gpuDst := make([]byte, n)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], gpuDst); err != nil {
		return err
	}

	for i := 0; i < n; i++ {
		if gpuDst[i] != b.Src[i] {
			return fmt.Errorf("at byte %d, expected 0x%02x, got 0x%02x",
				i, b.Src[i], gpuDst[i])
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GDst, 4*(b.NumElements)))
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
