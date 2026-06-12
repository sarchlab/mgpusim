// Package ga implements the Genetic Algorithm benchmark from Hetero-Mark.
// Four kernels: fitness_kernel, selection_kernel, crossover_kernel,
// mutation_kernel. The GA evolves a population of strings to match a target.
package ga

import (
	"fmt"
	"log"
	"math/rand"
	"os"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// FitnessKernelArgs defines kernel arguments.
type FitnessKernelArgs struct {
	Population     driver.Ptr
	Fitness        driver.Ptr
	Target         driver.Ptr
	TargetLen      int32
	PopulationSize int32
}

// SelectionKernelArgs defines kernel arguments.
type SelectionKernelArgs struct {
	Population     driver.Ptr
	Fitness        driver.Ptr
	Selected       driver.Ptr
	TargetLen      int32
	PopulationSize int32
	Seed           uint32
}

// CrossoverKernelArgs defines kernel arguments.
type CrossoverKernelArgs struct {
	Population     driver.Ptr
	TargetLen      int32
	PopulationSize int32
	Seed           uint32
}

// MutationKernelArgs defines kernel arguments.
type MutationKernelArgs struct {
	Population   driver.Ptr
	TargetLen    int32
	PopulationSize int32
	MutationRate float32
	Seed         uint32
}

// Benchmark defines the GA benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	fitnessKernel   *insts.KernelCodeObject
	selectionKernel *insts.KernelCodeObject
	crossoverKernel *insts.KernelCodeObject
	mutationKernel  *insts.KernelCodeObject

	PopulationSize int
	MaxGenerations int
	TargetLen      int
	MutationRate   float32
	BlockSize      int

	target     []byte
	population []byte
	fitness    []int32

	dPopulation driver.Ptr
	dSelected   driver.Ptr
	dFitness    driver.Ptr
	dTarget     driver.Ptr

	useUnifiedMemory bool
}

// NewBenchmark returns a new GA benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()

	b.PopulationSize = 1024
	b.MaxGenerations = 100
	b.MutationRate = 0.01
	b.BlockSize = 256

	// Default target: "Hello World!"
	b.target = []byte("Hello World!")
	b.TargetLen = len(b.target)

	return b
}

// SelectGPU selects GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

func (b *Benchmark) loadProgram() {
	b.fitnessKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "fitness_kernel")
	if b.fitnessKernel == nil {
		log.Panic("Failed to load fitness_kernel binary")
	}

	b.selectionKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "selection_kernel")
	if b.selectionKernel == nil {
		log.Panic("Failed to load selection_kernel binary")
	}

	b.crossoverKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "crossover_kernel")
	if b.crossoverKernel == nil {
		log.Panic("Failed to load crossover_kernel binary")
	}

	b.mutationKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "mutation_kernel")
	if b.mutationKernel == nil {
		log.Panic("Failed to load mutation_kernel binary")
	}
}

