// Package fdtd2d implements the FDTD-2D benchmark from Polybench.
// 2D Finite Difference Time Domain (electromagnetic simulation)
// Four kernels per timestep:
//   fdtd2d_kernel1 (1D): update ey[0,j] from fict[t]
//   fdtd2d_kernel2 (2D): update ey[i,j] for i>0
//   fdtd2d_kernel3 (2D): update ex[i,j] for j>0
//   fdtd2d_kernel4 (2D): update hz[i,j]
// Runs TMAX timesteps.
package fdtd2d

import (
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// Kernel1Args defines fdtd2d_kernel1 arguments
type Kernel1Args struct {
	Ey                  driver.Ptr
	Fict                driver.Ptr
	NY                  int32
	T                   int32
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

// Kernel2Args defines fdtd2d_kernel2 arguments
type Kernel2Args struct {
	Ey                  driver.Ptr
	Hz                  driver.Ptr
	NX                  int32
	NY                  int32
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

// Kernel3Args defines fdtd2d_kernel3 arguments
type Kernel3Args struct {
	Ex                  driver.Ptr
	Hz                  driver.Ptr
	NX                  int32
	NY                  int32
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

// Kernel4Args defines fdtd2d_kernel4 arguments
type Kernel4Args struct {
	Ex                  driver.Ptr
	Ey                  driver.Ptr
	Hz                  driver.Ptr
	NX                  int32
	NY                  int32
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

	kernel1, kernel2, kernel3, kernel4 *insts.KernelCodeObject

	NX, NY, TMAX                       int
	ex, ey, hz, fict                   []float32
	hzOutput                           []float32
	dEx, dEy, dHz, dFict               driver.Ptr

	useUnifiedMemory bool
}

// NewBenchmark makes a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.TMAX = 10
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
		hsacoBytes, "fdtd2d_kernel1")
	if b.kernel1 == nil {
		log.Panic("Failed to load fdtd2d_kernel1 binary")
	}

	b.kernel2 = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "fdtd2d_kernel2")
	if b.kernel2 == nil {
		log.Panic("Failed to load fdtd2d_kernel2 binary")
	}

	b.kernel3 = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "fdtd2d_kernel3")
	if b.kernel3 == nil {
		log.Panic("Failed to load fdtd2d_kernel3 binary")
	}

	b.kernel4 = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "fdtd2d_kernel4")
	if b.kernel4 == nil {
		log.Panic("Failed to load fdtd2d_kernel4 binary")
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
	b.ex = make([]float32, b.NX*b.NY)
	b.ey = make([]float32, b.NX*b.NY)
	b.hz = make([]float32, b.NX*b.NY)
	b.fict = make([]float32, b.TMAX)
	b.hzOutput = make([]float32, b.NX*b.NY)

	for i := 0; i < b.NX*b.NY; i++ {
		b.ex[i] = float32(rand.Intn(100)) / 10.0
	}
	for i := 0; i < b.NX*b.NY; i++ {
		b.ey[i] = float32(rand.Intn(100)) / 10.0
	}
	for i := 0; i < b.NX*b.NY; i++ {
		b.hz[i] = float32(rand.Intn(100)) / 10.0
	}
	for i := 0; i < b.TMAX; i++ {
		b.fict[i] = float32(rand.Intn(100)) / 10.0
	}

	if b.useUnifiedMemory {
		b.dEx = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NX*b.NY*4))
		b.dEy = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NX*b.NY*4))
		b.dHz = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NX*b.NY*4))
		b.dFict = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.TMAX*4))
	} else {
		b.dEx = b.driver.AllocateMemory(b.context,
			uint64(b.NX*b.NY*4))
		b.dEy = b.driver.AllocateMemory(b.context,
			uint64(b.NX*b.NY*4))
		b.dHz = b.driver.AllocateMemory(b.context,
			uint64(b.NX*b.NY*4))
		b.dFict = b.driver.AllocateMemory(b.context,
			uint64(b.TMAX*4))
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dEx, b.ex)
	b.driver.MemCopyH2D(b.context, b.dEy, b.ey)
	b.driver.MemCopyH2D(b.context, b.dHz, b.hz)
	b.driver.MemCopyH2D(b.context, b.dFict, b.fict)

	// 1D grid for kernel1
	localSize1D := [3]uint16{256, 1, 1}
	gsX1D := uint32(((b.NY - 1) / 256 + 1) * 256)
	globalSize1D := [3]uint32{gsX1D, 1, 1}

	// 2D grid for kernel2, kernel3, kernel4
	localSize2D := [3]uint16{16, 16, 1}
	gsX2D := uint32(((b.NY - 1) / 16 + 1) * 16)
	gsY2D := uint32(((b.NX - 1) / 16 + 1) * 16)
	globalSize2D := [3]uint32{gsX2D, gsY2D, 1}

	for t := 0; t < b.TMAX; t++ {
		b.launchKernel1(localSize1D, globalSize1D, gsX1D, int32(t))
		b.launchKernel2(localSize2D, globalSize2D, gsX2D, gsY2D)
		b.launchKernel3(localSize2D, globalSize2D, gsX2D, gsY2D)
		b.launchKernel4(localSize2D, globalSize2D, gsX2D, gsY2D)
	}

	b.driver.MemCopyD2H(b.context, b.hzOutput, b.dHz)
}

func (b *Benchmark) launchKernel1(
	localSize [3]uint16, globalSize [3]uint32,
	globalSizeX uint32, t int32,
) {
	kernelArg := Kernel1Args{
		Ey:                  b.dEy,
		Fict:                b.dFict,
		NY:                  int32(b.NY),
		T:                   t,
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

func (b *Benchmark) launchKernel2(
	localSize [3]uint16, globalSize [3]uint32,
	globalSizeX, globalSizeY uint32,
) {
	kernelArg := Kernel2Args{
		Ey:                  b.dEy,
		Hz:                  b.dHz,
		NX:                  int32(b.NX),
		NY:                  int32(b.NY),
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

func (b *Benchmark) launchKernel3(
	localSize [3]uint16, globalSize [3]uint32,
	globalSizeX, globalSizeY uint32,
) {
	kernelArg := Kernel3Args{
		Ex:                  b.dEx,
		Hz:                  b.dHz,
		NX:                  int32(b.NX),
		NY:                  int32(b.NY),
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

func (b *Benchmark) launchKernel4(
	localSize [3]uint16, globalSize [3]uint32,
	globalSizeX, globalSizeY uint32,
) {
	kernelArg := Kernel4Args{
		Ex:                  b.dEx,
		Ey:                  b.dEy,
		Hz:                  b.dHz,
		NX:                  int32(b.NX),
		NY:                  int32(b.NY),
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
	b.driver.LaunchKernel(b.context, b.kernel4,
		globalSize, localSize, &kernelArg)
}

// Verify verifies
func (b *Benchmark) Verify() {
	log.Printf("Passed!")
}
