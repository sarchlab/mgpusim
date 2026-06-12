// Package softmax implements row-wise softmax microbenchmark.
package softmax

import (
	"log"
	"math"

	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments for softmax_kernel.
type KernelArgs struct {
	A    driver.Ptr
	B    driver.Ptr
	Rows int32
	Cols int32
}

// Benchmark defines the softmax benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.KernelCodeObject

	Rows      int
	Cols      int
	BlockSize int

	useUnifiedMemory bool

	hA []float32
	hB []float32

	dA driver.Ptr
	dB driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new softmax benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.Rows = 1024
	b.Cols = 1024
	b.BlockSize = 256
	return b
}

// SelectGPU selects GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// SetRows sets number of rows.
func (b *Benchmark) SetRows(r int) {
	b.Rows = r
}

// SetCols sets number of columns.
func (b *Benchmark) SetCols(c int) {
	b.Cols = c
}

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "softmax_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load softmax_kernel")
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
	n := b.Rows * b.Cols

	b.hA = make([]float32, n)
	b.hB = make([]float32, n)

	for i := 0; i < n; i++ {
		b.hA[i] = float32(i%1000)*0.002 - 1.0
	}

	bytes := uint64(n * 4)

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dB = b.driver.AllocateUnifiedMemory(b.context, bytes)
	} else {
		b.dA = b.driver.AllocateMemory(b.context, bytes)
		b.dB = b.driver.AllocateMemory(b.context, bytes)
	}

	b.driver.MemCopyH2D(b.context, b.dA, b.hA)
}

func (b *Benchmark) exec() {
	blockSize := b.BlockSize

	// One workgroup per row
	globalSize := [3]uint32{uint32(b.Rows * blockSize), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	args := KernelArgs{
		A:    b.dA,
		B:    b.dB,
		Rows: int32(b.Rows),
		Cols: int32(b.Cols),
	}
	b.driver.LaunchKernel(b.context, b.kernel,
		globalSize, localSize, &args)

	b.driver.MemCopyD2H(b.context, b.hB, b.dB)
}

// Verify verifies the result.
func (b *Benchmark) Verify() {
	errors := 0

	for r := 0; r < b.Rows; r++ {
		offset := r * b.Cols

		// Find row max
		rowMax := float64(b.hA[offset])
		for c := 1; c < b.Cols; c++ {
			v := float64(b.hA[offset+c])
			if v > rowMax {
				rowMax = v
			}
		}

		// Compute exp and sum
		rowSum := 0.0
		expected := make([]float64, b.Cols)
		for c := 0; c < b.Cols; c++ {
			expected[c] = math.Exp(float64(b.hA[offset+c]) - rowMax)
			rowSum += expected[c]
		}

		// Normalize and check
		for c := 0; c < b.Cols; c++ {
			exp := float32(expected[c] / rowSum)
			diff := math.Abs(float64(b.hB[offset+c] - exp))
			tol := 1e-4*math.Abs(float64(exp)) + 1e-6

			if diff > tol {
				if errors < 10 {
					log.Printf("Mismatch at [%d,%d]: got %f, expected %f",
						r, c, b.hB[offset+c], exp)
				}
				errors++
			}
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in softmax verification", errors)
	}
	log.Printf("Passed!")
}
