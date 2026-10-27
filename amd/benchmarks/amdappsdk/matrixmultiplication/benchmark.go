// Package matrixmultiplication implements the matrix multiplication benchmark
// from AMDAPPSDK.
package matrixmultiplication

import (
	"fmt"
	"log"
	"math"
	"math/rand"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// Benchmark defines a benchmark
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]

	kernel *insts.KernelCodeObject

	rt *gpuruntime.Runtime
}

// NewBenchmark makes a new benchmark
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)

	b.rt = rt
	b.Context = rt.Init()
	return b
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.Gpus = gpus
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) setup() {
	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.initMem()
	b.MatrixC = *NewMatrix(b.MatrixB.Width, b.MatrixA.Height)
	b.loadKernel()
	b.GA, b.GB, b.GC = b.initMemory(&b.MatrixA, &b.MatrixB, &b.MatrixC)
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
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
	if err := b.Prepare(); err != nil {
		return err
	}
	if err := b.ReleaseUploads(b.rt); err != nil {
		return err
	}

	switch b.Phase {
	case 0:
		b.setup()
		b.Phase = 1
	case 1:
		b.launchKernel(b.GA, b.GB, b.GC, &b.MatrixA, &b.MatrixC)
		b.Phase = 2
	case 2:
		b.Readback = benchmarks.Download(b.rt, b.Queue, b.GC, len(b.MatrixC.Data)*4)
		b.Phase = 3
	case 3:
		if err := benchmarks.Read(b.rt, b.Readback, b.MatrixC.Data); err != nil {
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
