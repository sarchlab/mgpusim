// Package atax implements the ATAX benchmark from Polybench.
package atax

import (
	"log"
	"math"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// Kernel1Args defines kernel arguments
type Kernel1Args struct {
	A   driver.Ptr // offset 0
	X   driver.Ptr // offset 8
	Tmp driver.Ptr // offset 16
	NX  int32      // offset 24
	NY  int32      // offset 28
	// Implicit args expected by the runtime
	Pad1             [12]byte   // offset 32-43 (padding)
	HiddenGroupSizeX uint16     // offset 44 (0x2c)
	HiddenGroupSizeY uint16     // offset 46
	HiddenGroupSizeZ uint16     // offset 48
	Pad2             [238]byte  // offset 50-287 (rest of implicit args)
}

// Kernel2Args defines kernel arguments
type Kernel2Args struct {
	A   driver.Ptr // offset 0
	Y   driver.Ptr // offset 8
	Tmp driver.Ptr // offset 16
	NX  int32      // offset 24
	NY  int32      // offset 28
	// Implicit args expected by the runtime
	Pad1             [12]byte   // offset 32-43 (padding)
	HiddenGroupSizeX uint16     // offset 44 (0x2c)
	HiddenGroupSizeY uint16     // offset 46
	HiddenGroupSizeZ uint16     // offset 48
	Pad2             [238]byte  // offset 50-287 (rest of implicit args)
}

// Benchmark defines a benchmark
type Benchmark struct {
	driver           *driver.Driver
	context          *driver.Context
	gpus             []int
	queues           []*driver.CommandQueue
	kernel1, kernel2 *insts.KernelCodeObject

	NX, NY                int
	a, x, y, yOutput, tmp []float32
	dA, dX, dY, dTmp      driver.Ptr
	cpuY                  []float32

	useUnifiedMemory bool
}

// NewBenchmark makes a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	return b
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

func (b *Benchmark) loadProgram() {
	b.kernel1 = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "atax_kernel1")
	if b.kernel1 == nil {
		log.Panic("Failed to load kernel binary")
	}

	b.kernel2 = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "atax_kernel2")
	if b.kernel2 == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// Run runs
func (b *Benchmark) Run() {
	b.loadProgram()

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
	b.x = make([]float32, b.NX)
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

func (b *Benchmark) launchKernel1(localSize [3]uint16, globalSize [3]uint32) {
	kernel1Arg := Kernel1Args{
		A:                b.dA,
		X:                b.dX,
		Tmp:              b.dTmp,
		NX:               int32(b.NX),
		NY:               int32(b.NY),
		HiddenGroupSizeX: localSize[0],
		HiddenGroupSizeY: localSize[1],
		HiddenGroupSizeZ: localSize[2],
	}
	b.driver.LaunchKernel(b.context, b.kernel1,
		globalSize, localSize, &kernel1Arg)
}

func (b *Benchmark) launchKernel2(localSize [3]uint16, globalSize [3]uint32) {
	kernel2Arg := Kernel2Args{
		A:                b.dA,
		Y:                b.dY,
		Tmp:              b.dTmp,
		NX:               int32(b.NX),
		NY:               int32(b.NY),
		HiddenGroupSizeX: localSize[0],
		HiddenGroupSizeY: localSize[1],
		HiddenGroupSizeZ: localSize[2],
	}
	b.driver.LaunchKernel(b.context, b.kernel2,
		globalSize, localSize, &kernel2Arg)
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dA, b.a)
	b.driver.MemCopyH2D(b.context, b.dX, b.x)

	localSize := [3]uint16{256, 1, 1}
	globalSizeX := uint32(((b.NX-1)/256 + 1) * 256)
	globalSize := [3]uint32{globalSizeX, 1, 1}

	b.launchKernel1(localSize, globalSize)

	globalSizeX = uint32(((b.NY-1)/256 + 1) * 256)
	globalSize = [3]uint32{globalSizeX, 1, 1}

	b.launchKernel2(localSize, globalSize)

	b.driver.MemCopyD2H(b.context, b.yOutput, b.dY)
}

// Verify verifies
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