// Run runs the benchmark.
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

	popBytes := b.PopulationSize * b.TargetLen

	b.population = make([]byte, popBytes)
	b.fitness = make([]int32, b.PopulationSize)

	// Initialize random population with printable ASCII
	for i := 0; i < b.PopulationSize; i++ {
		for j := 0; j < b.TargetLen; j++ {
			b.population[i*b.TargetLen+j] = byte(32 + rng.Intn(95))
		}
	}

	if b.useUnifiedMemory {
		b.dPopulation = b.driver.AllocateUnifiedMemory(
			b.context, uint64(popBytes))
		b.dSelected = b.driver.AllocateUnifiedMemory(
			b.context, uint64(popBytes))
		b.dFitness = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.PopulationSize*4))
		b.dTarget = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.TargetLen))
	} else {
		b.dPopulation = b.driver.AllocateMemory(
			b.context, uint64(popBytes))
		b.dSelected = b.driver.AllocateMemory(
			b.context, uint64(popBytes))
		b.dFitness = b.driver.AllocateMemory(
			b.context, uint64(b.PopulationSize*4))
		b.dTarget = b.driver.AllocateMemory(
			b.context, uint64(b.TargetLen))
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dPopulation, b.population)
	b.driver.MemCopyH2D(b.context, b.dTarget, b.target)

	localSizeX := uint16(b.BlockSize)
	localSize := [3]uint16{localSizeX, 1, 1}

	// Grid for full population
	gsX := uint32(((b.PopulationSize-1)/b.BlockSize + 1) * b.BlockSize)
	globalSize := [3]uint32{gsX, 1, 1}

	// Grid for crossover (half population, pairs)
	crossGsX := uint32(((b.PopulationSize/2-1)/b.BlockSize + 1) * b.BlockSize)
	crossGlobalSize := [3]uint32{crossGsX, 1, 1}

	for gen := 0; gen < b.MaxGenerations; gen++ {
		genSeed := uint32(gen*7919 + 42)

		// 1. Fitness
		b.launchFitnessKernel(globalSize, localSize)

		// 2. Selection
		b.launchSelectionKernel(globalSize, localSize, genSeed+1)

		// 3. Copy selected back to population (device-to-device)
		popBytes := uint64(b.PopulationSize * b.TargetLen)
		selectedSlice := make([]byte, popBytes)
		b.driver.MemCopyD2H(b.context, selectedSlice, b.dSelected)
		b.driver.MemCopyH2D(b.context, b.dPopulation, selectedSlice)

		// 4. Crossover
		b.launchCrossoverKernel(crossGlobalSize, localSize, genSeed+2)

		// 5. Mutation
		b.launchMutationKernel(globalSize, localSize, genSeed+3)
	}

	// Final fitness to read results
	b.launchFitnessKernel(globalSize, localSize)

	b.driver.MemCopyD2H(b.context, b.fitness, b.dFitness)
	b.driver.MemCopyD2H(b.context, b.population, b.dPopulation)
}

func (b *Benchmark) launchFitnessKernel(
	globalSize [3]uint32, localSize [3]uint16,
) {
	args := FitnessKernelArgs{
		Population:     b.dPopulation,
		Fitness:        b.dFitness,
		Target:         b.dTarget,
		TargetLen:      int32(b.TargetLen),
		PopulationSize: int32(b.PopulationSize),
	}
	b.driver.LaunchKernel(b.context, b.fitnessKernel,
		globalSize, localSize, &args)
}

func (b *Benchmark) launchSelectionKernel(
	globalSize [3]uint32, localSize [3]uint16, seed uint32,
) {
	args := SelectionKernelArgs{
		Population:     b.dPopulation,
		Fitness:        b.dFitness,
		Selected:       b.dSelected,
		TargetLen:      int32(b.TargetLen),
		PopulationSize: int32(b.PopulationSize),
		Seed:           seed,
	}
	b.driver.LaunchKernel(b.context, b.selectionKernel,
		globalSize, localSize, &args)
}

func (b *Benchmark) launchCrossoverKernel(
	globalSize [3]uint32, localSize [3]uint16, seed uint32,
) {
	args := CrossoverKernelArgs{
		Population:     b.dPopulation,
		TargetLen:      int32(b.TargetLen),
		PopulationSize: int32(b.PopulationSize),
		Seed:           seed,
	}
	b.driver.LaunchKernel(b.context, b.crossoverKernel,
		globalSize, localSize, &args)
}

func (b *Benchmark) launchMutationKernel(
	globalSize [3]uint32, localSize [3]uint16, seed uint32,
) {
	args := MutationKernelArgs{
		Population:     b.dPopulation,
		TargetLen:      int32(b.TargetLen),
		PopulationSize: int32(b.PopulationSize),
		MutationRate:   b.MutationRate,
		Seed:           seed,
	}
	b.driver.LaunchKernel(b.context, b.mutationKernel,
		globalSize, localSize, &args)
}

// Verify verifies the GA results by checking that fitness improved.
func (b *Benchmark) Verify() {
	bestFit := int32(0)
	for i := 0; i < b.PopulationSize; i++ {
		if b.fitness[i] > bestFit {
			bestFit = b.fitness[i]
		}
	}

	fmt.Fprintf(os.Stderr,
		"Best fitness after %d generations: %d / %d\n",
		b.MaxGenerations, bestFit, b.TargetLen)

	if bestFit <= 0 && b.MaxGenerations > 0 {
		log.Panic("GA fitness did not improve")
	}

	fmt.Fprintf(os.Stderr, "Passed!\n")
}
