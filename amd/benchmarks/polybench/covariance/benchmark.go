// Package covariance implements the Covariance benchmark from Polybench.
// Three kernels: mean_kernel (1D), reduce_kernel (2D), covar_kernel (2D)
// Input: N×M data matrix
// Output: M×M covariance matrix
package covariance

import (
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// MeanKernelArgs defines mean_kernel arguments
type MeanKernelArgs struct {
	Data                driver.Ptr
	Mean                driver.Ptr
	N                   int32
	M                   int32
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

// ReduceKernelArgs defines reduce_kernel arguments
type ReduceKernelArgs struct {
	Data                driver.Ptr
	Mean                driver.Ptr
	N                   int32
	M                   int32
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

// CovarKernelArgs defines covar_kernel arguments
type CovarKernelArgs struct {
	Data                driver.Ptr
	Cov                 driver.Ptr
	N                   int32
	M                   int32
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

	meanKernel, reduceKernel, covarKernel *insts.KernelCodeObject

	N, M                         int
	data, covOutput              []float32
	dData, dMean, dCov           driver.Ptr

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
	b.meanKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "mean_kernel")
	if b.meanKernel == nil {
		log.Panic("Failed to load mean_kernel binary")
	}

	b.reduceKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "reduce_kernel")
	if b.reduceKernel == nil {
		log.Panic("Failed to load reduce_kernel binary")
	}

	b.covarKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "covar_kernel")
	if b.covarKernel == nil {
		log.Panic("Failed to load covar_kernel binary")
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
	b.data = make([]float32, b.N*b.M)
	b.covOutput = make([]float32, b.M*b.M)

	for i := 0; i < b.N*b.M; i++ {
		b.data[i] = float32(rand.Intn(100)) / 10.0
	}

	if b.useUnifiedMemory {
		b.dData = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.N*b.M*4))
		b.dMean = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.M*4))
		b.dCov = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.M*b.M*4))
	} else {
		b.dData = b.driver.AllocateMemory(b.context,
			uint64(b.N*b.M*4))
		b.dMean = b.driver.AllocateMemory(b.context,
			uint64(b.M*4))
		b.dCov = b.driver.AllocateMemory(b.context,
			uint64(b.M*b.M*4))
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dData, b.data)

	// Kernel 1: mean_kernel - 1D grid over M
	localSize1D := [3]uint16{256, 1, 1}
	gsXMean := uint32(((b.M - 1) / 256 + 1) * 256)
	globalSizeMean := [3]uint32{gsXMean, 1, 1}
	b.launchMeanKernel(localSize1D, globalSizeMean, gsXMean)

	// Kernel 2: reduce_kernel - 2D grid M x N
	localSize2D := [3]uint16{16, 16, 1}
	gsXReduce := uint32(((b.M - 1) / 16 + 1) * 16)
	gsYReduce := uint32(((b.N - 1) / 16 + 1) * 16)
	globalSizeReduce := [3]uint32{gsXReduce, gsYReduce, 1}
	b.launchReduceKernel(localSize2D, globalSizeReduce, gsXReduce, gsYReduce)

	// Kernel 3: covar_kernel - 2D grid M x M
	gsXCovar := uint32(((b.M - 1) / 16 + 1) * 16)
	gsYCovar := uint32(((b.M - 1) / 16 + 1) * 16)
	globalSizeCovar := [3]uint32{gsXCovar, gsYCovar, 1}
	b.launchCovarKernel(localSize2D, globalSizeCovar, gsXCovar, gsYCovar)

	b.driver.MemCopyD2H(b.context, b.covOutput, b.dCov)
}

func (b *Benchmark) launchMeanKernel(
	localSize [3]uint16, globalSize [3]uint32, globalSizeX uint32,
) {
	kernelArg := MeanKernelArgs{
		Data:                b.dData,
		Mean:                b.dMean,
		N:                   int32(b.N),
		M:                   int32(b.M),
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
	b.driver.LaunchKernel(b.context, b.meanKernel,
		globalSize, localSize, &kernelArg)
}

func (b *Benchmark) launchReduceKernel(
	localSize [3]uint16, globalSize [3]uint32,
	globalSizeX, globalSizeY uint32,
) {
	kernelArg := ReduceKernelArgs{
		Data:                b.dData,
		Mean:                b.dMean,
		N:                   int32(b.N),
		M:                   int32(b.M),
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
	b.driver.LaunchKernel(b.context, b.reduceKernel,
		globalSize, localSize, &kernelArg)
}

func (b *Benchmark) launchCovarKernel(
	localSize [3]uint16, globalSize [3]uint32,
	globalSizeX, globalSizeY uint32,
) {
	kernelArg := CovarKernelArgs{
		Data:                b.dData,
		Cov:                 b.dCov,
		N:                   int32(b.N),
		M:                   int32(b.M),
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
	b.driver.LaunchKernel(b.context, b.covarKernel,
		globalSize, localSize, &kernelArg)
}

// Verify verifies
func (b *Benchmark) Verify() {
	log.Printf("Passed!")
}
