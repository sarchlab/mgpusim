// Package matrixtranspose implements the matrix transpose benchmark from
// AMDAPPSDK.
package matrixtranspose

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

// GCN3KernelArgs defines kernel arguments for GCN3 architecture
type GCN3KernelArgs struct {
	Output              gpuruntime.Ptr
	Input               gpuruntime.Ptr
	Block               gpuruntime.LocalPtr
	WIWidth             uint32
	WIHeight            uint32
	NumWGWidth          uint32
	GroupXOffset        uint32
	GroupYOffset        uint32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// CDNA3KernelArgs defines kernel arguments for CDNA3 architecture (GFX942)
type CDNA3KernelArgs struct {
	Output       gpuruntime.Ptr      // offset 0
	Input        gpuruntime.Ptr      // offset 8
	Block        gpuruntime.LocalPtr // offset 16 (LDS allocation for HIP_DYNAMIC_SHARED)
	BlockPad     uint32              // offset 20 (pad to match 8-byte ptr in HSACO)
	WIWidth      uint32              // offset 24
	WIHeight     uint32              // offset 28
	NumWGWidth   uint32              // offset 32
	GroupXOffset uint32              // offset 36
	GroupYOffset uint32              // offset 40
	Pad          uint32              // offset 44 - alignment padding
	// Hidden kernel arguments (required by HIP runtime for GFX942)
	HiddenBlockCountX   uint32   // offset 48
	HiddenBlockCountY   uint32   // offset 52
	HiddenBlockCountZ   uint32   // offset 56
	HiddenGroupSizeX    uint16   // offset 60
	HiddenGroupSizeY    uint16   // offset 62
	HiddenGroupSizeZ    uint16   // offset 64
	HiddenRemainderX    uint16   // offset 66
	HiddenRemainderY    uint16   // offset 68
	HiddenRemainderZ    uint16   // offset 70
	Padding             [16]byte // offset 72-87 - reserved
	HiddenGlobalOffsetX int64    // offset 88
	HiddenGlobalOffsetY int64    // offset 96
	HiddenGlobalOffsetZ int64    // offset 104
	HiddenGridDims      uint16   // offset 112
}

// Benchmark defines a benchmark
type Benchmark struct {
	*State
	benchmarks.Checkpoint[checkpointConfig, State]
	rt *gpuruntime.Runtime

	kernel *insts.KernelCodeObject
}

// NewBenchmark makes a new benchmark
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.checkpointConfig, b.bind)

	b.rt = rt
	b.Context = rt.Init()
	b.ElemsPerThread1Dim = 4
	b.BlockSize = 16
	return b
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.Gpus = gpus
}

// SetUnifiedMemory use Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

//go:embed kernels.hsaco
var gcn3HSACOBytes []byte

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

func (b *Benchmark) loadProgram() {
	var hsacoBytes []byte
	if b.Arch == arch.CDNA3 {
		hsacoBytes = cdna3HSACOBytes
	} else {
		hsacoBytes = gcn3HSACOBytes
	}
	b.kernel = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "matrixTranspose")
	if b.kernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) setup() {
	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)

	b.loadProgram()

	for _, gpu := range b.Gpus {
		b.rt.SelectGPU(b.Context, gpu)
		b.Queues = append(b.Queues, b.rt.CreateCommandQueue(b.Context))
	}

	b.initMem()
}

func (b *Benchmark) initMem() {
	numData := b.Width * b.Width

	b.HInputData = make([]uint32, numData)
	b.HOutputData = make([]uint32, numData)

	for i := 0; i < numData; i++ {
		b.HInputData[i] = uint32(i)
	}

	if b.UseUnifiedMemory {
		b.DInputData = b.rt.AllocateUnifiedMemory(
			b.Context, uint64(numData*4))
		b.DOutputData = b.rt.AllocateUnifiedMemory(
			b.Context, uint64(numData*4))
	} else {
		b.DInputData = b.rt.AllocateMemory(
			b.Context, uint64(numData*4))
		b.DOutputData = b.rt.AllocateMemory(
			b.Context, uint64(numData*4))
		b.rt.Distribute(b.Context, b.DInputData, uint64(numData*4), b.Gpus)
		b.rt.Distribute(b.Context, b.DOutputData, uint64(numData*4), b.Gpus)
	}

	b.Upload(b.rt, b.Queue, b.DInputData, b.HInputData)
}

