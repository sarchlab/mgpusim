// Package stencil2d implements the stencil2d benchmark from the SHOC suite.
package stencil2d

import (
	"encoding/binary"
	"fmt"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// GCN3 Kernel Arguments

// CopyRectKernelArgs defines kernel arguments for GCN3
type CopyRectKernelArgs struct {
	Dst                 driver.Ptr
	DOffset             int32
	DPitch              int32
	Src                 driver.Ptr
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
	Data                driver.Ptr
	NewData             driver.Ptr
	Alignment           int32
	WCenter             float32
	WCardinal           float32
	WDiagonal           float32
	Sh                  driver.LocalPtr
	Padding             int32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// CDNA3 Kernel Arguments

// CDNA3CopyRectKernelArgs defines kernel arguments for CDNA3 architecture (GFX942)
type CDNA3CopyRectKernelArgs struct {
	Dst                 driver.Ptr
	DOffset             int32
	DPitch              int32
	Src                 driver.Ptr
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
	Data                driver.Ptr
	NewData             driver.Ptr
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
	benchmarks.Progress
	initialConfig  checkpointConfig
	configCaptured bool
	readbacks      []benchmarks.Readback
	queue          driver.CommandQueue

	driver  *driver.Runtime
	context driver.Context
	gpus    []int
	queues  []driver.CommandQueue

	copyRectKernel *insts.KernelCodeObject
	stencilKernel  *insts.KernelCodeObject

	Arch                          arch.Type
	wCenter, wCardinal, wDiagonal float32
	hInput, hOutput               []float32
	NumIteration                  int
	haloWidth                     int
	dData1, dData2                driver.Ptr
	currData, newData             driver.Ptr
	NumRows, NumCols              int
	dataSize                      int
	numPaddedCols                 int
	paddedDataSize                int
	pad                           int
	localRows, localCols          int

	useUnifiedMemory bool
}

// NewBenchmark returns a benchmark
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.haloWidth = 1
	b.pad = 16
	b.localRows = 16
	b.localCols = 64
	b.wCenter = 0.5
	b.wCardinal = 0.0
	b.wDiagonal = 0.0
	return b
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
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
	b.driver.SelectGPU(b.context, b.gpus[0])
	b.queue = b.driver.CreateCommandQueue(b.context)

	b.loadKernels()

	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues, b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
}

func (b *Benchmark) initMem() {
	rand.Seed(1)
	b.numPaddedCols = ((b.NumCols-1)/b.pad + 1) * b.pad
	b.paddedDataSize = b.NumRows * b.numPaddedCols

	b.hInput = make([]float32, b.paddedDataSize)
	b.hOutput = make([]float32, b.paddedDataSize)
	for i := 0; i < b.paddedDataSize; i++ {
		// b.hInput[i] = float32(i)
		b.hInput[i] = 1
	}

	if b.useUnifiedMemory {
		b.dData1 = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.paddedDataSize*4))
		b.dData2 = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.paddedDataSize*4))
	} else {
		b.dData1 = b.driver.AllocateMemory(b.context,
			uint64(b.paddedDataSize*4))
		b.dData2 = b.driver.AllocateMemory(b.context,
			uint64(b.paddedDataSize*4))
	}

	b.currData = b.dData1
	b.newData = b.dData2
}

