// Package simpleconvolution implements the Simple Convolution benchmark from
// AMDAPPSDK.
package simpleconvolution

import (
	// embed hsaco files
	_ "embed"
	"encoding/binary"
	"fmt"
	"log"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// GCN3KernelArgs defines kernel arguments for GCN3 architecture
type GCN3KernelArgs struct {
	Input                           driver.Ptr
	Mask                            driver.Ptr
	Output                          driver.Ptr
	InputDimensions, MaskDimensions [2]uint32
	NExWidth                        uint32
	Padding                         uint32
	OffsetX, OffsetY, OffsetZ       uint64
}

// CDNA3KernelArgs defines kernel arguments for CDNA3 architecture (GFX942)
type CDNA3KernelArgs struct {
	Input           driver.Ptr // offset 0
	Mask            driver.Ptr // offset 8
	Output          driver.Ptr // offset 16
	InputDimensions [2]uint32  // offset 24 (inputWidth, inputHeight)
	MaskDimensions  [2]uint32  // offset 32 (maskWidth, maskHeight)
	NExWidth        uint32     // offset 40
	Pad             uint32     // offset 44 - alignment padding
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
	benchmarks.Saved[checkpointConfig,

		State,
	]

	driver *driver.Runtime

	kernel *insts.KernelCodeObject
}

// NewBenchmark returns a benchmark
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Saved = benchmarks.NewSaved(b.State, b.checkpointConfig,
		b.bind)

	b.driver = driver
	b.Context = driver.Init()
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
	b.kernel = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "simpleNonSeparableConvolution")
	if b.kernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SetMaskSize sets masksize
func (b *Benchmark) SetMaskSize(maskSize uint32) {
	b.MaskSize = maskSize
	b.PadHeight = maskSize - 1
	b.PadWidth = maskSize - 1
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) setup() {
	b.driver.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.driver.CreateCommandQueue(b.Context)

	b.loadProgram()

	b.driver.SelectGPU(b.Context, b.Gpus[0])
	b.initMem()
}

func (b *Benchmark) initMem() {
	numInputData := (b.Width + b.PadWidth) * (b.Height + b.PadHeight)
	numOutputData := b.Width * b.Height

	b.HInputData = make([]uint32, numInputData)
	b.HOutputData = make([]uint32, numOutputData)
	b.HMask = make([]float32, b.MaskSize*b.MaskSize)

	for i := uint32(0); i < numInputData; i++ {
		// b.hInputData[i] = i
		b.HInputData[i] = 1
	}

	for i := uint32(0); i < b.MaskSize*b.MaskSize; i++ {
		// b.hMask[i] = float32(i)
		b.HMask[i] = float32(1)
	}

	if b.UseUnifiedMemory {
		b.DInputData = b.driver.AllocateUnifiedMemory(b.Context,
			uint64(numInputData*4))
		b.DOutputData = b.driver.AllocateUnifiedMemory(b.Context,
			uint64(numOutputData*4))
	} else {
		b.DInputData = b.driver.AllocateMemory(b.Context,
			uint64(numInputData*4))
		b.driver.Distribute(b.Context, b.DInputData,
			uint64(numInputData*4), b.Gpus)
		b.DOutputData = b.driver.AllocateMemory(b.Context,
			uint64(numOutputData*4))
		b.driver.Distribute(b.Context, b.DOutputData,
			uint64(numOutputData*4), b.Gpus)
	}

	b.DMasks = make([]driver.Ptr, len(b.Gpus))
	for i, gpu := range b.Gpus {
		b.driver.SelectGPU(b.Context, gpu)
		if b.UseUnifiedMemory {
			b.DMasks[i] = b.driver.AllocateUnifiedMemory(
				b.Context,
				uint64(b.MaskSize*b.MaskSize*4))
		} else {
			b.DMasks[i] = b.driver.AllocateMemory(
				b.Context,
				uint64(b.MaskSize*b.MaskSize*4))
		}
		b.driver.EnqueueMemCopyH2D(b.Queue, b.DMasks[i], b.HMask)
	}

	b.driver.EnqueueMemCopyH2D(b.Queue, b.DInputData, b.HInputData)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.DOutputData, b.HOutputData)
}

