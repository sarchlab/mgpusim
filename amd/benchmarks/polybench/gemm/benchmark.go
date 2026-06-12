// Package gemm implements the GEMM benchmark from Polybench.
// C = alpha * A * B + beta * C
// A is NI×NK, B is NK×NJ, C is NI×NJ
package gemm

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
	C                   driver.Ptr
	Alpha               float32
	Beta                float32
	NI                  int32
	NJ                  int32
	NK                  int32
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

	NI, NJ, NK                int
	Alpha, Beta               float32
	a, b, c, cOutput          []float32
	dA, dB, dC                driver.Ptr
	cpuC                      []float32

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
		hsacoBytes, "gemm_kernel")
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
	b.a = make([]float32, b.NI*b.NK)
	b.b = make([]float32, b.NK*b.NJ)
	b.c = make([]float32, b.NI*b.NJ)
	b.cOutput = make([]float32, b.NI*b.NJ)

	for i := 0; i < b.NI*b.NK; i++ {
		b.a[i] = float32(rand.Intn(100)) / 10.0
	}
	for i := 0; i < b.NK*b.NJ; i++ {
		b.b[i] = float32(rand.Intn(100)) / 10.0
	}
	for i := 0; i < b.NI*b.NJ; i++ {
		b.c[i] = float32(rand.Intn(100)) / 10.0
	}

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NI*b.NK*4))
		b.dB = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NK*b.NJ*4))
		b.dC = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NI*b.NJ*4))
	} else {
		b.dA = b.driver.AllocateMemory(b.context,
			uint64(b.NI*b.NK*4))
		b.dB = b.driver.AllocateMemory(b.context,
			uint64(b.NK*b.NJ*4))
		b.dC = b.driver.AllocateMemory(b.context,
			uint64(b.NI*b.NJ*4))
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dA, b.a)
	b.driver.MemCopyH2D(b.context, b.dB, b.b)
	b.driver.MemCopyH2D(b.context, b.dC, b.c)

	localSize := [3]uint16{16, 16, 1}
	globalSizeX := uint32(((b.NJ-1)/16 + 1) * 16)
	globalSizeY := uint32(((b.NI-1)/16 + 1) * 16)
	globalSize := [3]uint32{globalSizeX, globalSizeY, 1}

	b.launchKernel(localSize, globalSize, globalSizeX, globalSizeY)

	b.driver.MemCopyD2H(b.context, b.cOutput, b.dC)
}

func (b *Benchmark) launchKernel(localSize [3]uint16, globalSize [3]uint32, globalSizeX, globalSizeY uint32) {
	kernelArg := KernelArgs{
		A:                   b.dA,
		B:                   b.dB,
		C:                   b.dC,
		Alpha:               b.Alpha,
		Beta:                b.Beta,
		NI:                  int32(b.NI),
		NJ:                  int32(b.NJ),
		NK:                  int32(b.NK),
		HiddenBlockCountX:   globalSizeX / uint32(localSize[0]),
		HiddenBlockCountY:   globalSizeY / uint32(localSize[1]),
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(globalSizeX % uint32(localSize[0])),
		HiddenRemainderY:    uint16(globalSizeY % uint32(localSize[1])),
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      2,
	}
	b.driver.LaunchKernel(b.context, b.kernel,
		globalSize, localSize, &kernelArg)
}

// Verify verifies
func (b *Benchmark) Verify() {
	b.cpuGemm()

	for i := 0; i < b.NI*b.NJ; i++ {
		diff := b.cpuC[i] - b.cOutput[i]
		if diff < 0 {
			diff = -diff
		}
		threshold := float32(1.0)
		denom := b.cpuC[i]
		if denom < 0 {
			denom = -denom
		}
		if denom > 1 {
			threshold = denom * 1e-6
		}
		if diff > threshold {
			log.Panicf("Mismatch at %d, expected %f, but get %f (diff=%f)",
				i, b.cpuC[i], b.cOutput[i], diff)
		}
	}

	log.Printf("Passed!\n")
}

func (b *Benchmark) cpuGemm() {
	b.cpuC = make([]float32, b.NI*b.NJ)
	copy(b.cpuC, b.c)

	for i := 0; i < b.NI; i++ {
		for j := 0; j < b.NJ; j++ {
			var sum float32
			for k := 0; k < b.NK; k++ {
				sum += b.a[i*b.NK+k] * b.b[k*b.NJ+j]
			}
			b.cpuC[i*b.NJ+j] = b.Alpha*sum + b.Beta*b.cpuC[i*b.NJ+j]
		}
	}
}
