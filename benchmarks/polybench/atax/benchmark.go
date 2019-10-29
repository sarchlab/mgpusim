package atax

import (
	"log"
	"math"
	"math/rand"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

type Kernel1Args struct {
	A   driver.GPUPtr
	X   driver.GPUPtr
	Tmp driver.GPUPtr
	NX  int32
	NY  int32
}

type Kernel2Args struct {
	A      driver.GPUPtr
	Y      driver.GPUPtr
	Tmp    driver.GPUPtr
	NX, NY int32
}

type Benchmark struct {
	driver           *driver.Driver
	context          *driver.Context
	gpus             []int
	queues           []*driver.CommandQueue
	kernel1, kernel2 *insts.HsaCo

	NX, NY                int
	a, x, y, yOutput, tmp []float32
	dA, dX, dY, dTmp      driver.GPUPtr
	cpuY                  []float32

	useUnifiedMemory bool
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
	hsacoBytes := _escFSMustByte(false, "/kernels.hsaco")

	b.kernel1 = kernels.LoadProgramFromMemory(
		hsacoBytes, "atax_kernel1")
	if b.kernel1 == nil {
		log.Panic("Failed to load kernel binary")
	}

	b.kernel2 = kernels.LoadProgramFromMemory(
		hsacoBytes, "atax_kernel2")
	if b.kernel2 == nil {
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
	b.a = make([]float32, b.NX*b.NY)
	b.x = make([]float32, b.NY)
	b.y = make([]float32, b.NY)
	b.yOutput = make([]float32, b.NY)
	b.tmp = make([]float32, b.NX)

	for i := 0; i < b.NX; i++ {
		b.x[i] = float32(i) * math.Pi
		for j := 0; j < b.NY; j++ {
			b.a[i*b.NY+j] = float32(i) * float32(j) / float32(b.NX)
		}
	}

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NY*b.NX*4))
		b.dX = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NY*4))
		b.dY = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NY*4))
		b.dTmp = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NX*4))
	} else {
		b.dA = b.driver.AllocateMemory(b.context,
			uint64(b.NY*b.NX*4))
		b.dX = b.driver.AllocateMemory(b.context,
			uint64(b.NY*4))
		b.dY = b.driver.AllocateMemory(b.context,
			uint64(b.NY*4))
		b.dTmp = b.driver.AllocateMemory(b.context,
			uint64(b.NX*4))
	}

}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dA, b.a)
	b.driver.MemCopyH2D(b.context, b.dX, b.x)

	localSize := [3]uint16{256, 1, 1}
	globalSizeX := uint32(((b.NX-1)/256 + 1) * 256)
	globalSize := [3]uint32{globalSizeX, 1, 1}

	kernel1Arg := Kernel1Args{
		A:   b.dA,
		X:   b.dX,
		Tmp: b.dTmp,
		NX:  int32(b.NX),
		NY:  int32(b.NY),
	}
	b.driver.LaunchKernel(b.context, b.kernel1,
		globalSize, localSize, &kernel1Arg)

	globalSizeX = uint32(((b.NY-1)/256 + 1) * 256)
	globalSize = [3]uint32{globalSizeX, 1, 1}

	kernel2Arg := Kernel2Args{
		A:   b.dA,
		Y:   b.dY,
		Tmp: b.dTmp,
		NX:  int32(b.NX),
		NY:  int32(b.NY),
	}
	b.driver.LaunchKernel(b.context, b.kernel2,
		globalSize, localSize, &kernel2Arg)

	b.driver.MemCopyD2H(b.context, b.yOutput, b.dY)
}

func (b *Benchmark) Verify() {
	b.cpuAtax()

	for i := 0; i < b.NY; i++ {
		if b.cpuY[i] != b.yOutput[i] {
			log.Panicf("Mismatch at %d, expected %f, but get %f",
				i, b.cpuY[i], b.yOutput[i])
		}
	}

	log.Printf("Passed!\n")
}

func (b *Benchmark) cpuAtax() {
	b.cpuY = make([]float32, b.NY)
	tmp := make([]float32, b.NX)

	for i := 0; i < b.NY; i++ {
		b.cpuY[i] = 0
	}

	for i := 0; i < b.NX; i++ {
		tmp[i] = 0

		for j := 0; j < b.NY; j++ {
			tmp[i] += b.a[i*b.NY+j] * b.x[j]
		}

		for j := 0; j < b.NY; j++ {
			b.cpuY[j] += b.a[i*b.NY+j] * tmp[i]
		}
	}
}