func (b *Benchmark) enqueueKernel(queue driver.CommandQueue, gpuIndex int, gridSize uint32) {
	if b.Arch == arch.CDNA3 {
		wgSizeX := uint16(64)
		wgSizeY := uint16(1)
		wgSizeZ := uint16(1)

		kernArg := CDNA3KernelArgs{
			Input:           b.DInputData,
			Mask:            b.DMasks[gpuIndex],
			Output:          b.DOutputData,
			InputDimensions: [2]uint32{b.Width, b.Height},
			MaskDimensions:  [2]uint32{b.MaskSize, b.MaskSize},
			NExWidth:        b.Width + b.PadWidth,
			// Hidden kernel arguments for GFX942
			HiddenBlockCountX:   gridSize / uint32(wgSizeX),
			HiddenBlockCountY:   1,
			HiddenBlockCountZ:   1,
			HiddenGroupSizeX:    wgSizeX,
			HiddenGroupSizeY:    wgSizeY,
			HiddenGroupSizeZ:    wgSizeZ,
			HiddenRemainderX:    uint16(gridSize % uint32(wgSizeX)),
			HiddenRemainderY:    0,
			HiddenRemainderZ:    0,
			HiddenGlobalOffsetX: int64(gridSize * uint32(gpuIndex)),
			HiddenGlobalOffsetY: 0,
			HiddenGlobalOffsetZ: 0,
			HiddenGridDims:      1,
		}

		b.driver.EnqueueLaunchKernel(
			queue,
			b.kernel,
			[3]uint32{gridSize, 1, 1},
			[3]uint16{wgSizeX, 1, 1},
			&kernArg,
		)
	} else {
		kernArg := GCN3KernelArgs{
			b.DInputData,
			b.DMasks[gpuIndex],
			b.DOutputData,
			[2]uint32{b.Width, b.Height},
			[2]uint32{b.MaskSize, b.MaskSize},
			b.Width + b.PadWidth,
			0,
			uint64(gridSize * uint32(gpuIndex)), 0, 0,
		}

		b.driver.EnqueueLaunchKernel(
			queue,
			b.kernel,
			[3]uint32{gridSize, 1, 1},
			[3]uint16{uint16(64), 1, 1},
			&kernArg,
		)
	}
}

func (b *Benchmark) exec() {
	queues := make([]driver.CommandQueue, len(b.Gpus))
	gridSize := ((b.Width + b.PadWidth) * (b.Height + b.PadHeight)) / uint32(len(b.Gpus))

	for i, gpu := range b.Gpus {
		b.driver.SelectGPU(b.Context, gpu)
		queues[i] = b.driver.CreateCommandQueue(b.Context)
		b.enqueueKernel(queues[i], i, gridSize)
	}

	b.Readbacks = append(b.Readbacks, benchmarks.Readback{Source: b.DOutputData, Target: 0})
}

// Verify verifies
func (b *Benchmark) verify() error {
	cpuOutputImage := b.cpuSimpleConvolution()

	mismatch := false
	for i := uint32(0); i < b.Height; i++ {
		for j := uint32(0); j < b.Width; j++ {
			index := i*b.Width + j
			gpuOutput := b.HOutputData[index]
			cpuOutput := cpuOutputImage[index]

			if cpuOutput != gpuOutput {
				log.Printf("mismatch as position %d, %d (addr 0x%x). "+
					"Expected %d, but get %d",
					i, j,
					uint64(b.DOutputData)+uint64(4*index),
					cpuOutput, gpuOutput)
				mismatch = true
			}
		}
	}

	if mismatch {
		return fmt.Errorf("verification failed")
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) cpuSimpleConvolution() []uint32 {
	numOutputData := (b.Width + b.PadWidth) * (b.Height + b.PadHeight)
	cpuOutputData := make([]uint32, numOutputData)

	for y := uint32(0); y < b.Height+b.PadHeight; y++ {
		for x := uint32(0); x < b.Width+b.PadWidth; x++ {
			outputIndex := y*b.Width + x
			if x >= b.Width || y >= b.Height {
				break
			}

			sum := float32(0)
			for j := uint32(0); j < b.MaskSize; j++ {
				for i := uint32(0); i < b.MaskSize; i++ {
					maskIndex := j*b.MaskSize + i
					imageIndex := (y+j)*(b.Width+b.PadWidth) + (x + i)

					sum += float32(b.HInputData[imageIndex]) * b.HMask[maskIndex]
				}
			}

			sum += 0.5
			cpuOutputData[outputIndex] = uint32(sum)
		}
	}

	return cpuOutputData
}

func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
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
			b.Readbacks[i].Command = b.driver.EnqueueMemCopyD2H(b.Queue, b.Readbacks[i].Source, binary.Size(dst))
		}
		b.Phase = 3
	case 3:
		for _, copy := range b.Readbacks {
			var dst any
			switch copy.Target {
			case 0:
				dst = b.HOutputData
			}
			if err := b.driver.ReadResult(copy.Command, dst); err != nil {
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
