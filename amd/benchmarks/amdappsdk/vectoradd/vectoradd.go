// Package vectoradd implements the VectorAdd benchmark from AMDAPPSDK.
package vectoradd

import (
	// embed hsaco files
	_ "embed"
	"encoding/binary"
	"fmt"
	"log"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// KernelArgs defines kernel arguments
// Layout must match AMDGPU hidden kernel argument format for GFX942
type KernelArgs struct {
	A      gpuruntime.Ptr // offset 0 - 8 bytes
	B      gpuruntime.Ptr // offset 8 - 8 bytes
	C      gpuruntime.Ptr // offset 16 - 8 bytes
	Width  int32          // offset 24 - 4 bytes
	Height int32          // offset 28 - 4 bytes
	// Hidden kernel arguments (required by HIP runtime for GFX942)
	HiddenBlockCountX   uint32   // offset 32 - number of workgroups in X
	HiddenBlockCountY   uint32   // offset 36 - number of workgroups in Y
	HiddenBlockCountZ   uint32   // offset 40 - number of workgroups in Z
	HiddenGroupSizeX    uint16   // offset 44 - workgroup size X
	HiddenGroupSizeY    uint16   // offset 46 - workgroup size Y
	HiddenGroupSizeZ    uint16   // offset 48 - workgroup size Z
	HiddenRemainderX    uint16   // offset 50 - grid size % workgroup size X
	HiddenRemainderY    uint16   // offset 52 - grid size % workgroup size Y
	HiddenRemainderZ    uint16   // offset 54 - grid size % workgroup size Z
	Padding             [16]byte // offset 56-71 - reserved
	HiddenGlobalOffsetX int64    // offset 72 - global offset X
	HiddenGlobalOffsetY int64    // offset 80 - global offset Y
	HiddenGlobalOffsetZ int64    // offset 88 - global offset Z
	HiddenGridDims      uint16   // offset 96 - grid dimensions
}

// Benchmark defines a benchmark
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	kernel *insts.KernelCodeObject
}

// NewBenchmark returns a benchmark
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)

	b.rt = rt
	b.Context = rt.Init()
	b.loadProgram()
	return b
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.Gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

//go:embed kernels.hsaco
var hsacoBytes []byte

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "_Z15vectoradd_floatPfPKfS1_ii")
	if b.kernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) setup() {
	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.initMem()
}

func (b *Benchmark) initMem() {
	numData := b.Width * b.Height

	b.HA = make([]float32, numData)
	b.HB = make([]float32, numData)
	b.HC = make([]float32, numData)

	for i := uint32(0); i < numData; i++ {
		b.HB[i] = float32(i)
		b.HC[i] = float32(i) * 100.0
	}

	if b.UseUnifiedMemory {
		b.DA = b.rt.AllocateUnifiedMemory(b.Context, uint64(numData*4))
		b.DB = b.rt.AllocateUnifiedMemory(b.Context, uint64(numData*4))
		b.DC = b.rt.AllocateUnifiedMemory(b.Context, uint64(numData*4))
	} else {
		b.DA = b.rt.AllocateMemory(b.Context, uint64(numData*4))
		b.rt.Distribute(b.Context, b.DA, uint64(numData*4), b.Gpus)
		b.DB = b.rt.AllocateMemory(b.Context, uint64(numData*4))
		b.rt.Distribute(b.Context, b.DB, uint64(numData*4), b.Gpus)
		b.DC = b.rt.AllocateMemory(b.Context, uint64(numData*4))
		b.rt.Distribute(b.Context, b.DC, uint64(numData*4), b.Gpus)
	}

	b.Upload(b.rt, b.Queue, b.DB, b.HB)
	b.Upload(b.rt, b.Queue, b.DC, b.HC)
}

func (b *Benchmark) exec() {
	queues := make([]gpuruntime.CommandQueue, len(b.Gpus))
	numData := b.Width * b.Height

	// Workgroup size is fixed at 64x1x1
	wgSizeX := uint16(64)
	wgSizeY := uint16(1)
	wgSizeZ := uint16(1)

	for i, gpu := range b.Gpus {
		b.rt.SelectGPU(b.Context, gpu)
		queues[i] = b.rt.CreateCommandQueue(b.Context)

		gridSize := numData / uint32(len(b.Gpus))

		kernArg := KernelArgs{
			A:      b.DA,
			B:      b.DB,
			C:      b.DC,
			Width:  int32(b.Width),
			Height: int32(b.Height),
			// Hidden kernel arguments for GFX942 thread ID calculation
			HiddenBlockCountX:   gridSize / uint32(wgSizeX),
			HiddenBlockCountY:   1,
			HiddenBlockCountZ:   1,
			HiddenGroupSizeX:    wgSizeX,
			HiddenGroupSizeY:    wgSizeY,
			HiddenGroupSizeZ:    wgSizeZ,
			HiddenGlobalOffsetX: int64(gridSize * uint32(i)),
			HiddenGlobalOffsetY: 0,
			HiddenGlobalOffsetZ: 0,
		}

		b.rt.EnqueueLaunchKernel(
			queues[i],
			b.kernel,
			[3]uint32{gridSize, 1, 1},
			[3]uint16{64, 1, 1},
			&kernArg,
		)
	}

	b.Readbacks = append(b.Readbacks, benchmarks.Readback{Source: b.DA, Target: 0})
}

// Verify verifies
func (b *Benchmark) verify() error {
	numData := b.Width * b.Height
	mismatch := false

	for i := uint32(0); i < numData; i++ {
		expected := b.HB[i] + b.HC[i]
		if b.HA[i] != expected {
			log.Printf("mismatch at position %d. Expected %f, but got %f",
				i, expected, b.HA[i])
			mismatch = true
		}
	}

	if mismatch {
		return fmt.Errorf("verification failed")
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
		b.Phase = 1
	case 1:
		b.exec()
		b.Phase = 2
	case 2:
		for i := range b.Readbacks {
			var dst any
			switch b.Readbacks[i].Target {
			case 0:
				dst = b.HA
			}
			b.Readbacks[i].Buffer = benchmarks.Download(b.rt, b.Queue, b.Readbacks[i].Source, binary.Size(dst))
		}
		b.Phase = 3
	case 3:
		for _, copy := range b.Readbacks {
			var dst any
			switch copy.Target {
			case 0:
				dst = b.HA
			}
			if err := benchmarks.Read(b.rt, copy.Buffer, dst); err != nil {
				return err
			}
		}
		if b.Verification {
			if err := b.verify(); err != nil {
				return err
			}
		}
		b.Readbacks = nil
		b.Ended = true
	}
	return nil
}
