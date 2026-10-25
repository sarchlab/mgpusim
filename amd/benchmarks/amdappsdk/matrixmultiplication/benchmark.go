// Package matrixmultiplication implements the matrix multiplication benchmark
// from AMDAPPSDK.
package matrixmultiplication

import (
	"fmt"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"log"
	"math"
	"math/rand"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

// Benchmark defines a benchmark
type Benchmark struct {
	benchmarks.Progress
	initialConfig  checkpointConfig
	configCaptured bool
	queue          driver.CommandQueue
	kernel         *insts.KernelCodeObject
	blockABuf      driver.Ptr
	gA, gB, gC     driver.Ptr
	readback       driver.CommandID
	driver         *driver.Runtime
	context        driver.Context
	gpus           []int

	Arch                      arch.Type
	X, Y, Z                   uint32
	MatrixA, MatrixB, MatrixC Matrix
	useUnifiedMemory          bool
}

// NewBenchmark makes a new benchmark
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	return b
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) setup() {
	b.driver.SelectGPU(b.context, b.gpus[0])
	b.queue = b.driver.CreateCommandQueue(b.context)
	b.initMem()
	b.MatrixC = *NewMatrix(b.MatrixB.Width, b.MatrixA.Height)
	b.loadKernel()
	b.gA, b.gB, b.gC = b.initMemory(&b.MatrixA, &b.MatrixB, &b.MatrixC)
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

	b.MatrixA = *NewMatrix(b.X, b.Y)
	for i := uint32(0); i < b.X; i++ {
		for j := uint32(0); j < b.Y; j++ {
			b.MatrixA.Data[j*b.X+i] = rng.Float32()
			//b.MatrixA.Data[j*b.X+i] = float32(j*b.X + i)
		}
	}

	b.MatrixB = *NewMatrix(b.Z, b.X)
	for i := uint32(0); i < b.Z; i++ {
		for j := uint32(0); j < b.X; j++ {
			b.MatrixB.Data[j*b.Z+i] = rng.Float32()
			//b.MatrixB.Data[j*b.Z+i] = float32(j*b.Z + i)
		}
	}
}

// Verify verifies
func (b *Benchmark) verify() error {
	m := CPUMatrixMultiplier{}
	mCPU := m.Multiply(&b.MatrixA, &b.MatrixB)
	for i := uint32(0); i < mCPU.Width; i++ {
		for j := uint32(0); j < mCPU.Height; j++ {
			index := i + j*mCPU.Width

			if math.Abs(float64(mCPU.Data[index]-b.MatrixC.Data[index])) > 1e-3 {
				return fmt.Errorf("mismatch at [%d, %d]: expected %f, but get %f",
					i, j, mCPU.Data[index], b.MatrixC.Data[index])
			}
		}
	}

	log.Print("Passed!")

	return nil
}

func (b *Benchmark) Step() error {
	switch b.Phase {
	case 0:
		b.initialConfig = b.checkpointConfig()
		b.configCaptured = true
		b.setup()
		b.Phase = 1
	case 1:
		b.launchKernel(b.gA, b.gB, b.gC, &b.MatrixA, &b.MatrixC)
		b.Phase = 2
	case 2:
		b.readback = b.driver.EnqueueMemCopyD2H(b.queue, b.gC, len(b.MatrixC.Data)*4)
		b.Phase = 3
	case 3:
		if err := b.driver.ReadResult(b.readback, b.MatrixC.Data); err != nil {
			return err
		}
		if b.Verification {
			if err := b.verify(); err != nil {
				return err
			}
		}
		b.Ended = true
	}
	return nil
}