func (b *Benchmark) launchStencilKernel(
	globalSize [3]uint32,
	localSize [3]uint16,
	ldsSize int,
) {
	if b.Arch == arch.CDNA3 {
		args := CDNA3StencilKernelArgs{
			Data:                b.currData,
			NewData:             b.newData,
			Alignment:           16,
			WCenter:             b.wCenter,
			WCardinal:           b.wCardinal,
			WDiagonal:           b.wDiagonal,
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
		b.driver.EnqueueLaunchKernel(b.queue,
			b.stencilKernel,
			globalSize, localSize,
			&args,
		)
	} else {
		args := StencilKernelArgs{
			Data:                b.currData,
			NewData:             b.newData,
			Alignment:           16,
			WCenter:             b.wCenter,
			WCardinal:           b.wCardinal,
			WDiagonal:           b.wDiagonal,
			Sh:                  driver.LocalPtr(ldsSize),
			Padding:             0,
			HiddenGlobalOffsetX: 0,
			HiddenGlobalOffsetY: 0,
			HiddenGlobalOffsetZ: 0,
		}
		b.driver.EnqueueLaunchKernel(b.queue,
			b.stencilKernel,
			globalSize, localSize,
			&args,
		)
	}
}

func (b *Benchmark) exec() {
	b.driver.EnqueueMemCopyH2D(b.queue, b.currData, b.hInput)
	b.driver.EnqueueMemCopyH2D(b.queue, b.newData, b.hInput)

	for i := 0; i < b.NumIteration; i++ {
		ldsSize := (b.localRows + 2) * (b.localCols + 2) * 4

		globalSize := [3]uint32{
			uint32((b.NumRows - 2) / b.localRows),
			uint32(b.NumCols - 2),
			1,
		}
		localSize := [3]uint16{1, uint16(b.localCols), 1}

		b.launchStencilKernel(globalSize, localSize, ldsSize)
		b.currData, b.newData = b.newData, b.currData
	}

	b.readbacks = append(b.readbacks, benchmarks.Readback{Source: b.currData, Target: 0})
}

// Verify verfies
func (b *Benchmark) verify() error {
	cpuOutput := b.cpuStencil2D()

	mismatch := false
	for x := 0; x < b.NumRows; x++ {
		for y := 0; y < b.NumCols; y++ {
			index := x*b.numPaddedCols + y
			if b.hOutput[index] != cpuOutput[index] {
				mismatch = true
				log.Printf("not match at (%d,%d), expected %f to equal %f\n",
					x, y,
					b.hOutput[index], cpuOutput[index])
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
	cpuOutput := make([]float32, b.paddedDataSize)
	for x := 0; x < b.NumRows; x++ {
		for y := 0; y < b.NumCols; y++ {
			cpuOutput[x*b.numPaddedCols+y] =
				b.hInput[x*b.numPaddedCols+y]
		}
	}

	for i := 0; i < b.NumIteration; i++ {
		for x := 0; x < b.NumRows; x++ {
			for y := 0; y < b.NumCols; y++ {
				if x == 0 || y == 0 ||
					x == b.NumRows-1 || y == b.NumCols-1 {
					continue
				}

				center := cpuOutput[x*b.numPaddedCols+y]
				cardinal := cpuOutput[(x-1)*b.numPaddedCols+y] +
					cpuOutput[(x+1)*b.numPaddedCols+y] +
					cpuOutput[x*b.numPaddedCols+(y+1)] +
					cpuOutput[x*b.numPaddedCols+(y-1)]
				diagonal := cpuOutput[(x-1)*b.numPaddedCols+(y+1)] +
					cpuOutput[(x+1)*b.numPaddedCols+(y-1)] +
					cpuOutput[(x+1)*b.numPaddedCols+(y+1)] +
					cpuOutput[(x-1)*b.numPaddedCols+(y-1)]

				out := b.wCenter*center +
					b.wCardinal*cardinal +
					b.wDiagonal*diagonal

				cpuOutput[x*b.numPaddedCols+y] = out
			}
		}
	}

	return cpuOutput
}

func (b *Benchmark) Step() error {
	switch b.Phase {
	case 0:
		b.initialConfig = b.checkpointConfig()
		b.configCaptured = true
		b.setup()
		b.Phase = 1
	case 1:
		b.exec()
		b.Phase = 2
	case 2:
		for i := range b.readbacks {
			var dst any
			switch b.readbacks[i].Target {
			case 0:
				dst = b.hOutput
			}
			b.readbacks[i].Command = b.driver.EnqueueMemCopyD2H(b.queue, b.readbacks[i].Source, binary.Size(dst))
		}
		b.Phase = 3
	case 3:
		for _, copy := range b.readbacks {
			var dst any
			switch copy.Target {
			case 0:
				dst = b.hOutput
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
		b.readbacks = nil
		b.Ended = true
	}
	return nil
}
