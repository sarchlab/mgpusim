package matrixtranpose

import (
	"log"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

type MatrixTransposeKernelArgs struct {
	Output              driver.GPUPtr
	Input               driver.GPUPtr
	Block               driver.LocalPtr
	Padding             uint32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

type Benchmark struct {
	kernel *insts.HsaCo

	driver    *driver.Driver
	numGPUs   int
	gpuQueues []*driver.CommandQueue

	Width              int
	elemsPerThread1Dim int
	blockSize          int

	hInputData  []uint32
	hOutputData []uint32
	dInputData  driver.GPUPtr
	dOutputData driver.GPUPtr
}

func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.numGPUs = driver.GetNumGPUs()
	for i := 0; i < b.numGPUs; i++ {
		b.driver.SelectGPU(i)
		b.gpuQueues = append(b.gpuQueues, b.driver.CreateCommandQueue())
	}
	b.loadProgram()
	b.elemsPerThread1Dim = 4
	b.blockSize = 16
	return b
}

func (b *Benchmark) loadProgram() {
	hsacoBytes, err := Asset("kernels.hsaco")
	if err != nil {
		log.Panic(err)
	}

	b.kernel = kernels.LoadProgramFromMemory(hsacoBytes, "matrixTranspose")
	if b.kernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

func (b *Benchmark) Run() {
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	numData := b.Width * b.Width

	b.hInputData = make([]uint32, numData)
	b.hOutputData = make([]uint32, numData)

	for i := 0; i < numData; i++ {
		b.hInputData[i] = uint32(i)
	}

	b.dInputData = b.driver.AllocateMemory(uint64(numData * 4))
	b.dOutputData = b.driver.AllocateMemory(uint64(numData * 4))
	//for i := 0; i < b.numGPUs; i++ {
	//	b.driver.Remap(uint64(b.dInputData)+uint64(i*numData*4/b.numGPUs),
	//		uint64(numData*4/b.numGPUs), i)
	//	b.driver.Remap(uint64(b.dOutputData)+uint64(i*numData*4/b.numGPUs),
	//		uint64(numData*4/b.numGPUs), i)
	//}

	b.driver.MemCopyH2D(b.dInputData, b.hInputData)
}

func (b *Benchmark) exec() {
	for i := 0; i < b.numGPUs; i++ {
		kernArg := MatrixTransposeKernelArgs{
			b.dOutputData,
			b.dInputData,
			driver.LocalPtr(b.blockSize * b.blockSize * b.elemsPerThread1Dim * b.elemsPerThread1Dim * 4),
			0,
			int64(i * b.Width / b.elemsPerThread1Dim / b.numGPUs),
			0,
			0,
		}

		b.driver.EnqueueLaunchKernel(
			b.gpuQueues[i],
			b.kernel,
			[3]uint32{
				uint32(b.Width / b.elemsPerThread1Dim / b.numGPUs),
				uint32(b.Width / b.elemsPerThread1Dim),
				1,
			},
			[3]uint16{uint16(b.blockSize), uint16(b.blockSize), 1},
			&kernArg,
		)
	}
	b.driver.ExecuteAllCommands()
}

func (b *Benchmark) Verify() {
	b.driver.MemCopyD2H(b.hOutputData, b.dOutputData)

	for i := 0; i < b.Width; i++ {
		for j := 0; j < b.Width; j++ {
			if b.hOutputData[j*b.Width+i] != b.hInputData[i*b.Width+j] {
				log.Panicf("mismatch at (%d, %d), expected %d, but get %d",
					i, j,
					b.hInputData[i*b.Width+j],
					b.hOutputData[j*b.Width+i])
			}
		}
	}

	log.Printf("Passed!\n")
}
