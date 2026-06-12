// Package mm2 implements the 2MM benchmark from Polybench.
// D = alpha*A*B*C + beta*D
// Two kernels: mm2_kernel1 computes tmp=alpha*A*B,
//              mm2_kernel2 computes D=tmp*C+beta*D
package mm2

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
	B                   driver.Ptr
	C                   driver.Ptr
	Alpha               float32
	NI                  int32
	NK                  int32
	NJ                  int32
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
	C                   driver.Ptr
	D                   driver.Ptr
	Dout                driver.Ptr
	Beta                float32
	NI                  int32
	NJ                  int32
	NL                  int32
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

	NI, NJ, NK, NL      int
	Alpha, Beta         float32
	a, b, c, d          []float32
	dOutput             []float32
	dA, dB, dC, dD, dTmp driver.Ptr
	cpuD                []float32

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
	b.kernel1 = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "mm2_kernel1")
	if b.kernel1 == nil {
		log.Panic("Failed to load kernel binary")
	}

	b.kernel2 = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "mm2_kernel2")
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
	b.a = make([]float32, b.NI*b.NK)
	b.b = make([]float32, b.NK*b.NJ)
	b.c = make([]float32, b.NJ*b.NL)
	b.d = make([]float32, b.NI*b.NL)
	b.dOutput = make([]float32, b.NI*b.NL)

	for i := 0; i < b.NI*b.NK; i++ {
		b.a[i] = float32(rand.Intn(100)) / 10.0
	}
	for i := 0; i < b.NK*b.NJ; i++ {
		b.b[i] = float32(rand.Intn(100)) / 10.0
	}
	for i := 0; i < b.NJ*b.NL; i++ {
		b.c[i] = float32(rand.Intn(100)) / 10.0
	}
	for i := 0; i < b.NI*b.NL; i++ {
		b.d[i] = float32(rand.Intn(100)) / 10.0
	}

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NI*b.NK*4))
		b.dB = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NK*b.NJ*4))
		b.dTmp = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NI*b.NJ*4))
		b.dC = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NJ*b.NL*4))
		b.dD = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NI*b.NL*4))
	} else {
		b.dA = b.driver.AllocateMemory(b.context,
			uint64(b.NI*b.NK*4))
		b.dB = b.driver.AllocateMemory(b.context,
			uint64(b.NK*b.NJ*4))
		b.dTmp = b.driver.AllocateMemory(b.context,
			uint64(b.NI*b.NJ*4))
		b.dC = b.driver.AllocateMemory(b.context,
			uint64(b.NJ*b.NL*4))
		b.dD = b.driver.AllocateMemory(b.context,
			uint64(b.NI*b.NL*4))
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dA, b.a)
	b.driver.MemCopyH2D(b.context, b.dB, b.b)
	b.driver.MemCopyH2D(b.context, b.dC, b.c)
	b.driver.MemCopyH2D(b.context, b.dD, b.d)

	localSize := [3]uint16{16, 16, 1}

	// Kernel 1: tmp = alpha * A * B (NI x NJ)
	globalSizeX1 := uint32(((b.NJ - 1) / 16 + 1) * 16)
	globalSizeY1 := uint32(((b.NI - 1) / 16 + 1) * 16)
	globalSize1 := [3]uint32{globalSizeX1, globalSizeY1, 1}
	b.launchKernel1(localSize, globalSize1, globalSizeX1, globalSizeY1)

	// Kernel 2: D = tmp * C + beta * D (NI x NL)
	globalSizeX2 := uint32(((b.NL - 1) / 16 + 1) * 16)
	globalSizeY2 := uint32(((b.NI - 1) / 16 + 1) * 16)
	globalSize2 := [3]uint32{globalSizeX2, globalSizeY2, 1}
	b.launchKernel2(localSize, globalSize2, globalSizeX2, globalSizeY2)

	b.driver.MemCopyD2H(b.context, b.dOutput, b.dD)
}

func (b *Benchmark) launchKernel1(localSize [3]uint16, globalSize [3]uint32, globalSizeX, globalSizeY uint32) {
	kernelArg := Kernel1Args{
		A:                   b.dA,
		B:                   b.dB,
		C:                   b.dTmp,
		Alpha:               b.Alpha,
		NI:                  int32(b.NI),
		NK:                  int32(b.NK),
		NJ:                  int32(b.NJ),
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
	b.driver.LaunchKernel(b.context, b.kernel1,
		globalSize, localSize, &kernelArg)
}

func (b *Benchmark) launchKernel2(localSize [3]uint16, globalSize [3]uint32, globalSizeX, globalSizeY uint32) {
	kernelArg := Kernel2Args{
		C:                   b.dTmp,
		D:                   b.dC,
		Dout:                b.dD,
		Beta:                b.Beta,
		NI:                  int32(b.NI),
		NJ:                  int32(b.NJ),
		NL:                  int32(b.NL),
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
	b.driver.LaunchKernel(b.context, b.kernel2,
		globalSize, localSize, &kernelArg)
}

// Verify verifies
func (b *Benchmark) Verify() {
	b.cpu2mm()

	for i := 0; i < b.NI*b.NL; i++ {
		if b.cpuD[i] != b.dOutput[i] {
			log.Panicf("Mismatch at %d, expected %f, but get %f",
				i, b.cpuD[i], b.dOutput[i])
		}
	}

	log.Printf("Passed!\n")
}

func (b *Benchmark) cpu2mm() {
	tmp := make([]float32, b.NI*b.NJ)
	b.cpuD = make([]float32, b.NI*b.NL)
	copy(b.cpuD, b.d)

	// tmp = alpha * A * B
	for i := 0; i < b.NI; i++ {
		for j := 0; j < b.NJ; j++ {
			var sum float32
			for k := 0; k < b.NK; k++ {
				sum += b.Alpha * b.a[i*b.NK+k] * b.b[k*b.NJ+j]
			}
			tmp[i*b.NJ+j] = sum
		}
	}

	// D = tmp * C + beta * D
	for i := 0; i < b.NI; i++ {
		for j := 0; j < b.NL; j++ {
			var sum float32
			for k := 0; k < b.NJ; k++ {
				sum += tmp[i*b.NJ+k] * b.c[k*b.NL+j]
			}
			b.cpuD[i*b.NL+j] = sum + b.Beta*b.cpuD[i*b.NL+j]
		}
	}
}
