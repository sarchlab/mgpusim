// Package sharedmembandwidth implements the GPU Benchmarks v1.1 dependent
// 48 KiB LDS read/update/write workload for MI300X (gfx942).
package sharedmembandwidth

import (
	// embed hsaco files
	_ "embed"
	"fmt"
	"log"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

const (
	defaultBlockSize = 256
	smemFloats       = 12288
)

// KernelArgs is the device ABI for both version 1.1 HIP kernels.
type KernelArgs struct {
	Sink       gpuruntime.Ptr
	InnerIters int32
	_          [4]byte
	Launch     benchmarks.GFX942LaunchDimensions
}

// Benchmark defines the shared-memory bandwidth benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]

	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
	conf  *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new shared-memory bandwidth benchmark.
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
	b.Gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) setup() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the shared_mem_bandwidth benchmark ships only a gfx942 " +
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

	if b.AccessPattern != "no_conflict" && b.AccessPattern != "conflict" && b.AccessPattern != "both" {
		panic("shared memory bandwidth: invalid access pattern")
	}

	bytes := uint64(b.NumBlocks * b.BlockSize * 4)
	if b.UseUnifiedMemory {
		b.GSink = b.rt.AllocateUnifiedMemory(b.Context, bytes)
	} else {
		b.GSink = b.rt.AllocateMemory(b.Context, bytes)
	}
}

func (b *Benchmark) launch(kernel *insts.KernelCodeObject) {
	grid := [3]uint32{uint32(b.NumBlocks * b.BlockSize), 1, 1}
	group := [3]uint16{uint16(b.BlockSize), 1, 1}
	args := KernelArgs{
		Sink:       b.GSink,
		InnerIters: int32(b.InnerIters),
		Launch:     benchmarks.NewGFX942LaunchDimensions(grid, group),
	}
	b.rt.EnqueueLaunchKernel(b.Queue, kernel, grid, group, &args)
}

func (b *Benchmark) verify(result []float32) error {
	reference := make([]float32, b.BlockSize)
	// The conflict permutation changes physical addresses but preserves each
	// logical element and all dependencies within its owning work-item.
	byCount := map[int]float32{}
	for lane := range reference {
		count := (smemFloats + b.BlockSize - 1 - lane) / b.BlockSize
		acc, ok := byCount[count]
		if !ok {
			cells := make([]float32, count)
			for i := range cells {
				cells[i] = 1
			}
			for it := 0; it < b.InnerIters; it++ {
				for i, current := range cells {
					acc = 0.5 * (acc + current)
					cells[i] = acc
				}
			}
			byCount[count] = acc
		}
		reference[lane] = acc
	}
	for tid, got := range result {
		want := reference[tid%b.BlockSize]
		if got != want {
			return fmt.Errorf("work-item %d: expected sink %f, got %f", tid, want, got)
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
			b.NoConflictResult = make([]float32, b.NumBlocks*b.BlockSize)
			b.Readback = benchmarks.Download(b.rt, b.Queue, b.GSink, len(b.NoConflictResult)*4)
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
		if !b.Verification {
			b.Ended = true
			return nil
		}
		if b.NoConflictResult != nil {
			if err := b.verify(b.NoConflictResult); err != nil {
				return err
			}
		}
		if b.AccessPattern == "no_conflict" {
			b.Ended = true
			return nil
		}
		b.Readback = benchmarks.Download(b.rt, b.Queue, b.GSink, 4*b.NumBlocks*b.BlockSize)
		b.Phase = 4
	case 4:
		conflict := make([]float32, b.NumBlocks*b.BlockSize)
		if err := benchmarks.Read(b.rt, b.Readback, conflict); err != nil {
			return err
		}
		b.Readback = gpuruntime.HostBuffer{}
		if err := b.verify(conflict); err != nil {
			return err
		}
		b.Ended = true
	}
	return nil
}
