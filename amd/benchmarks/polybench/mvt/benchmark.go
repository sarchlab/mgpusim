// Package mvt implements the MVT benchmark from Polybench.
// x1 = x1 + A * y1
// x2 = x2 + A^T * y2
// Two kernels, each with N threads (1D).
package mvt

import (
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// Kernel1Args defines kernel1 arguments
type Kernel1Args struct {
	A                   driver.Ptr
	X1                  driver.Ptr
	Y1                  driver.Ptr
	N                   int32
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Pad0                [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

// Kernel2Args defines kernel2 arguments
type Kernel2Args struct {
	A                   driver.Ptr
	X2                  driver.Ptr
	Y2                  driver.Ptr
	N                   int32
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Pad0                [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

// Benchmark defines a benchmark
type Benchmark struct {
	driver           *driver.Driver
	context          *driver.Context
	gpus             []int
	queues           []*driver.CommandQueue
	kernel1, kernel2 *insts.KernelCodeObject

	N                             int
	a, x1, x2, y1, y2            []float32
	x1Output, x2Output           []float32
	dA, dX1, dX2, dY1, dY2       driver.Ptr
	cpuX1, cpuX2                  []float32

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
		hsacoBytes, "mvt_kernel1")
	if b.kernel1 == nil {
		log.Panic("Failed to load kernel binary")
	}

	b.kernel2 = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "mvt_kernel2")
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
	b.a = make([]float32, b.N*b.N)
	b.x1 = make([]float32, b.N)
	b.x2 = make([]float32, b.N)
	b.y1 = make([]float32, b.N)
	b.y2 = make([]float32, b.N)
	b.x1Output = make([]float32, b.N)
	b.x2Output = make([]float32, b.N)

	for i := 0; i < b.N*b.N; i++ {
		b.a[i] = float32(rand.Intn(100)) / 10.0
	}
	for i := 0; i < b.N; i++ {
		b.x1[i] = float32(rand.Intn(100)) / 10.0
		b.x2[i] = float32(rand.Intn(100)) / 10.0
		b.y1[i] = float32(rand.Intn(100)) / 10.0
		b.y2[i] = float32(rand.Intn(100)) / 10.0
	}

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.N*b.N*4))
		b.dX1 = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.N*4))
		b.dX2 = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.N*4))
		b.dY1 = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.N*4))
		b.dY2 = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.N*4))
	} else {
		b.dA = b.driver.AllocateMemory(b.context,
			uint64(b.N*b.N*4))
		b.dX1 = b.driver.AllocateMemory(b.context,
			uint64(b.N*4))
		b.dX2 = b.driver.AllocateMemory(b.context,
			uint64(b.N*4))
		b.dY1 = b.driver.AllocateMemory(b.context,
			uint64(b.N*4))
		b.dY2 = b.driver.AllocateMemory(b.context,
			uint64(b.N*4))
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dA, b.a)
	b.driver.MemCopyH2D(b.context, b.dX1, b.x1)
	b.driver.MemCopyH2D(b.context, b.dX2, b.x2)
	b.driver.MemCopyH2D(b.context, b.dY1, b.y1)
	b.driver.MemCopyH2D(b.context, b.dY2, b.y2)

	localSize := [3]uint16{256, 1, 1}
	globalSizeX := uint32(((b.N - 1) / 256 + 1) * 256)
	globalSize := [3]uint32{globalSizeX, 1, 1}

	b.launchKernel1(localSize, globalSize, globalSizeX)
	b.launchKernel2(localSize, globalSize, globalSizeX)

	b.driver.MemCopyD2H(b.context, b.x1Output, b.dX1)
	b.driver.MemCopyD2H(b.context, b.x2Output, b.dX2)
}

func (b *Benchmark) launchKernel1(localSize [3]uint16, globalSize [3]uint32, globalSizeX uint32) {
	kernelArg := Kernel1Args{
		A:                   b.dA,
		X1:                  b.dX1,
		Y1:                  b.dY1,
		N:                   int32(b.N),
		HiddenBlockCountX:   globalSizeX / uint32(localSize[0]),
		HiddenBlockCountY:   1,
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(globalSizeX % uint32(localSize[0])),
		HiddenRemainderY:    0,
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      1,
	}
	b.driver.LaunchKernel(b.context, b.kernel1,
		globalSize, localSize, &kernelArg)
}

func (b *Benchmark) launchKernel2(localSize [3]uint16, globalSize [3]uint32, globalSizeX uint32) {
	kernelArg := Kernel2Args{
		A:                   b.dA,
		X2:                  b.dX2,
		Y2:                  b.dY2,
		N:                   int32(b.N),
		HiddenBlockCountX:   globalSizeX / uint32(localSize[0]),
		HiddenBlockCountY:   1,
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(globalSizeX % uint32(localSize[0])),
		HiddenRemainderY:    0,
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      1,
	}
	b.driver.LaunchKernel(b.context, b.kernel2,
		globalSize, localSize, &kernelArg)
}

// Verify verifies
func (b *Benchmark) Verify() {
	b.cpuMvt()

	for i := 0; i < b.N; i++ {
		if b.cpuX1[i] != b.x1Output[i] {
			log.Panicf("Mismatch in x1 at %d, expected %f, but get %f",
				i, b.cpuX1[i], b.x1Output[i])
		}
	}

	for i := 0; i < b.N; i++ {
		if b.cpuX2[i] != b.x2Output[i] {
			log.Panicf("Mismatch in x2 at %d, expected %f, but get %f",
				i, b.cpuX2[i], b.x2Output[i])
		}
	}

	log.Printf("Passed!\n")
}

func (b *Benchmark) cpuMvt() {
	b.cpuX1 = make([]float32, b.N)
	b.cpuX2 = make([]float32, b.N)
	copy(b.cpuX1, b.x1)
	copy(b.cpuX2, b.x2)

	for i := 0; i < b.N; i++ {
		var sum float32
		for j := 0; j < b.N; j++ {
			sum += b.a[i*b.N+j] * b.y1[j]
		}
		b.cpuX1[i] += sum
	}

	for i := 0; i < b.N; i++ {
		var sum float32
		for j := 0; j < b.N; j++ {
			sum += b.a[j*b.N+i] * b.y2[j]
		}
		b.cpuX2[i] += sum
	}
}
