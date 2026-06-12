// Package backprop implements the Back Propagation benchmark from Rodinia.
// Two kernels: layerforward (forward pass with sigmoid activation),
// adjust_weights (weight update with learning rate and momentum).
// Input: num_input input units, num_hidden=16 hidden units
// Output: updated weights after forward pass + weight adjustment
package backprop

import (
	"log"
	"math"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

const (
	verificationTolerance     = 1e-3
	maxLoggedWeightMismatches = 10
)

// LayerforwardKernelArgs defines layerforward arguments
type LayerforwardKernelArgs struct {
	Input               driver.Ptr
	Weights             driver.Ptr
	Output              driver.Ptr
	NumInput            int32
	NumOutput           int32
	Pad1                int16 //nolint:unused
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Pad0                [16]byte //nolint:unused
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

// AdjustWeightsKernelArgs defines adjust_weights arguments
type AdjustWeightsKernelArgs struct {
	Delta               driver.Ptr
	Input               driver.Ptr
	Weights             driver.Ptr
	PrevWeights         driver.Ptr
	NumInput            int32
	NumOutput           int32
	LearningRate        float32
	Momentum            float32
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Pad0                [16]byte //nolint:unused
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

	layerforwardKernel  *insts.KernelCodeObject
	adjustWeightsKernel *insts.KernelCodeObject

	NumInput  int
	numHidden int

	// SkipLayerforward, when true, omits the layerforward kernel from
	// exec(). The adjust_weights kernel runs alone, which lets benchmarks
	// time the second kernel without the layerforward contribution.
	SkipLayerforward bool

	blockSize    int
	learningRate float32
	momentum     float32

	input       []float32
	weights     []float32
	prevWeights []float32
	hidden      []float32
	delta       []float32
	cpuHidden   []float32
	cpuWeights  []float32

	dInput       driver.Ptr
	dWeights     driver.Ptr
	dPrevWeights driver.Ptr
	dHidden      driver.Ptr
	dDelta       driver.Ptr

	useUnifiedMemory bool
}

// NewBenchmark makes a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()

	b.NumInput = 65536
	b.numHidden = 16
	b.blockSize = 256
	b.learningRate = 0.3
	b.momentum = 0.5

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
	b.layerforwardKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "layerforward")
	if b.layerforwardKernel == nil {
		log.Panic("Failed to load layerforward kernel binary")
	}

	b.adjustWeightsKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "adjust_weights")
	if b.adjustWeightsKernel == nil {
		log.Panic("Failed to load adjust_weights kernel binary")
	}
}

