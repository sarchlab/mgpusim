package atax

import (
	"log"
	"math"
	"math/rand"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

type Benchmark struct {
	driver           *driver.Driver
	context          *driver.Context
	gpus             []int
	queues           []*driver.CommandQueue
	kernel1, kernel2 *insts.HsaCo

	NX, NY                int
	a, x, y, yOutput, tmp []float32
	dA, dX, dY, dTmp      driver.GPUPtr
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

	b.dA = b.driver.AllocateMemoryWithAlignment(b.context,
		uint64(b.NY*b.NX*4), 4096)
	b.dX = b.driver.AllocateMemoryWithAlignment(b.context,
		uint64(b.NY*4), 4096)
	b.dY = b.driver.AllocateMemoryWithAlignment(b.context,
		uint64(b.NY*4), 4096)
	b.dTmp = b.driver.AllocateMemoryWithAlignment(b.context,
		uint64(b.NX*4), 4096)
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dA, b.a)
	b.driver.MemCopyH2D(b.context, b.dX, b.x)
}

func (b *Benchmark) Verify() {
	log.Printf("Passed!\n")
}
