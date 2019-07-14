package stencil2d

import (
	"log"
	"math/rand"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

type CopyRectKernelArgs struct {
	Dst                 driver.GPUPtr
	DOffset             int32
	DPitch              int32
	Src                 driver.GPUPtr
	SOffset             int32
	SPitch              int32
	Width               int32
	Height              int32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

type StencilKernelArgs struct {
	Data                driver.GPUPtr
	NewData             driver.GPUPtr
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

type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	copyRectKernel *insts.HsaCo
	stencilKernel  *insts.HsaCo

	wCenter, wCardinal, wDiagonal float32
	hInput, hOutput               []float32
	NumIteration                  int
	haloWidth                     int
	dData1, dData2                driver.GPUPtr
	currData, newData             *driver.GPUPtr
	NumRows, NumCols              int
	dataSize                      int
	numPaddedCols                 int
	paddedDataSize                int
	pad                           int
	localRows, localCols          int
}

func NewBenchmark(driver *driver.Driver) *Benchmark {
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
	b.loadProgram()
	return b
}

func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

func (b *Benchmark) loadProgram() {
	hsacoBytes := _escFSMustByte(false, "/kernels.hsaco")

	b.copyRectKernel = kernels.LoadProgramFromMemory(
		hsacoBytes, "CopyRect")
	if b.copyRectKernel == nil {
		log.Panic("Failed to load kernel binary")
	}

	b.stencilKernel = kernels.LoadProgramFromMemory(
		hsacoBytes, "StencilKernel")
	if b.stencilKernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

func (b *Benchmark) Run() {
	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues, b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
	b.exec()
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

	b.dData1 = b.driver.AllocateMemoryWithAlignment(b.context,
		uint64(b.paddedDataSize*4), 4096)
	b.dData2 = b.driver.AllocateMemoryWithAlignment(b.context,
		uint64(b.paddedDataSize*4), 4096)

	b.currData = &b.dData1
	b.newData = &b.dData2
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, *b.currData, b.hInput)
	b.driver.MemCopyH2D(b.context, *b.newData, b.hInput)

	for i := 0; i < b.NumIteration; i++ {
		ldsSize := (b.localRows + 2) * (b.localCols + 2) * 4

		args := StencilKernelArgs{
			Data:                *b.currData,
			NewData:             *b.newData,
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

		globalSize := [3]uint32{
			uint32((b.NumRows - 2) / b.localRows),
			uint32(b.NumCols - 2),
			1,
		}
		localSize := [3]uint16{1, uint16(b.localCols), 1}
		b.driver.LaunchKernel(b.context,
			b.stencilKernel,
			globalSize, localSize,
			&args,
		)

		b.currData, b.newData = b.newData, b.currData
	}

	b.driver.MemCopyD2H(b.context, b.hOutput, *b.currData)
}

func (b *Benchmark) Verify() {
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
	for x := 0; x < b.NumRows; x++ {
		for y := 0; y < b.NumCols; y++ {
			index := x*b.numPaddedCols + y
			if b.hOutput[index] != cpuOutput[index] {
				log.Printf("not match at (%d,%d), expected %f to equal %f\n",
					x, y,
					b.hOutput[index], cpuOutput[index])
			}
		}
	}

	log.Printf("Passed!\n")
}