// Run runs
func (b *Benchmark) Run() {
	b.loadProgram()

	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues,
			b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	rng := rand.New(rand.NewSource(42)) //nolint:gosec

	numWeights := (b.NumInput + 1) * b.numHidden

	b.input = make([]float32, b.NumInput)
	b.weights = make([]float32, numWeights)
	b.prevWeights = make([]float32, numWeights)
	b.hidden = make([]float32, b.numHidden)
	b.delta = make([]float32, b.numHidden)

	for i := 0; i < b.NumInput; i++ {
		b.input[i] = float32(rng.Intn(100)) / 100.0
	}

	for i := 0; i < numWeights; i++ {
		b.weights[i] = float32(rng.Intn(100))/1000.0 - 0.05
	}

	for i := 0; i < numWeights; i++ {
		b.prevWeights[i] = 0.0
	}

	for i := 0; i < b.numHidden; i++ {
		b.delta[i] = float32(rng.Intn(100))/1000.0 - 0.05
	}

	if b.useUnifiedMemory {
		b.dInput = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NumInput*4))
		b.dWeights = b.driver.AllocateUnifiedMemory(b.context,
			uint64(numWeights*4))
		b.dPrevWeights = b.driver.AllocateUnifiedMemory(b.context,
			uint64(numWeights*4))
		b.dHidden = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.numHidden*4))
		b.dDelta = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.numHidden*4))
	} else {
		b.dInput = b.driver.AllocateMemory(b.context,
			uint64(b.NumInput*4))
		b.dWeights = b.driver.AllocateMemory(b.context,
			uint64(numWeights*4))
		b.dPrevWeights = b.driver.AllocateMemory(b.context,
			uint64(numWeights*4))
		b.dHidden = b.driver.AllocateMemory(b.context,
			uint64(b.numHidden*4))
		b.dDelta = b.driver.AllocateMemory(b.context,
			uint64(b.numHidden*4))
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dInput, b.input)
	b.driver.MemCopyH2D(b.context, b.dWeights, b.weights)
	b.driver.MemCopyH2D(b.context, b.dPrevWeights, b.prevWeights)
	b.driver.MemCopyH2D(b.context, b.dDelta, b.delta)

	// Kernel 1: layerforward - 1D, numHidden threads
	if !b.SkipLayerforward {
		b.runLayerforward()
	}

	// Kernel 2: adjust_weights - 2D grid
	b.runAdjustWeights()

	// Read back results
	b.driver.MemCopyD2H(b.context, b.hidden, b.dHidden)
	numWeights := (b.NumInput + 1) * b.numHidden
	outputWeights := make([]float32, numWeights)
	b.driver.MemCopyD2H(b.context, outputWeights, b.dWeights)
	b.weights = outputWeights
}

func (b *Benchmark) runLayerforward() {
	localSizeX := uint16(b.blockSize)
	if b.numHidden < b.blockSize {
		localSizeX = uint16(b.numHidden)
	}
	localSize := [3]uint16{localSizeX, 1, 1}
	gsX := uint32(((b.numHidden-1)/int(localSizeX) + 1) * int(localSizeX))
	globalSize := [3]uint32{gsX, 1, 1}

	args := LayerforwardKernelArgs{
		Input:               b.dInput,
		Weights:             b.dWeights,
		Output:              b.dHidden,
		NumInput:            int32(b.NumInput),
		NumOutput:           int32(b.numHidden),
		HiddenBlockCountX:   gsX / uint32(localSizeX),
		HiddenBlockCountY:   1,
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSizeX,
		HiddenGroupSizeY:    1,
		HiddenGroupSizeZ:    1,
		HiddenRemainderX:    uint16(gsX % uint32(localSizeX)),
		HiddenRemainderY:    0,
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      1,
	}
	b.driver.LaunchKernel(b.context, b.layerforwardKernel,
		globalSize, localSize, &args)
}

func (b *Benchmark) runAdjustWeights() {
	localSize := [3]uint16{uint16(b.blockSize), 1, 1}
	gsX := uint32(((b.NumInput)/b.blockSize + 1) * b.blockSize)
	gsY := uint32(b.numHidden)
	globalSize := [3]uint32{gsX, gsY, 1}

	args := AdjustWeightsKernelArgs{
		Delta:               b.dDelta,
		Input:               b.dInput,
		Weights:             b.dWeights,
		PrevWeights:         b.dPrevWeights,
		NumInput:            int32(b.NumInput),
		NumOutput:           int32(b.numHidden),
		LearningRate:        b.learningRate,
		Momentum:            b.momentum,
		HiddenBlockCountX:   gsX / uint32(localSize[0]),
		HiddenBlockCountY:   gsY / uint32(localSize[1]),
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(gsX % uint32(localSize[0])),
		HiddenRemainderY:    uint16(gsY % uint32(localSize[1])),
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      2,
	}
	b.driver.LaunchKernel(b.context, b.adjustWeightsKernel,
		globalSize, localSize, &args)
}

// Verify verifies
func (b *Benchmark) Verify() {
	b.cpuBackprop()

	if !b.SkipLayerforward {
		b.verifyHiddenOutput()
	}
	b.verifyAdjustedWeights()

	log.Printf("Passed!")
}

