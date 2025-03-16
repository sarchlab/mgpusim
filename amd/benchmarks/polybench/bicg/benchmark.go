// Package bicg implements the bicg benchmark from Polybench.
package bicg

import (
	"log"
	"math"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/kernels"
)

// Kernel1Args list first set of kernel arguments
type Kernel1Args struct {
	A      driver.Ptr
	P      driver.Ptr
	Q      driver.Ptr
	NX, NY int32
}

// Kernel2Args list second set of kernel arguments
type Kernel2Args struct {
	A      driver.Ptr
	R      driver.Ptr
	S      driver.Ptr
	NX, NY int32
}

// Benchmark defines a benchmark
type Benchmark struct {
	driver           *driver.Driver
	context          *driver.Context
	gpus             []int
	queues           []*driver.CommandQueue
	kernel1, kernel2 *insts.HsaCo

	NX, NY             int
	a, r, s, p, q      []float32
	sOutput, qOutput   []float32
	cpuS, cpuQ         []float32
	dA, dR, dS, dP, dQ driver.Ptr

	useUnifiedMemory bool
}

// NewBenchmark makes a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.loadProgram()
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

//go:embed kernels.hsaco
var hsacoBytes []byte

func (b *Benchmark) loadProgram() {
	b.kernel1 = kernels.LoadProgramFromMemory(
		hsacoBytes, "bicgKernel1")
	if b.kernel1 == nil {
		log.Panic("Failed to load kernel binary")
	}

	b.kernel2 = kernels.LoadProgramFromMemory(
		hsacoBytes, "bicgKernel2")
	if b.kernel2 == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// Run runs
func (b *Benchmark) Run() {
	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues, b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	rand.Seed(1)
	b.a = make([]float32, b.NX*b.NY)
	b.r = make([]float32, b.NX)
	b.s = make([]float32, b.NY)
	b.p = make([]float32, b.NY)
	b.q = make([]float32, b.NX)
	b.sOutput = make([]float32, b.NY)
	b.qOutput = make([]float32, b.NX)

	for i := 0; i < b.NX; i++ {
		b.r[i] = float32(i) * math.Pi
		for j := 0; j < b.NY; j++ {
			b.a[i*b.NY+j] = float32(i) * float32(j) / float32(b.NX)
		}
	}

	for i := 0; i < b.NY; i++ {
		b.p[i] = float32(i) * math.Pi
	}

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NY*b.NX*4))
		b.dR = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NX*4))
		b.dS = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NY*4))
		b.dP = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NY*4))
		b.dQ = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NX*4))
	} else {
		b.dA = b.driver.AllocateMemory(b.context,
			uint64(b.NY*b.NX*4))
		b.dR = b.driver.AllocateMemory(b.context,
			uint64(b.NX*4))
		b.dS = b.driver.AllocateMemory(b.context,
			uint64(b.NY*4))
		b.dP = b.driver.AllocateMemory(b.context,
			uint64(b.NY*4))
		b.dQ = b.driver.AllocateMemory(b.context,
			uint64(b.NX*4))
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dA, b.a)
	b.driver.MemCopyH2D(b.context, b.dR, b.r)
	b.driver.MemCopyH2D(b.context, b.dP, b.p)

	localSize := [3]uint16{256, 1, 1}
	globalSizeX := uint32(((b.NX-1)/256 + 1) * 256)
	globalSize := [3]uint32{globalSizeX, 1, 1}

	kernel1Arg := Kernel1Args{
		A:  b.dA,
		P:  b.dP,
		Q:  b.dQ,
		NX: int32(b.NX),
		NY: int32(b.NY),
	}
	b.driver.LaunchKernel(b.context, b.kernel1,
		globalSize, localSize, &kernel1Arg)

	globalSizeX = uint32(((b.NY-1)/256 + 1) * 256)
	globalSize = [3]uint32{globalSizeX, 1, 1}

	kernel2Arg := Kernel2Args{
		A:  b.dA,
		R:  b.dR,
		S:  b.dS,
		NX: int32(b.NX),
		NY: int32(b.NY),
	}
	b.driver.LaunchKernel(b.context, b.kernel2,
		globalSize, localSize, &kernel2Arg)

	b.driver.MemCopyD2H(b.context, b.sOutput, b.dS)
	b.driver.MemCopyD2H(b.context, b.qOutput, b.dQ)
}

// Verify verifies
func (b *Benchmark) Verify() {
	b.cpuBicg()

	for i := 0; i < b.NY; i++ {
		if b.cpuS[i] != b.sOutput[i] {
			log.Panicf("Mismatch in s at %d, expected %f, but get %f",
				i, b.cpuS[i], b.sOutput[i])
		}
	}

	for i := 0; i < b.NX; i++ {
		if b.cpuQ[i] != b.qOutput[i] {
			log.Panicf("Mismatch in q at %d, expected %f, but get %f",
				i, b.cpuQ[i], b.qOutput[i])
		}
	}

	log.Printf("Passed!\n")
}

func (b *Benchmark) cpuBicg() {
	b.cpuS = make([]float32, b.NY)
	b.cpuQ = make([]float32, b.NX)

	for i := 0; i < b.NX; i++ {
		for j := 0; j < b.NY; j++ {
			b.cpuS[j] += b.r[i] * b.a[i*b.NY+j]
			b.cpuQ[i] += b.p[j] * b.a[i*b.NY+j]
		}
	}
}
