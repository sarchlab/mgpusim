package matrixmultiplication

import (
	"log"
	"math/rand"

	"gitlab.com/akita/gcn3/driver"
)

type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context

	X, Y, Z                   uint32
	MatrixA, MatrixB, MatrixC *Matrix
}

func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	return b
}

func (b *Benchmark) Run() {
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	rand.Seed(0)

	b.MatrixA = NewMatrix(b.X, b.Y)
	for i := uint32(0); i < b.X; i++ {
		for j := uint32(0); j < b.Y; j++ {
			b.MatrixA.Data[j*b.X+i] = 1.0
		}
	}

	b.MatrixB = NewMatrix(b.Z, b.X)
	for i := uint32(0); i < b.Z; i++ {
		for j := uint32(0); j < b.X; j++ {
			b.MatrixB.Data[j*b.Z+i] = 1.0
		}
	}
}

func (b *Benchmark) exec() {
	m := NewGPUMatrixMultiplier(b.driver, b.context)
	b.MatrixC = m.Multiply(b.MatrixA, b.MatrixB)
}

func (b *Benchmark) Verify() {
	m := CPUMatrixMultiplier{}
	mCPU := m.Multiply(b.MatrixA, b.MatrixB)
	for i := uint32(0); i < mCPU.Width; i++ {
		for j := uint32(0); i < mCPU.Width; i++ {
			index := i + j*mCPU.Width

			if mCPU.Data[index] != b.MatrixC.Data[index] {
				log.Panicf("mismatch at [%d, %d]: expected %f, but get %f",
					i, j, mCPU.Data[index], b.MatrixC.Data[index])
			}
		}
	}

	log.Print("Passed!")
}
