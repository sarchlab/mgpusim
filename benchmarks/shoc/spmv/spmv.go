// Package spmv include the benchmark of sparse matrix-vector matiplication.
package spmv

import (
	"log"

	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/kernels"
)

type MatrixTransposeKernelArgs struct {
	Output              driver.GPUPtr
	Input               driver.GPUPtr
	Block               driver.LocalPtr
	WIWidth             uint32
	WIHeight            uint32
	NumWGWidth          uint32
	GroupXOffset        uint32
	GroupYOffset        uint32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

type Benchmark struct {
	driver            *driver.Driver
	context           *driver.Context
	gpus              []int
	queues            []*driver.CommandQueue
	userUnifiedMemory bool

	kernel *insts.HsaCo
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

// Use Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) loadProgram() {
	hsacoBytes := _escFSMustByte(false, "/spmv.hsaco")

	b.kernel = kernels.LoadProgramFromMemory(hsacoBytes, "matrixTranspose")
	if b.kernel == nil {
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
}

func spmv(
	val []float32, cols, rowDelimiter []int,
	vec []float64,
	dim int32,
	out []float64,
) {

}