func (b *Benchmark) createCDNA3KernelArgs(
	blockPtr gpuruntime.LocalPtr,
	wiWidth, wiHeight, numWGWidth, wgXPerGPU uint32,
	wiWidthPerGPU, gpuIndex int,
) CDNA3KernelArgs {
	wgSizeX := uint16(b.BlockSize)
	wgSizeY := uint16(b.BlockSize)
	wgSizeZ := uint16(1)
	gridSizeX := uint32(wiWidthPerGPU)
	gridSizeY := wiHeight

	return CDNA3KernelArgs{
		Output:              b.DOutputData,
		Input:               b.DInputData,
		Block:               blockPtr,
		WIWidth:             wiWidth,
		WIHeight:            wiHeight,
		NumWGWidth:          numWGWidth,
		GroupXOffset:        wgXPerGPU * uint32(gpuIndex),
		GroupYOffset:        0,
		HiddenBlockCountX:   gridSizeX / uint32(wgSizeX),
		HiddenBlockCountY:   gridSizeY / uint32(wgSizeY),
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    wgSizeX,
		HiddenGroupSizeY:    wgSizeY,
		HiddenGroupSizeZ:    wgSizeZ,
		HiddenRemainderX:    uint16(gridSizeX % uint32(wgSizeX)),
		HiddenRemainderY:    uint16(gridSizeY % uint32(wgSizeY)),
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      2,
	}
}

func (b *Benchmark) enqueueKernel(
	queue gpuruntime.CommandQueue,
	wiWidth, wiHeight, numWGWidth, wgXPerGPU uint32,
	wiWidthPerGPU int,
	gpuIndex int,
) {
	blockPtr := gpuruntime.LocalPtr(b.BlockSize * b.BlockSize *
		b.ElemsPerThread1Dim * b.ElemsPerThread1Dim * 4)

	if b.Arch == arch.CDNA3 {
		kernArg := b.createCDNA3KernelArgs(blockPtr, wiWidth, wiHeight, numWGWidth, wgXPerGPU, wiWidthPerGPU, gpuIndex)
		wgSizeX := uint16(b.BlockSize)
		wgSizeY := uint16(b.BlockSize)
		gridSizeX := uint32(wiWidthPerGPU)
		gridSizeY := wiHeight

		b.rt.EnqueueLaunchKernel(
			queue,
			b.kernel,
			[3]uint32{gridSizeX, gridSizeY, 1},
			[3]uint16{wgSizeX, wgSizeY, 1},
			&kernArg,
		)
	} else {
		kernArg := GCN3KernelArgs{
			b.DOutputData,
			b.DInputData,
			blockPtr,
			wiWidth, wiHeight, numWGWidth,
			wgXPerGPU * uint32(gpuIndex), 0,
			0, 0, 0,
		}

		b.rt.EnqueueLaunchKernel(
			queue,
			b.kernel,
			[3]uint32{uint32(wiWidthPerGPU), wiHeight, 1},
			[3]uint16{uint16(b.BlockSize), uint16(b.BlockSize), 1},
			&kernArg,
		)
	}
}

func (b *Benchmark) exec() {
	wiWidth := uint32(b.Width / b.ElemsPerThread1Dim)
	wiHeight := uint32(b.Width / b.ElemsPerThread1Dim)
	numWGWidth := wiWidth / uint32(b.BlockSize)
	wgXPerGPU := numWGWidth / uint32(len(b.Queues))

	for i, queue := range b.Queues {
		wiWidthPerGPU := int(wiWidth) / len(b.Queues)
		b.enqueueKernel(queue, wiWidth, wiHeight, numWGWidth, wgXPerGPU, wiWidthPerGPU, i)
	}

	b.Readbacks = append(b.Readbacks, benchmarks.Readback{Source: b.DOutputData, Target: 0})
}

// Verify verifies
func (b *Benchmark) verify() error {
	failed := false
	for i := 0; i < b.Width; i++ {
		for j := 0; j < b.Width; j++ {
			actual := b.HOutputData[i*b.Width+j]
			expected := b.HInputData[j*b.Width+i]
			if expected != actual {
				log.Printf("mismatch at (%d, %d), expected %d, but get %d\n",
					i, j, expected, actual)
				failed = true
			}
		}
	}

	if failed {
		return fmt.Errorf("failed to verify matrix transpose result")
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
				dst = b.HOutputData
			}
			b.Readbacks[i].Buffer = benchmarks.Download(b.rt, b.Queue, b.Readbacks[i].Source, binary.Size(dst))
		}
		b.Phase = 3
	case 3:
		for _, copy := range b.Readbacks {
			var dst any
			switch copy.Target {
			case 0:
				dst = b.HOutputData
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
