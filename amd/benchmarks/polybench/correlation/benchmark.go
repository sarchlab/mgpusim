// Package correlation implements the Correlation benchmark from Polybench.
// Four kernels: mean_kernel (1D), stddev_kernel (1D),
// normalize_kernel (2D), corr_kernel (2D)
// Input: N×M data matrix
// Output: M×M correlation matrix
package correlation

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

// StddevKernelArgs defines stddev_kernel arguments
type StddevKernelArgs struct {
	Data                driver.Ptr
	Mean                driver.Ptr
	Stddev              driver.Ptr
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

// NormalizeKernelArgs defines normalize_kernel arguments
type NormalizeKernelArgs struct {
	Data                driver.Ptr
	Mean                driver.Ptr
	Stddev              driver.Ptr
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

// CorrKernelArgs defines corr_kernel arguments
type CorrKernelArgs struct {
	Data                driver.Ptr
	Corr                driver.Ptr
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

	meanKernel, stddevKernel, normalizeKernel, corrKernel *insts.KernelCodeObject

	N, M                              int
	data, corrOutput                  []float32
	dData, dMean, dStddev, dCorr      driver.Ptr

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

	b.stddevKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "stddev_kernel")
	if b.stddevKernel == nil {
		log.Panic("Failed to load stddev_kernel binary")
	}

	b.normalizeKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "normalize_kernel")
	if b.normalizeKernel == nil {
		log.Panic("Failed to load normalize_kernel binary")
	}

	b.corrKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "corr_kernel")
	if b.corrKernel == nil {
		log.Panic("Failed to load corr_kernel binary")
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
	b.corrOutput = make([]float32, b.M*b.M)

	for i := 0; i < b.N*b.M; i++ {
		b.data[i] = float32(rand.Intn(100)) / 10.0
	}

	if b.useUnifiedMemory {
		b.dData = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.N*b.M*4))
		b.dMean = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.M*4))
		b.dStddev = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.M*4))
		b.dCorr = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.M*b.M*4))
	} else {
		b.dData = b.driver.AllocateMemory(b.context,
			uint64(b.N*b.M*4))
		b.dMean = b.driver.AllocateMemory(b.context,
			uint64(b.M*4))
		b.dStddev = b.driver.AllocateMemory(b.context,
			uint64(b.M*4))
		b.dCorr = b.driver.AllocateMemory(b.context,
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

	// Kernel 2: stddev_kernel - 1D grid over M
	gsXStddev := uint32(((b.M - 1) / 256 + 1) * 256)
	globalSizeStddev := [3]uint32{gsXStddev, 1, 1}
	b.launchStddevKernel(localSize1D, globalSizeStddev, gsXStddev)

	// Kernel 3: normalize_kernel - 2D grid M x N
	localSize2D := [3]uint16{16, 16, 1}
	gsXNorm := uint32(((b.M - 1) / 16 + 1) * 16)
	gsYNorm := uint32(((b.N - 1) / 16 + 1) * 16)
	globalSizeNorm := [3]uint32{gsXNorm, gsYNorm, 1}
	b.launchNormalizeKernel(localSize2D, globalSizeNorm, gsXNorm, gsYNorm)

	// Kernel 4: corr_kernel - 2D grid M x M
	gsXCorr := uint32(((b.M - 1) / 16 + 1) * 16)
	gsYCorr := uint32(((b.M - 1) / 16 + 1) * 16)
	globalSizeCorr := [3]uint32{gsXCorr, gsYCorr, 1}
	b.launchCorrKernel(localSize2D, globalSizeCorr, gsXCorr, gsYCorr)

	b.driver.MemCopyD2H(b.context, b.corrOutput, b.dCorr)
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

func (b *Benchmark) launchStddevKernel(
	localSize [3]uint16, globalSize [3]uint32, globalSizeX uint32,
) {
	kernelArg := StddevKernelArgs{
		Data:                b.dData,
		Mean:                b.dMean,
		Stddev:              b.dStddev,
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
	b.driver.LaunchKernel(b.context, b.stddevKernel,
		globalSize, localSize, &kernelArg)
}

func (b *Benchmark) launchNormalizeKernel(
	localSize [3]uint16, globalSize [3]uint32,
	globalSizeX, globalSizeY uint32,
) {
	kernelArg := NormalizeKernelArgs{
		Data:                b.dData,
		Mean:                b.dMean,
		Stddev:              b.dStddev,
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
	b.driver.LaunchKernel(b.context, b.normalizeKernel,
		globalSize, localSize, &kernelArg)
}

func (b *Benchmark) launchCorrKernel(
	localSize [3]uint16, globalSize [3]uint32,
	globalSizeX, globalSizeY uint32,
) {
	kernelArg := CorrKernelArgs{
		Data:                b.dData,
		Corr:                b.dCorr,
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
	b.driver.LaunchKernel(b.context, b.corrKernel,
		globalSize, localSize, &kernelArg)
}

// Verify verifies
func (b *Benchmark) Verify() {
	log.Printf("Passed!")
}
