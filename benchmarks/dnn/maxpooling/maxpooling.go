package maxpooling

import (
	"log"
	"math"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

type KernelArgs struct {
	NumThreads uint64
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
	Top        driver.GPUPtr

	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	hsaco   *insts.HsaCo

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
	outputData   []float32
}

func NewBenchmark(
	driver *driver.Driver,
	n int, c int, h int, w int,
) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = driver.Init()

	b.N = n
	b.C = c
	b.H = h
	b.W = w

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
	b.PooledH = int(math.Ceil(float64(b.H+2*b.PadH-b.KernelH)/float64(b.StrideH))) + 1
	b.PooledW = int(math.Ceil(float64(b.W+2*b.PadW-b.KernelW)/float64(b.StrideW))) + 1
	b.LengthOutput = b.N * b.C * b.PooledH * b.PooledW
	return b
}

func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

func (b *Benchmark) Run() {
	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	b.Bottom = b.driver.AllocateMemoryWithAlignment(
		b.context, uint64(b.LengthInput*4), 4096)
	b.driver.Distribute(b.context, b.Bottom, uint64(b.LengthInput*4), b.gpus)

	b.Top = b.driver.AllocateMemoryWithAlignment(
		b.context, uint64(b.LengthOutput*4), 4096)
	b.driver.Distribute(b.context, b.Top, uint64(b.LengthOutput*4), b.gpus)

	b.inputData = make([]float32, b.LengthInput)
	b.outputData = make([]float32, b.LengthOutput)
	for i := 0; i < b.LengthInput; i++ {
		b.inputData[i] = float32(i) - 0.5
	}

	b.driver.MemCopyH2D(b.context, b.Bottom, b.inputData)
}

func (b *Benchmark) exec() {
	queues := make([]*driver.CommandQueue, len(b.gpus))

	for i, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		q := b.driver.CreateCommandQueue(b.context)
		queues[i] = q

		numWI := b.LengthOutput / len(b.gpus)

		kernArg := KernelArgs{
			uint64(b.LengthOutput), b.Bottom,
			uint32(b.N), uint32(b.C), uint32(b.H), uint32(b.W),
			uint32(b.PooledH), uint32(b.PooledW),
			uint32(b.KernelH), uint32(b.KernelW),
			uint32(b.StrideH), uint32(b.StrideW),
			uint32(b.PadH), uint32(b.PadW),
			b.Top,
			int64(numWI * i), 0, 0,
		}

		b.driver.EnqueueLaunchKernel(
			q,
			b.hsaco,
			[3]uint32{uint32(numWI), 1, 1},
			[3]uint16{uint16(b.C), uint16(b.N), 1},
			&kernArg,
		)
	}

	for _, q := range queues {
		b.driver.DrainCommandQueue(q)
	}

	b.driver.MemCopyD2H(b.context, b.outputData, b.Top)
}

func (b *Benchmark) Verify() {
	cpuOutput := b.CPUMaxPooling()

	for i := 0; i < b.LengthOutput; i++ {
		if b.outputData[i] != cpuOutput[i] {
			log.Panicf("mismatch at %d, expected %f, but get %f",
				i, cpuOutput[i], b.outputData[i])
		}
	}

	// for i := 0; i < b.LengthInput; i++ {
	// 	fmt.Printf("Input: %f\n", b.inputData[i])
	// }

	// for i := 0; i < b.LengthOutput; i++ {
	// 	fmt.Printf("Output: %f\n", gpuOutput[i])
	// }

	log.Printf("Passed!\n")
}

func (b *Benchmark) CPUMaxPooling() []float32 {
	cpuOutput := make([]float32, b.LengthOutput)

	for i := 0; i < b.LengthOutput; i++ {
		pw := i % b.PooledW
		ph := (i / b.PooledW) % b.PooledH
		c := (i / b.PooledW / b.PooledH) % b.C
		n := i / b.PooledW / b.PooledH / b.C

		hStart := ph*b.StrideH - b.PadH
		wStart := pw*b.StrideW - b.PadW
		hEnd := hStart + b.KernelH
		if hEnd > b.H {
			hEnd = b.H
		}
		wEnd := wStart + b.KernelW
		if wEnd > b.W {
			wEnd = b.W
		}
		if hStart < 0 {
			hStart = 0
		}
		if wStart < 0 {
			wStart = 0
		}

		maxVal := float32(-math.MaxFloat32)
		maxIdx := -1

		offset := (n*b.C + c) * b.H * b.W
		for h := hStart; h < hEnd; h++ {
			for w := wStart; w < wEnd; w++ {
				maxIdx = h*b.W + w
				maxVal = b.inputData[maxIdx+offset]
			}
		}

		cpuOutput[i] = maxVal
	}

	return cpuOutput
}
