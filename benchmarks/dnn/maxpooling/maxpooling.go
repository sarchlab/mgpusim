package maxpooling

import (
	"log"
	"math"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

type KernelArgs struct {
	NumThreads uint32
	Bottom     driver.GPUPtr
	N          uint32
	C          uint32
	H          uint32
	W          uint32
	PooledH    uint32
	PooledW    uint32
	KernelH    uint32
	KernelW    uint32
	StrideH    uint32
	StrideW    uint32
	PadH       uint32
	PadW       uint32

	Top                 driver.GPUPtr
	Mask                driver.GPUPtr
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

type Benchmark struct {
	driver *driver.Driver
	hsaco  *insts.HsaCo

	N            int
	C            int
	H            int
	W            int
	PooledH      int
	PooledW      int
	KernelH      int
	KernelW      int
	StrideH      int
	StrideW      int
	PadH         int
	PadW         int
	LengthInput  int
	LengthOutput int
	inputData    []float32
	Bottom       driver.GPUPtr
	Top          driver.GPUPtr
	Mask         driver.GPUPtr
}

func NewBenchmark(driver *driver.Driver, n *int, c *int, h *int, w *int) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.N = *n
	b.C = *c
	b.H = *h
	b.W = *w

	hsacoBytes, err := Asset("maxpooling.hsaco")
	if err != nil {
		log.Panic(err)
	}
	b.hsaco = kernels.LoadProgramFromMemory(hsacoBytes, "MaxPoolForward")
	b.LengthInput = b.N * b.C * b.H * b.W
	b.KernelH = 2
	b.KernelW = 2
	b.StrideH = 2
	b.StrideW = 2
	b.PadH = 0
	b.PadW = 0
	b.PooledH = int(math.Ceil(float64(b.H+2*b.StrideH-b.KernelH)/float64(b.StrideH))) + 1
	b.PooledW = int(math.Ceil(float64(b.W+2*b.StrideW-b.KernelW)/float64(b.StrideW))) + 1
	b.LengthOutput = b.N * b.C * b.PooledH * b.PooledW
	return b
}

func (b *Benchmark) Run() {
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	b.Bottom = b.driver.AllocateMemory(uint64(b.LengthInput * 4))
	b.Top = b.driver.AllocateMemory(uint64(b.LengthOutput * 4))

	b.inputData = make([]float32, b.LengthInput)
	for i := 0; i < b.LengthInput; i++ {
		b.inputData[i] = float32(i) - 0.5
	}

	b.driver.MemCopyH2D(b.Bottom, b.inputData)
}

func (b *Benchmark) exec() {
	kernArg := KernelArgs{
		uint32(b.LengthInput), b.Top, uint32(b.N), uint32(b.C), uint32(b.H), uint32(b.W),
		uint32(b.PooledH), uint32(b.PooledW), uint32(b.KernelH), uint32(b.KernelW),
		uint32(b.StrideH), uint32(b.StrideW), uint32(b.StrideH), uint32(b.StrideW),
		b.Top, b.Mask,
		0, 0, 0,
	}

	b.driver.LaunchKernel(
		b.hsaco,
		[3]uint32{uint32(b.LengthInput), 1, 1},
		[3]uint16{256, 1, 1},
		&kernArg,
	)
}

func (b *Benchmark) Verify() {
	gpuOutput := make([]float32, b.LengthOutput)
	b.driver.MemCopyD2H(gpuOutput, b.Top)

	for i := 0; i < b.LengthOutput; i++ {
		if b.inputData[i] > 0 && gpuOutput[i] != b.inputData[i] {
			log.Panicf("mismatch at %d, input %f, output %f", i,
				b.inputData[i], gpuOutput[i])
		}

		if b.inputData[i] <= 0 && gpuOutput[i] != 0 {
			log.Panicf("mismatch at %d, input %f, output %f", i,
				b.inputData[i], gpuOutput[i])
		}
	}

	log.Printf("Passed!\n")
}
