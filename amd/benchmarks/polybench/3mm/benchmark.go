// Package mm3 implements the 3MM benchmark from Polybench.
// E = A*B, F = C*D, G = E*F
// Three kernels: mm3_kernel1, mm3_kernel2, mm3_kernel3
package mm3

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
	E                   driver.Ptr
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
	F                   driver.Ptr
	NJ                  int32
	NM                  int32
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

// Kernel3Args defines kernel3 arguments
type Kernel3Args struct {
	E                   driver.Ptr
	F                   driver.Ptr
	G                   driver.Ptr
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
	driver                    *driver.Driver
	context                   *driver.Context
	gpus                      []int
	queues                    []*driver.CommandQueue
	kernel1, kernel2, kernel3 *insts.KernelCodeObject

	NI, NJ, NK, NL, NM     int
	a, b, c, d              []float32
	gOutput                 []float32
	dA, dB, dC, dD, dE, dF, dG driver.Ptr
	cpuG                    []float32

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
		hsacoBytes, "mm3_kernel1")
	if b.kernel1 == nil {
		log.Panic("Failed to load kernel binary")
	}

	b.kernel2 = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "mm3_kernel2")
	if b.kernel2 == nil {
		log.Panic("Failed to load kernel binary")
	}

	b.kernel3 = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "mm3_kernel3")
	if b.kernel3 == nil {
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
	b.c = make([]float32, b.NJ*b.NM)
	b.d = make([]float32, b.NM*b.NL)
	b.gOutput = make([]float32, b.NI*b.NL)

	for i := 0; i < b.NI*b.NK; i++ {
		b.a[i] = float32(rand.Intn(100)) / 10.0
	}
	for i := 0; i < b.NK*b.NJ; i++ {
		b.b[i] = float32(rand.Intn(100)) / 10.0
	}
	for i := 0; i < b.NJ*b.NM; i++ {
		b.c[i] = float32(rand.Intn(100)) / 10.0
	}
	for i := 0; i < b.NM*b.NL; i++ {
		b.d[i] = float32(rand.Intn(100)) / 10.0
	}

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NI*b.NK*4))
		b.dB = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NK*b.NJ*4))
		b.dC = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NJ*b.NM*4))
		b.dD = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NM*b.NL*4))
		b.dE = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NI*b.NJ*4))
		b.dF = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NJ*b.NL*4))
		b.dG = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NI*b.NL*4))
	} else {
		b.dA = b.driver.AllocateMemory(b.context,
			uint64(b.NI*b.NK*4))
		b.dB = b.driver.AllocateMemory(b.context,
			uint64(b.NK*b.NJ*4))
		b.dC = b.driver.AllocateMemory(b.context,
			uint64(b.NJ*b.NM*4))
		b.dD = b.driver.AllocateMemory(b.context,
			uint64(b.NM*b.NL*4))
		b.dE = b.driver.AllocateMemory(b.context,
			uint64(b.NI*b.NJ*4))
		b.dF = b.driver.AllocateMemory(b.context,
			uint64(b.NJ*b.NL*4))
		b.dG = b.driver.AllocateMemory(b.context,
			uint64(b.NI*b.NL*4))
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dA, b.a)
	b.driver.MemCopyH2D(b.context, b.dB, b.b)
	b.driver.MemCopyH2D(b.context, b.dC, b.c)
	b.driver.MemCopyH2D(b.context, b.dD, b.d)

	localSize := [3]uint16{16, 16, 1}

	// Kernel 1: E = A * B (NI x NJ)
	gsX1 := uint32(((b.NJ - 1) / 16 + 1) * 16)
	gsY1 := uint32(((b.NI - 1) / 16 + 1) * 16)
	b.launchKernel1(localSize, [3]uint32{gsX1, gsY1, 1}, gsX1, gsY1)

	// Kernel 2: F = C * D (NJ x NL)
	gsX2 := uint32(((b.NL - 1) / 16 + 1) * 16)
	gsY2 := uint32(((b.NJ - 1) / 16 + 1) * 16)
	b.launchKernel2(localSize, [3]uint32{gsX2, gsY2, 1}, gsX2, gsY2)

	// Kernel 3: G = E * F (NI x NL)
	gsX3 := uint32(((b.NL - 1) / 16 + 1) * 16)
	gsY3 := uint32(((b.NI - 1) / 16 + 1) * 16)
	b.launchKernel3(localSize, [3]uint32{gsX3, gsY3, 1}, gsX3, gsY3)

	b.driver.MemCopyD2H(b.context, b.gOutput, b.dG)
}

func (b *Benchmark) launchKernel1(localSize [3]uint16, globalSize [3]uint32, globalSizeX, globalSizeY uint32) {
	kernelArg := Kernel1Args{
		A:                   b.dA,
		B:                   b.dB,
		E:                   b.dE,
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
		C:                   b.dC,
		D:                   b.dD,
		F:                   b.dF,
		NJ:                  int32(b.NJ),
		NM:                  int32(b.NM),
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

func (b *Benchmark) launchKernel3(localSize [3]uint16, globalSize [3]uint32, globalSizeX, globalSizeY uint32) {
	kernelArg := Kernel3Args{
		E:                   b.dE,
		F:                   b.dF,
		G:                   b.dG,
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
	b.driver.LaunchKernel(b.context, b.kernel3,
		globalSize, localSize, &kernelArg)
}

// Verify verifies
func (b *Benchmark) Verify() {
	b.cpu3mm()

	for i := 0; i < b.NI*b.NL; i++ {
		if b.cpuG[i] != b.gOutput[i] {
			log.Panicf("Mismatch at %d, expected %f, but get %f",
				i, b.cpuG[i], b.gOutput[i])
		}
	}

	log.Printf("Passed!\n")
}

func (b *Benchmark) cpu3mm() {
	e := make([]float32, b.NI*b.NJ)
	f := make([]float32, b.NJ*b.NL)
	b.cpuG = make([]float32, b.NI*b.NL)

	// E = A * B
	for i := 0; i < b.NI; i++ {
		for j := 0; j < b.NJ; j++ {
			var sum float32
			for k := 0; k < b.NK; k++ {
				sum += b.a[i*b.NK+k] * b.b[k*b.NJ+j]
			}
			e[i*b.NJ+j] = sum
		}
	}

	// F = C * D
	for i := 0; i < b.NJ; i++ {
		for j := 0; j < b.NL; j++ {
			var sum float32
			for k := 0; k < b.NM; k++ {
				sum += b.c[i*b.NM+k] * b.d[k*b.NL+j]
			}
			f[i*b.NL+j] = sum
		}
	}

	// G = E * F
	for i := 0; i < b.NI; i++ {
		for j := 0; j < b.NL; j++ {
			var sum float32
			for k := 0; k < b.NJ; k++ {
				sum += e[i*b.NJ+k] * f[k*b.NL+j]
			}
			b.cpuG[i*b.NL+j] = sum
		}
	}
}
