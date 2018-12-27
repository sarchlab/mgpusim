package relu

import (
	"log"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

type KernelArgs struct {
	Count               uint32
	Padding             uint32
	Input               driver.GPUPtr
	Output              driver.GPUPtr
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

type Benchmark struct {
	driver *driver.Driver
	hsaco  *insts.HsaCo

	Length      int
	inputData   []float32
	gInputData  driver.GPUPtr
	gOutputData driver.GPUPtr
}

func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver

	hsacoBytes, err := Asset("relu.hsaco")
	if err != nil {
		log.Panic(err)
	}
	b.hsaco = kernels.LoadProgramFromMemory(hsacoBytes, "ReLUForward")

	return b
}

func (b *Benchmark) Run() {
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	b.gInputData = b.driver.AllocateMemory(uint64(b.Length * 4))
	b.gOutputData = b.driver.AllocateMemory(uint64(b.Length * 4))

	b.inputData = make([]float32, b.Length)
	for i := 0; i < b.Length; i++ {
		b.inputData[i] = float32(i) - 0.5
	}

	b.driver.MemCopyH2D(b.gInputData, b.inputData)
}

func (b *Benchmark) exec() {
	kernArg := KernelArgs{
		uint32(b.Length), 0,
		b.gInputData, b.gOutputData,
		0, 0, 0,
	}

	b.driver.LaunchKernel(
		b.hsaco,
		[3]uint32{uint32(b.Length), 1, 1},
		[3]uint16{256, 1, 1},
		&kernArg,
	)
}

func (b *Benchmark) Verify() {
	gpuOutput := make([]float32, b.Length)
	b.driver.MemCopyD2H(gpuOutput, b.gOutputData)

	for i := 0; i < b.Length; i++ {
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
