// Package matrixmultiplication implements the matrix multiplication benchmark
// from AMDAPPSDK.
package matrixmultiplication

import (
	"log"
	"math"
	"math/rand"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

// Benchmark defines a benchmark
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int

	Arch                      arch.Type
	X, Y, Z                   uint32
	MatrixA, MatrixB, MatrixC *Matrix
	useUnifiedMemory          bool
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

// Run runs
func (b *Benchmark) Run() {
	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) initMem() {
	// Use a local random source so the input is reproducible. rand.Seed has
	// been a no-op since Go 1.24, so seeding the global generator no longer
	// produces a deterministic sequence.
	rng := rand.New(rand.NewSource(0))

	b.MatrixA = NewMatrix(b.X, b.Y)
	for i := uint32(0); i < b.X; i++ {
		for j := uint32(0); j < b.Y; j++ {
			b.MatrixA.Data[j*b.X+i] = rng.Float32()
			//b.MatrixA.Data[j*b.X+i] = float32(j*b.X + i)
		}
	}

	b.MatrixB = NewMatrix(b.Z, b.X)
	for i := uint32(0); i < b.Z; i++ {
		for j := uint32(0); j < b.X; j++ {
			b.MatrixB.Data[j*b.Z+i] = rng.Float32()
			//b.MatrixB.Data[j*b.Z+i] = float32(j*b.Z + i)
		}
	}
}

func (b *Benchmark) exec() {
	m := NewGPUMatrixMultiplier(b.driver, b.context)
	m.SelectGPU(b.gpus)
	m.Arch = b.Arch
	m.useUnifiedMemory = b.useUnifiedMemory
	b.MatrixC = m.Multiply(b.MatrixA, b.MatrixB)
}

// Verify verifies
func (b *Benchmark) Verify() {
	m := CPUMatrixMultiplier{}
	mCPU := m.Multiply(b.MatrixA, b.MatrixB)
	for i := uint32(0); i < mCPU.Width; i++ {
		for j := uint32(0); i < mCPU.Width; i++ {
			index := i + j*mCPU.Width

			if math.Abs(float64(mCPU.Data[index]-b.MatrixC.Data[index])) > 1e-3 {
				log.Panicf("mismatch at [%d, %d]: expected %f, but get %f",
					i, j, mCPU.Data[index], b.MatrixC.Data[index])
			}
		}
	}

	log.Print("Passed!")
}
