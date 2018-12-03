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

	numGPUs   int
	gpuQueues []*driver.CommandQueue

	Length      int
	inputData   []float32
	gInputData  driver.GPUPtr
	gOutputData driver.GPUPtr
}

func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.numGPUs = driver.GetNumGPUs()
	for i := 0; i < b.numGPUs; i++ {
		b.driver.SelectGPU(i)
		b.gpuQueues = append(b.gpuQueues, b.driver.CreateCommandQueue())
	}

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

	for i := uint64(0); i < uint64(b.numGPUs); i++ {
		inputBytePerGPU := uint64(b.Length * 4 / b.numGPUs)
		inputPtr := uint64(b.gInputData) + i*inputBytePerGPU
		b.driver.Remap(inputPtr, inputBytePerGPU, int(i))

		b.driver.EnqueueMemCopyH2D(b.gpuQueues[i],
			driver.GPUPtr(inputPtr),
			b.inputData[b.Length/b.numGPUs*int(i):b.Length/b.numGPUs*(int(i)+1)])

		outputPtr := uint64(b.gOutputData) + i*inputBytePerGPU
		b.driver.Remap(outputPtr, inputBytePerGPU, int(i))
	}
}

func (b *Benchmark) exec() {
	for i := 0; i < b.numGPUs; i++ {
		kernArg := KernelArgs{
			uint32(b.Length), 0,
			b.gInputData, b.gOutputData,
			int64(b.Length / b.numGPUs * i), 0, 0,
		}

		b.driver.EnqueueLaunchKernel(
			b.gpuQueues[i],
			b.hsaco,
			[3]uint32{uint32(b.Length / b.numGPUs), 1, 1},
			[3]uint16{256, 1, 1},
			&kernArg,
		)
	}

	b.driver.ExecuteAllCommands()
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
