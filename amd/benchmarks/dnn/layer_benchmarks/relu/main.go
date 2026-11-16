// Package relu implements the relu algorithm as a benchmark.
package relu

import (
	// embed hsaco files
	_ "embed"
	"encoding/binary"
	"fmt"
	"log"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// KernelArgs defines kernel arguments for GCN3
type KernelArgs struct {
	Count               uint32
	Padding             uint32
	Input               gpuruntime.Ptr
	Output              gpuruntime.Ptr
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// CDNA3KernelArgs defines kernel arguments for CDNA3 (gfx942)
// Total size: 280 bytes matching HSACO metadata
type CDNA3KernelArgs struct {
	Count               uint32         // offset 0, size 4
	Pad0                uint32         // offset 4 (alignment padding to reach offset 8)
	Input               gpuruntime.Ptr // offset 8, size 8
	Output              gpuruntime.Ptr // offset 16, size 8
	HiddenBlockCountX   uint32         // offset 24, size 4
	HiddenBlockCountY   uint32         // offset 28, size 4
	HiddenBlockCountZ   uint32         // offset 32, size 4
	HiddenGroupSizeX    uint16         // offset 36, size 2
	HiddenGroupSizeY    uint16         // offset 38, size 2
	HiddenGroupSizeZ    uint16         // offset 40, size 2
	HiddenRemainderX    uint16         // offset 42, size 2
	HiddenRemainderY    uint16         // offset 44, size 2
	HiddenRemainderZ    uint16         // offset 46, size 2
	Pad1                [16]byte       // offset 48, padding to reach offset 64
	HiddenGlobalOffsetX int64          // offset 64, size 8
	HiddenGlobalOffsetY int64          // offset 72, size 8
	HiddenGlobalOffsetZ int64          // offset 80, size 8
	HiddenGridDims      uint16         // offset 88, size 2
	Pad2                [190]byte      // offset 90, padding to reach 280 bytes total
}

// Benchmark defines a benchmark
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels.hsaco
var gcn3HSACOBytes []byte

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a benchmark
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)

	b.rt = rt
	b.Context = rt.Init()

	return b
}

func (b *Benchmark) loadProgram() {
	var hsacoBytes []byte
	if b.Arch == arch.CDNA3 {
		hsacoBytes = cdna3HSACOBytes
	} else {
		hsacoBytes = gcn3HSACOBytes
	}
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "ReLUForward")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.Gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) setup() {
	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)

	b.loadProgram()
	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.initMem()
}

func (b *Benchmark) initMem() {
	if b.UseUnifiedMemory {
		b.GInputData = b.rt.AllocateUnifiedMemory(b.Context,
			uint64(b.Length*4))
		b.GOutputData = b.rt.AllocateUnifiedMemory(b.Context,
			uint64(b.Length*4))
	} else {
		b.GInputData = b.rt.AllocateMemory(b.Context, uint64(b.Length*4))
		b.rt.Distribute(b.Context, b.GInputData, uint64(b.Length*4), b.Gpus)

		b.GOutputData = b.rt.AllocateMemory(b.Context, uint64(b.Length*4))
		b.rt.Distribute(b.Context, b.GOutputData,
			uint64(b.Length*4), b.Gpus)
	}

	b.InputData = make([]float32, b.Length)
	b.OutputData = make([]float32, b.Length)
	for i := 0; i < b.Length; i++ {
		b.InputData[i] = float32(i) - 0.5
	}

	b.Upload(b.rt, b.Queue, b.GInputData, b.InputData)
}

func (b *Benchmark) createKernelArgs(
	globalSize [3]uint32,
	localSize [3]uint16,
	globalOffsetX int64,
) interface{} {
	if b.Arch == arch.CDNA3 {
		// Calculate grid dimensions for CDNA3
		gridDimX := (globalSize[0] + uint32(localSize[0]) - 1) / uint32(localSize[0])
		gridDimY := (globalSize[1] + uint32(localSize[1]) - 1) / uint32(localSize[1])
		gridDimZ := (globalSize[2] + uint32(localSize[2]) - 1) / uint32(localSize[2])

		remainderX := globalSize[0] % uint32(localSize[0])
		remainderY := globalSize[1] % uint32(localSize[1])
		remainderZ := globalSize[2] % uint32(localSize[2])

		return &CDNA3KernelArgs{
			Count:               uint32(b.Length),
			Pad0:                0,
			Input:               b.GInputData,
			Output:              b.GOutputData,
			HiddenBlockCountX:   gridDimX,
			HiddenBlockCountY:   gridDimY,
			HiddenBlockCountZ:   gridDimZ,
			HiddenGroupSizeX:    localSize[0],
			HiddenGroupSizeY:    localSize[1],
			HiddenGroupSizeZ:    localSize[2],
			HiddenRemainderX:    uint16(remainderX),
			HiddenRemainderY:    uint16(remainderY),
			HiddenRemainderZ:    uint16(remainderZ),
			Pad1:                [16]byte{},
			HiddenGlobalOffsetX: globalOffsetX,
			HiddenGlobalOffsetY: 0,
			HiddenGlobalOffsetZ: 0,
			HiddenGridDims:      1,
		}
	}

	return &KernelArgs{
		Count:               uint32(b.Length),
		Padding:             0,
		Input:               b.GInputData,
		Output:              b.GOutputData,
		HiddenGlobalOffsetX: globalOffsetX,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
	}
}

func (b *Benchmark) exec() {
	queues := make([]gpuruntime.CommandQueue, len(b.Gpus))

	for i, gpu := range b.Gpus {
		b.rt.SelectGPU(b.Context, gpu)
		q := b.rt.CreateCommandQueue(b.Context)
		queues[i] = q

		numWI := b.Length / len(b.Gpus)
		globalSize := [3]uint32{uint32(numWI), 1, 1}
		localSize := [3]uint16{64, 1, 1}

		kernArg := b.createKernelArgs(globalSize, localSize, int64(numWI*i))

		b.rt.EnqueueLaunchKernel(
			q,
			b.hsaco,
			globalSize,
			localSize,
			kernArg,
		)
	}

	b.Readbacks = append(b.Readbacks, benchmarks.Readback{Source: b.GOutputData, Target: 0})
}

// Verify verifies
func (b *Benchmark) verify() error {
	for i := 0; i < b.Length; i++ {
		if b.InputData[i] > 0 && b.OutputData[i] != b.InputData[i] {
			return fmt.Errorf("mismatch at %d, input %f, output %f", i,
				b.InputData[i], b.OutputData[i])
		}

		if b.InputData[i] <= 0 && b.OutputData[i] != 0 {
			return fmt.Errorf("mismatch at %d, input %f, output %f", i,
				b.InputData[i], b.OutputData[i])
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
		b.Phase = 1
	case 1:
		b.exec()
		b.Phase = 2
	case 2:
		for i := range b.Readbacks {
			var dst any
			switch b.Readbacks[i].Target {
			case 0:
				dst = b.OutputData
			}
			b.Readbacks[i].Buffer = benchmarks.Download(b.rt, b.Queue, b.Readbacks[i].Source, binary.Size(dst))
		}
		b.Phase = 3
	case 3:
		for _, copy := range b.Readbacks {
			var dst any
			switch copy.Target {
			case 0:
				dst = b.OutputData
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
