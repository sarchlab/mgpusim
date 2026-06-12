// Package gesummv implements the GESUMMV benchmark from Polybench.
// y = alpha * A * x + beta * B * x
// A,B are N×N, x is N, y is N
// Single kernel.
package gesummv

import (
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments
type KernelArgs struct {
	A                   driver.Ptr
	B                   driver.Ptr
	X                   driver.Ptr
	Y                   driver.Ptr
	Alpha               float32
	Beta                float32
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
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue
	kernel  *insts.KernelCodeObject

	N              int
	Alpha, Beta    float32
	a, b, x, y    []float32
	yOutput        []float32
	dA, dB, dX, dY driver.Ptr
	cpuY           []float32

	useUnifiedMemory bool
}

// NewBenchmark makes a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.Alpha = 32412.0
	b.Beta = 2123.0
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
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "gesummv_kernel")
	if b.kernel == nil {
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
	b.b = make([]float32, b.N*b.N)
	b.x = make([]float32, b.N)
	b.y = make([]float32, b.N)
	b.yOutput = make([]float32, b.N)

	for i := 0; i < b.N*b.N; i++ {
		b.a[i] = float32(rand.Intn(100)) / 10.0
	}
	for i := 0; i < b.N*b.N; i++ {
		b.b[i] = float32(rand.Intn(100)) / 10.0
	}
	for i := 0; i < b.N; i++ {
		b.x[i] = float32(rand.Intn(100)) / 10.0
	}

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.N*b.N*4))
		b.dB = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.N*b.N*4))
		b.dX = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.N*4))
		b.dY = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.N*4))
	} else {
		b.dA = b.driver.AllocateMemory(b.context,
			uint64(b.N*b.N*4))
		b.dB = b.driver.AllocateMemory(b.context,
			uint64(b.N*b.N*4))
		b.dX = b.driver.AllocateMemory(b.context,
			uint64(b.N*4))
		b.dY = b.driver.AllocateMemory(b.context,
			uint64(b.N*4))
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dA, b.a)
	b.driver.MemCopyH2D(b.context, b.dB, b.b)
	b.driver.MemCopyH2D(b.context, b.dX, b.x)

	localSize := [3]uint16{256, 1, 1}
	globalSizeX := uint32(((b.N - 1) / 256 + 1) * 256)
	globalSize := [3]uint32{globalSizeX, 1, 1}

	b.launchKernel(localSize, globalSize, globalSizeX)

	b.driver.MemCopyD2H(b.context, b.yOutput, b.dY)
}

func (b *Benchmark) launchKernel(localSize [3]uint16, globalSize [3]uint32, globalSizeX uint32) {
	kernelArg := KernelArgs{
		A:                   b.dA,
		B:                   b.dB,
		X:                   b.dX,
		Y:                   b.dY,
		Alpha:               b.Alpha,
		Beta:                b.Beta,
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
	b.driver.LaunchKernel(b.context, b.kernel,
		globalSize, localSize, &kernelArg)
}

// Verify verifies
func (b *Benchmark) Verify() {
	b.cpuGesummv()

	for i := 0; i < b.N; i++ {
		expected := b.cpuY[i]
		got := b.yOutput[i]
		if expected != got {
			diff := expected - got
			if diff < 0 {
				diff = -diff
			}
			rel := diff / expected
			if rel < 0 {
				rel = -rel
			}
			if rel > 1e-5 && diff > 1.0 {
				log.Panicf("Mismatch at %d, expected %f, but get %f",
					i, expected, got)
			}
		}
	}

	log.Printf("Passed!\n")
}

func (b *Benchmark) cpuGesummv() {
	b.cpuY = make([]float32, b.N)

	for i := 0; i < b.N; i++ {
		var tmp1, tmp2 float32
		for j := 0; j < b.N; j++ {
			tmp1 += b.a[i*b.N+j] * b.x[j]
			tmp2 += b.b[i*b.N+j] * b.x[j]
		}
		b.cpuY[i] = b.Alpha*tmp1 + b.Beta*tmp2
	}
}
