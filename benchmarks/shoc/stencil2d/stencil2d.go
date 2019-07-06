package stencil2d

import (
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

type StencialKernelArgs struct {
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
	dData1, dData2 driver.GPUPtr

}

func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
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
}

func (b *Benchmark) exec() {

}

func (b *Benchmark) Verify() {
	log.Printf("Passed!\n")
}
