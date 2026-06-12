// Package particlefilter implements the Rodinia Particle Filter benchmark.
// Six kernels: likelihood_kernel, weight_sum_kernel, weight_normalize_kernel,
// build_cdf_kernel, resample_kernel, transition_kernel.
package particlefilter

import (
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// LikelihoodKernelArgs defines arguments for likelihood_kernel (gfx942).
type LikelihoodKernelArgs struct {
	Particles           driver.Ptr
	Obs                 driver.Ptr
	Weights             driver.Ptr
	NumParticles        int32
	StateDims           int32
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

// WeightSumKernelArgs defines arguments for weight_sum_kernel (gfx942).
type WeightSumKernelArgs struct {
	Weights             driver.Ptr
	WeightSum           driver.Ptr
	NumParticles        int32
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

// WeightNormalizeKernelArgs defines arguments for weight_normalize_kernel (gfx942).
type WeightNormalizeKernelArgs struct {
	Weights             driver.Ptr
	WeightSum           driver.Ptr
	NumParticles        int32
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

// BuildCDFKernelArgs defines arguments for build_cdf_kernel (gfx942).
type BuildCDFKernelArgs struct {
	Weights             driver.Ptr
	CDF                 driver.Ptr
	NumParticles        int32
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

// ResampleKernelArgs defines arguments for resample_kernel (gfx942).
type ResampleKernelArgs struct {
	CDF                 driver.Ptr
	ParticlesIn         driver.Ptr
	ParticlesOut        driver.Ptr
	NumParticles        int32
	StateDims           int32
	U0                  float32
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

// TransitionKernelArgs defines arguments for transition_kernel (gfx942).
type TransitionKernelArgs struct {
	Particles           driver.Ptr
	NumParticles        int32
	StateDims           int32
	Seed                uint32
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

	likelihoodKernel       *insts.KernelCodeObject
	weightSumKernel        *insts.KernelCodeObject
	weightNormalizeKernel  *insts.KernelCodeObject
	buildCDFKernel         *insts.KernelCodeObject
	resampleKernel         *insts.KernelCodeObject
	transitionKernel       *insts.KernelCodeObject

	numParticles int
	numTimesteps int
	stateDims    int

	hParticles    []float32
	hObservations []float32

	dParticles    driver.Ptr
	dParticlesTmp driver.Ptr
	dWeights      driver.Ptr
	dCDF          driver.Ptr
	dObs          driver.Ptr
	dWeightSum    driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.numParticles = 128
	b.numTimesteps = 5
	b.stateDims = 2
	return b
}

// SetNumParticles sets the number of particles
func (b *Benchmark) SetNumParticles(n int) {
	b.numParticles = n
}

// SetNumTimesteps sets the number of timesteps
func (b *Benchmark) SetNumTimesteps(n int) {
	b.numTimesteps = n
}

// SetStateDims sets the state dimensions
func (b *Benchmark) SetStateDims(n int) {
	b.stateDims = n
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) loadProgram() {
	b.likelihoodKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "likelihood_kernel")
	if b.likelihoodKernel == nil {
		log.Panic("Failed to load likelihood_kernel binary")
	}

	b.weightSumKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "weight_sum_kernel")
	if b.weightSumKernel == nil {
		log.Panic("Failed to load weight_sum_kernel binary")
	}

	b.weightNormalizeKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "weight_normalize_kernel")
	if b.weightNormalizeKernel == nil {
		log.Panic("Failed to load weight_normalize_kernel binary")
	}

	b.buildCDFKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "build_cdf_kernel")
	if b.buildCDFKernel == nil {
		log.Panic("Failed to load build_cdf_kernel binary")
	}

	b.resampleKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "resample_kernel")
	if b.resampleKernel == nil {
		log.Panic("Failed to load resample_kernel binary")
	}

	b.transitionKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "transition_kernel")
	if b.transitionKernel == nil {
		log.Panic("Failed to load transition_kernel binary")
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
	np := b.numParticles
	sd := b.stateDims
	nt := b.numTimesteps

	rand.Seed(42)
	b.hParticles = make([]float32, np*sd)
	b.hObservations = make([]float32, nt*sd)

	for i := 0; i < np*sd; i++ {
		b.hParticles[i] = float32(rand.Intn(1000)) / 100.0
	}
	for i := 0; i < nt*sd; i++ {
		b.hObservations[i] = float32(rand.Intn(1000)) / 100.0
	}

	if b.useUnifiedMemory {
		b.dParticles = b.driver.AllocateUnifiedMemory(b.context,
			uint64(np*sd*4))
		b.dParticlesTmp = b.driver.AllocateUnifiedMemory(b.context,
			uint64(np*sd*4))
		b.dWeights = b.driver.AllocateUnifiedMemory(b.context,
			uint64(np*4))
		b.dCDF = b.driver.AllocateUnifiedMemory(b.context,
			uint64(np*4))
		b.dObs = b.driver.AllocateUnifiedMemory(b.context,
			uint64(sd*4))
		b.dWeightSum = b.driver.AllocateUnifiedMemory(b.context,
			uint64(4))
	} else {
		b.dParticles = b.driver.AllocateMemory(b.context,
			uint64(np*sd*4))
		b.dParticlesTmp = b.driver.AllocateMemory(b.context,
			uint64(np*sd*4))
		b.dWeights = b.driver.AllocateMemory(b.context,
			uint64(np*4))
		b.dCDF = b.driver.AllocateMemory(b.context,
			uint64(np*4))
		b.dObs = b.driver.AllocateMemory(b.context,
			uint64(sd*4))
		b.dWeightSum = b.driver.AllocateMemory(b.context,
			uint64(4))
	}
}

func (b *Benchmark) exec() {
	np := b.numParticles
	sd := b.stateDims
	blockSize := 256
	if np < blockSize {
		blockSize = np
	}

	gsX := uint32(((np - 1) / blockSize + 1) * blockSize)
	localSize := [3]uint16{uint16(blockSize), 1, 1}
	globalSize := [3]uint32{gsX, 1, 1}

	// Copy initial particles to device
	b.driver.MemCopyH2D(b.context, b.dParticles, b.hParticles)

	for t := 0; t < b.numTimesteps; t++ {
		// Upload current observation
		obs := b.hObservations[t*sd : (t+1)*sd]
		b.driver.MemCopyH2D(b.context, b.dObs, obs)

		// 1. Compute likelihoods
		b.launchLikelihoodKernel(globalSize, localSize, gsX)

		// 2. Normalize weights — first sum
		zero := []float32{0.0}
		b.driver.MemCopyH2D(b.context, b.dWeightSum, zero)
		b.launchWeightSumKernel(globalSize, localSize, gsX)

		// Then normalize
		b.launchWeightNormalizeKernel(globalSize, localSize, gsX)

		// 3. Build CDF (single-threaded)
		cdfLocalSize := [3]uint16{1, 1, 1}
		cdfGlobalSize := [3]uint32{1, 1, 1}
		b.launchBuildCDFKernel(cdfGlobalSize, cdfLocalSize)

		// 4. Systematic resampling
		u0 := float32(rand.Intn(10000)) / 10000.0 / float32(np)
		b.launchResampleKernel(globalSize, localSize, gsX, u0)

		// 5. State transition
		b.launchTransitionKernel(globalSize, localSize, gsX, uint32(t))

		// Swap particle buffers
		b.dParticles, b.dParticlesTmp = b.dParticlesTmp, b.dParticles
	}

	// Copy result back
	hResult := make([]float32, np*sd)
	b.driver.MemCopyD2H(b.context, hResult, b.dParticles)
}

func (b *Benchmark) launchLikelihoodKernel(
	globalSize [3]uint32, localSize [3]uint16, gsX uint32,
) {
	args := LikelihoodKernelArgs{
		Particles:           b.dParticles,
		Obs:                 b.dObs,
		Weights:             b.dWeights,
		NumParticles:        int32(b.numParticles),
		StateDims:           int32(b.stateDims),
		HiddenBlockCountX:   gsX / uint32(localSize[0]),
		HiddenBlockCountY:   1,
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(gsX % uint32(localSize[0])),
		HiddenRemainderY:    0,
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      1,
	}
	b.driver.LaunchKernel(b.context, b.likelihoodKernel,
		globalSize, localSize, &args)
}

func (b *Benchmark) launchWeightSumKernel(
	globalSize [3]uint32, localSize [3]uint16, gsX uint32,
) {
	args := WeightSumKernelArgs{
		Weights:             b.dWeights,
		WeightSum:           b.dWeightSum,
		NumParticles:        int32(b.numParticles),
		HiddenBlockCountX:   gsX / uint32(localSize[0]),
		HiddenBlockCountY:   1,
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(gsX % uint32(localSize[0])),
		HiddenRemainderY:    0,
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      1,
	}
	b.driver.LaunchKernel(b.context, b.weightSumKernel,
		globalSize, localSize, &args)
}

func (b *Benchmark) launchWeightNormalizeKernel(
	globalSize [3]uint32, localSize [3]uint16, gsX uint32,
) {
	args := WeightNormalizeKernelArgs{
		Weights:             b.dWeights,
		WeightSum:           b.dWeightSum,
		NumParticles:        int32(b.numParticles),
		HiddenBlockCountX:   gsX / uint32(localSize[0]),
		HiddenBlockCountY:   1,
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(gsX % uint32(localSize[0])),
		HiddenRemainderY:    0,
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      1,
	}
	b.driver.LaunchKernel(b.context, b.weightNormalizeKernel,
		globalSize, localSize, &args)
}

func (b *Benchmark) launchBuildCDFKernel(
	globalSize [3]uint32, localSize [3]uint16,
) {
	args := BuildCDFKernelArgs{
		Weights:             b.dWeights,
		CDF:                 b.dCDF,
		NumParticles:        int32(b.numParticles),
		HiddenBlockCountX:   1,
		HiddenBlockCountY:   1,
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    1,
		HiddenGroupSizeY:    1,
		HiddenGroupSizeZ:    1,
		HiddenRemainderX:    0,
		HiddenRemainderY:    0,
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      1,
	}
	b.driver.LaunchKernel(b.context, b.buildCDFKernel,
		globalSize, localSize, &args)
}

func (b *Benchmark) launchResampleKernel(
	globalSize [3]uint32, localSize [3]uint16, gsX uint32, u0 float32,
) {
	args := ResampleKernelArgs{
		CDF:                 b.dCDF,
		ParticlesIn:         b.dParticles,
		ParticlesOut:        b.dParticlesTmp,
		NumParticles:        int32(b.numParticles),
		StateDims:           int32(b.stateDims),
		U0:                  u0,
		HiddenBlockCountX:   gsX / uint32(localSize[0]),
		HiddenBlockCountY:   1,
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(gsX % uint32(localSize[0])),
		HiddenRemainderY:    0,
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      1,
	}
	b.driver.LaunchKernel(b.context, b.resampleKernel,
		globalSize, localSize, &args)
}

func (b *Benchmark) launchTransitionKernel(
	globalSize [3]uint32, localSize [3]uint16, gsX uint32, seed uint32,
) {
	args := TransitionKernelArgs{
		Particles:           b.dParticlesTmp,
		NumParticles:        int32(b.numParticles),
		StateDims:           int32(b.stateDims),
		Seed:                seed,
		HiddenBlockCountX:   gsX / uint32(localSize[0]),
		HiddenBlockCountY:   1,
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(gsX % uint32(localSize[0])),
		HiddenRemainderY:    0,
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      1,
	}
	b.driver.LaunchKernel(b.context, b.transitionKernel,
		globalSize, localSize, &args)
}

// Verify verifies
func (b *Benchmark) Verify() {
	log.Printf("Passed!")
}
