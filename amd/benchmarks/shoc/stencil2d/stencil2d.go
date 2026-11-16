// Package stencil2d implements the stencil2d benchmark from the SHOC suite.
package stencil2d

import (
	// embed hsaco files
	_ "embed"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// GCN3 Kernel Arguments

// CopyRectKernelArgs defines kernel arguments for GCN3
type CopyRectKernelArgs struct {
	Dst                 gpuruntime.Ptr
	DOffset             int32
	DPitch              int32
	Src                 gpuruntime.Ptr
	SOffset             int32
	SPitch              int32
	Width               int32
	Height              int32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// StencilKernelArgs defines kernel arguments for GCN3
type StencilKernelArgs struct {
	Data                gpuruntime.Ptr
	NewData             gpuruntime.Ptr
	Alignment           int32
	WCenter             float32
	WCardinal           float32
	WDiagonal           float32
	Sh                  gpuruntime.LocalPtr
	Padding             int32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// CDNA3 Kernel Arguments

// CDNA3CopyRectKernelArgs defines kernel arguments for CDNA3 architecture (GFX942)
type CDNA3CopyRectKernelArgs struct {
	Dst                 gpuruntime.Ptr
	DOffset             int32
	DPitch              int32
	Src                 gpuruntime.Ptr
	SOffset             int32
	SPitch              int32
	Width               int32
	Height              int32
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Padding             [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

// CDNA3StencilKernelArgs defines kernel arguments for CDNA3 architecture (GFX942).
// The CDNA3 kernel uses __shared__ memory (LDS) instead of a global Sh pointer,
// so there is no Sh parameter in the kernel arguments.
//
// Layout must match the HSACO metadata from kernels_gfx942.hsaco:
//
//	offset 0x00: Data             (8 bytes, global_buffer)
//	offset 0x08: NewData          (8 bytes, global_buffer)
//	offset 0x10: Alignment        (4 bytes, by_value)
//	offset 0x14: WCenter          (4 bytes, by_value)
//	offset 0x18: WCardinal        (4 bytes, by_value)
//	offset 0x1c: WDiagonal        (4 bytes, by_value)
//	offset 0x20: HiddenBlockCountX  (4 bytes)
//	offset 0x24: HiddenBlockCountY  (4 bytes)
//	offset 0x28: HiddenBlockCountZ  (4 bytes)
//	offset 0x2c: HiddenGroupSizeX   (2 bytes)
//	offset 0x2e: HiddenGroupSizeY   (2 bytes)
//	offset 0x30: HiddenGroupSizeZ   (2 bytes)
//	offset 0x32: HiddenRemainderX   (2 bytes)
//	offset 0x34: HiddenRemainderY   (2 bytes)
//	offset 0x36: HiddenRemainderZ   (2 bytes)
//	offset 0x38: (padding, 16 bytes)
//	offset 0x48: HiddenGlobalOffsetX (8 bytes)
//	offset 0x50: HiddenGlobalOffsetY (8 bytes)
//	offset 0x58: HiddenGlobalOffsetZ (8 bytes)
//	offset 0x60: HiddenGridDims      (2 bytes)
type CDNA3StencilKernelArgs struct {
	Data                gpuruntime.Ptr
	NewData             gpuruntime.Ptr
	Alignment           int32
	WCenter             float32
	WCardinal           float32
	WDiagonal           float32
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Padding             [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

// Benchmark defines a benchmark
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	copyRectKernel *insts.KernelCodeObject
	stencilKernel  *insts.KernelCodeObject
}

// NewBenchmark returns a benchmark
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)

	b.rt = rt
	b.Context = rt.Init()
	b.HaloWidth = 1
	b.Pad = 16
	b.LocalRows = 16
	b.LocalCols = 64
	b.WCenter = 0.5
	b.WCardinal = 0.0
	b.WDiagonal = 0.0
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

func (b *Benchmark) loadKernels() {
	var hsacoBytes []byte
	if b.Arch == arch.CDNA3 {
		hsacoBytes = cdna3HSACOBytes
	} else {
		hsacoBytes = gcn3HSACOBytes
	}

	b.copyRectKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "CopyRect")
	if b.copyRectKernel == nil {
		log.Panic("Failed to load CopyRect kernel binary")
	}

	b.stencilKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "StencilKernel")
	if b.stencilKernel == nil {
		log.Panic("Failed to load StencilKernel kernel binary")
	}
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) setup() {
	b.rt.SelectGPU(b.Context, b.Gpus[len(b.Gpus)-1])
	b.Queue = b.rt.CreateCommandQueue(b.Context)

	b.loadKernels()

	b.initMem()
}

func (b *Benchmark) initMem() {
	rand.Seed(1)
	b.NumPaddedCols = ((b.NumCols-1)/b.Pad + 1) * b.Pad
	b.PaddedDataSize = b.NumRows * b.NumPaddedCols

	b.HInput = make([]float32, b.PaddedDataSize)
	b.HOutput = make([]float32, b.PaddedDataSize)
	for i := 0; i < b.PaddedDataSize; i++ {
		// b.hInput[i] = float32(i)
		b.HInput[i] = 1
	}

	if b.UseUnifiedMemory {
		b.DData1 = b.rt.AllocateUnifiedMemory(b.Context,
			uint64(b.PaddedDataSize*4))
		b.DData2 = b.rt.AllocateUnifiedMemory(b.Context,
			uint64(b.PaddedDataSize*4))
	} else {
		b.DData1 = b.rt.AllocateMemory(b.Context,
			uint64(b.PaddedDataSize*4))
		b.DData2 = b.rt.AllocateMemory(b.Context,
			uint64(b.PaddedDataSize*4))
	}

	b.CurrData = b.DData1
	b.NewData = b.DData2
}

func (b *Benchmark) launchStencilKernel(
	globalSize [3]uint32,
	localSize [3]uint16,
	ldsSize int,
) {
	if b.Arch == arch.CDNA3 {
		args := CDNA3StencilKernelArgs{
			Data:                b.CurrData,
			NewData:             b.NewData,
			Alignment:           16,
			WCenter:             b.WCenter,
			WCardinal:           b.WCardinal,
			WDiagonal:           b.WDiagonal,
			HiddenBlockCountX:   globalSize[0] / uint32(localSize[0]),
			HiddenBlockCountY:   globalSize[1] / uint32(localSize[1]),
			HiddenBlockCountZ:   globalSize[2] / uint32(localSize[2]),
			HiddenGroupSizeX:    localSize[0],
			HiddenGroupSizeY:    localSize[1],
			HiddenGroupSizeZ:    localSize[2],
			HiddenRemainderX:    uint16(globalSize[0] % uint32(localSize[0])),
			HiddenRemainderY:    uint16(globalSize[1] % uint32(localSize[1])),
			HiddenRemainderZ:    uint16(globalSize[2] % uint32(localSize[2])),
			HiddenGlobalOffsetX: 0,
			HiddenGlobalOffsetY: 0,
			HiddenGlobalOffsetZ: 0,
			HiddenGridDims:      2,
		}
		b.rt.EnqueueLaunchKernel(b.Queue,
			b.stencilKernel,
			globalSize, localSize,
			&args,
		)
	} else {
		args := StencilKernelArgs{
			Data:                b.CurrData,
			NewData:             b.NewData,
			Alignment:           16,
			WCenter:             b.WCenter,
			WCardinal:           b.WCardinal,
			WDiagonal:           b.WDiagonal,
			Sh:                  gpuruntime.LocalPtr(ldsSize),
			Padding:             0,
			HiddenGlobalOffsetX: 0,
			HiddenGlobalOffsetY: 0,
			HiddenGlobalOffsetZ: 0,
		}
		b.rt.EnqueueLaunchKernel(b.Queue,
			b.stencilKernel,
			globalSize, localSize,
			&args,
		)
	}
}

func (b *Benchmark) exec() {
	b.Upload(b.rt, b.Queue, b.CurrData, b.HInput)
	b.Upload(b.rt, b.Queue, b.NewData, b.HInput)

	for i := 0; i < b.NumIteration; i++ {
		ldsSize := (b.LocalRows + 2) * (b.LocalCols + 2) * 4

		globalSize := [3]uint32{
			uint32((b.NumRows - 2) / b.LocalRows),
			uint32(b.NumCols - 2),
			1,
		}
		localSize := [3]uint16{1, uint16(b.LocalCols), 1}

		b.launchStencilKernel(globalSize, localSize, ldsSize)
		b.CurrData, b.NewData = b.NewData, b.CurrData
	}

	b.Readbacks = append(b.Readbacks, benchmarks.Readback{Source: b.CurrData, Target: 0})
}

// Verify verfies
func (b *Benchmark) verify() error {
	cpuOutput := b.cpuStencil2D()

	mismatch := false
	for x := 0; x < b.NumRows; x++ {
		for y := 0; y < b.NumCols; y++ {
			index := x*b.NumPaddedCols + y
			if b.HOutput[index] != cpuOutput[index] {
				mismatch = true
				log.Printf("not match at (%d,%d), expected %f to equal %f\n",
					x, y,
					b.HOutput[index], cpuOutput[index])
			}
		}
	}

	if mismatch {
		return fmt.Errorf("Mismatch!\n")
	}
	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) cpuStencil2D() []float32 {
	cpuOutput := make([]float32, b.PaddedDataSize)
	for x := 0; x < b.NumRows; x++ {
		for y := 0; y < b.NumCols; y++ {
			cpuOutput[x*b.NumPaddedCols+y] =
				b.HInput[x*b.NumPaddedCols+y]
		}
	}

	for i := 0; i < b.NumIteration; i++ {
		for x := 0; x < b.NumRows; x++ {
			for y := 0; y < b.NumCols; y++ {
				if x == 0 || y == 0 ||
					x == b.NumRows-1 || y == b.NumCols-1 {
					continue
				}

				center := cpuOutput[x*b.NumPaddedCols+y]
				cardinal := cpuOutput[(x-1)*b.NumPaddedCols+y] +
					cpuOutput[(x+1)*b.NumPaddedCols+y] +
					cpuOutput[x*b.NumPaddedCols+(y+1)] +
					cpuOutput[x*b.NumPaddedCols+(y-1)]
				diagonal := cpuOutput[(x-1)*b.NumPaddedCols+(y+1)] +
					cpuOutput[(x+1)*b.NumPaddedCols+(y-1)] +
					cpuOutput[(x+1)*b.NumPaddedCols+(y+1)] +
					cpuOutput[(x-1)*b.NumPaddedCols+(y-1)]

				out := b.WCenter*center +
					b.WCardinal*cardinal +
					b.WDiagonal*diagonal

				cpuOutput[x*b.NumPaddedCols+y] = out
			}
		}
	}

	return cpuOutput
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
				dst = b.HOutput
			}
			b.Readbacks[i].Buffer = benchmarks.Download(b.rt, b.Queue, b.Readbacks[i].Source, binary.Size(dst))
		}
		b.Phase = 3
	case 3:
		for _, copy := range b.Readbacks {
			var dst any
			switch copy.Target {
			case 0:
				dst = b.HOutput
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