func (b *Benchmark) verifyHiddenOutput() {
	if len(b.hidden) != b.numHidden {
		log.Panicf("hidden output verification requires %d values, got %d",
			b.numHidden, len(b.hidden))
	}

	mismatchCount := 0
	for i := 0; i < b.numHidden; i++ {
		diff := math.Abs(float64(b.hidden[i] - b.cpuHidden[i]))
		if diff > verificationTolerance {
			log.Printf("mismatch at hidden[%d], expected %v, got %v (diff=%v)",
				i, b.cpuHidden[i], b.hidden[i], diff)
			mismatchCount++
		}
	}

	if mismatchCount > 0 {
		log.Panicf("hidden output verification failed: %d mismatches found",
			mismatchCount)
	}
}

func (b *Benchmark) verifyAdjustedWeights() {
	expectedWeights := (b.NumInput + 1) * b.numHidden
	if len(b.weights) != expectedWeights {
		log.Panicf("adjusted-weight verification requires %d values, got %d",
			expectedWeights, len(b.weights))
	}
	if len(b.cpuWeights) != expectedWeights {
		log.Panicf("adjusted-weight verification expected %d CPU values, got %d",
			expectedWeights, len(b.cpuWeights))
	}

	mismatchCount := 0
	for i := 0; i < expectedWeights; i++ {
		diff := math.Abs(float64(b.weights[i] - b.cpuWeights[i]))
		if diff > verificationTolerance {
			if mismatchCount < maxLoggedWeightMismatches {
				log.Printf("mismatch at weights[%d], expected %v, got %v (diff=%v)",
					i, b.cpuWeights[i], b.weights[i], diff)
			}
			mismatchCount++
		}
	}

	if mismatchCount > 0 {
		log.Panicf("adjusted-weight verification failed: %d mismatches found",
			mismatchCount)
	}
}

func (b *Benchmark) cpuBackprop() {
	b.cpuHidden = make([]float32, b.numHidden)
	numWeights := (b.NumInput + 1) * b.numHidden

	// Reset for CPU using same seed as initMem
	rng := rand.New(rand.NewSource(42)) //nolint:gosec
	cpuInput := make([]float32, b.NumInput)
	cpuWeights := make([]float32, numWeights)
	cpuPrevWeights := make([]float32, numWeights)
	cpuDelta := make([]float32, b.numHidden)

	for i := 0; i < b.NumInput; i++ {
		cpuInput[i] = float32(rng.Intn(100)) / 100.0
	}
	for i := 0; i < numWeights; i++ {
		cpuWeights[i] = float32(rng.Intn(100))/1000.0 - 0.05
	}
	for i := 0; i < numWeights; i++ {
		cpuPrevWeights[i] = 0.0
	}
	for i := 0; i < b.numHidden; i++ {
		cpuDelta[i] = float32(rng.Intn(100))/1000.0 - 0.05
	}

	// layerforward
	for idx := 0; idx < b.numHidden; idx++ {
		sum := float32(0.0)
		for j := 0; j < b.NumInput; j++ {
			sum += cpuInput[j] * cpuWeights[idx*b.NumInput+j]
		}
		sum += cpuWeights[idx*b.NumInput+b.NumInput]
		b.cpuHidden[idx] = float32(1.0 / (1.0 + math.Exp(float64(-sum))))
	}

	// adjust_weights
	for by := 0; by < b.numHidden; by++ {
		for tx := 0; tx <= b.NumInput; tx++ {
			idx := by*(b.NumInput+1) + tx
			inp := float32(1.0) // bias
			if tx < b.NumInput {
				inp = cpuInput[tx]
			}
			newDw := b.learningRate*cpuDelta[by]*inp +
				b.momentum*cpuPrevWeights[idx]
			cpuWeights[idx] += newDw
			cpuPrevWeights[idx] = newDw
		}
	}
	b.cpuWeights = cpuWeights
}
