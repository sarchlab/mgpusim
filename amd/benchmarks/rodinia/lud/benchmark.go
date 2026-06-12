// Package lud implements the LU Decomposition benchmark from Rodinia.
// Three kernels called iteratively for each diagonal block:
//   - lud_diagonal: diagonal block factorization (1D, 32 threads)
//   - lud_perimeter: border block update (1D, 32 threads per block)
//   - lud_internal: internal block update (2D, 32x32 threads)
//
// Input: N×N matrix
// Output: in-place LU decomposition
package lud

import (
	"log"
	"math"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

const blockSize = 32

// DiagonalKernelArgs defines lud_diagonal arguments
// kernarg_size=16: 1 pointer (8) + 2 ints (8) = 16, no implicit args
type DiagonalKernelArgs struct {
	M         driver.Ptr
	MatrixDim int32
	Offset    int32
}

// PerimeterKernelArgs defines lud_perimeter arguments
// kernarg_size=16: 1 pointer (8) + 2 ints (8) = 16, no implicit args
type PerimeterKernelArgs struct {
	M         driver.Ptr
	MatrixDim int32
	Offset    int32
}

// InternalKernelArgs defines lud_internal arguments
// kernarg_size=16: 1 pointer (8) + 2 ints (8) = 16, no implicit args
type InternalKernelArgs struct {
	M         driver.Ptr
	MatrixDim int32
	Offset    int32
}

// Benchmark defines a benchmark
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	diagonalKernel  *insts.KernelCodeObject
	perimeterKernel *insts.KernelCodeObject
	internalKernel  *insts.KernelCodeObject

	MatrixDim int

	matrix    []float32
	cpuMatrix []float32
	dMatrix   driver.Ptr

	useUnifiedMemory bool
}

// NewBenchmark makes a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()

	b.MatrixDim = 256

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
	b.diagonalKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "lud_diagonal")
	if b.diagonalKernel == nil {
		log.Panic("Failed to load lud_diagonal kernel binary")
	}

	b.perimeterKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "lud_perimeter")
	if b.perimeterKernel == nil {
		log.Panic("Failed to load lud_perimeter kernel binary")
	}

	b.internalKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "lud_internal")
	if b.internalKernel == nil {
		log.Panic("Failed to load lud_internal kernel binary")
	}
}

// Run runs
func (b *Benchmark) Run() {
	// Ensure matrix_dim is a multiple of blockSize
	if b.MatrixDim%blockSize != 0 {
		b.MatrixDim = ((b.MatrixDim + blockSize - 1) / blockSize) * blockSize
	}

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
	n := b.MatrixDim

	b.matrix = make([]float32, n*n)

	// Generate a diagonally dominant matrix for numerical stability
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			b.matrix[i*n+j] = rng.Float32()*2.0 - 1.0
		}
		b.matrix[i*n+i] += float32(n)
	}

	if b.useUnifiedMemory {
		b.dMatrix = b.driver.AllocateUnifiedMemory(b.context,
			uint64(n*n*4))
	} else {
		b.dMatrix = b.driver.AllocateMemory(b.context,
			uint64(n*n*4))
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dMatrix, b.matrix)

	numBlocks := b.MatrixDim / blockSize

	for i := 0; i < numBlocks; i++ {
		offset := i * blockSize

		// Diagonal kernel: 1 block, 32 threads
		b.runDiagonal(offset)

		if i < numBlocks-1 {
			periBlocks := numBlocks - i - 1

			// Perimeter kernel: periBlocks blocks, 32 threads each
			b.runPerimeter(offset, periBlocks)

			// Internal kernel: periBlocks x periBlocks blocks, 32x32 threads
			b.runInternal(offset, periBlocks)
		}
	}

	outputMatrix := make([]float32, b.MatrixDim*b.MatrixDim)
	b.driver.MemCopyD2H(b.context, outputMatrix, b.dMatrix)
	b.matrix = outputMatrix
}

func (b *Benchmark) runDiagonal(offset int) {
	localSize := [3]uint16{blockSize, 1, 1}
	globalSize := [3]uint32{blockSize, 1, 1}

	args := DiagonalKernelArgs{
		M:         b.dMatrix,
		MatrixDim: int32(b.MatrixDim),
		Offset:    int32(offset),
	}
	b.driver.LaunchKernel(b.context, b.diagonalKernel,
		globalSize, localSize, &args)
}

func (b *Benchmark) runPerimeter(offset, periBlocks int) {
	localSize := [3]uint16{blockSize, 1, 1}
	gsX := uint32(periBlocks * blockSize)
	globalSize := [3]uint32{gsX, 1, 1}

	args := PerimeterKernelArgs{
		M:         b.dMatrix,
		MatrixDim: int32(b.MatrixDim),
		Offset:    int32(offset),
	}
	b.driver.LaunchKernel(b.context, b.perimeterKernel,
		globalSize, localSize, &args)
}

func (b *Benchmark) runInternal(offset, periBlocks int) {
	localSize := [3]uint16{blockSize, blockSize, 1}
	gsX := uint32(periBlocks * blockSize)
	gsY := uint32(periBlocks * blockSize)
	globalSize := [3]uint32{gsX, gsY, 1}

	args := InternalKernelArgs{
		M:         b.dMatrix,
		MatrixDim: int32(b.MatrixDim),
		Offset:    int32(offset),
	}
	b.driver.LaunchKernel(b.context, b.internalKernel,
		globalSize, localSize, &args)
}

// Verify verifies
func (b *Benchmark) Verify() {
	b.cpuLUD()

	n := b.MatrixDim
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			diff := math.Abs(float64(b.matrix[i*n+j] - b.cpuMatrix[i*n+j]))
			if diff > 1e-2 {
				log.Panicf(
					"mismatch at (%d,%d), expected %f, got %f",
					i, j, b.cpuMatrix[i*n+j], b.matrix[i*n+j])
			}
		}
	}

	log.Printf("Passed!")
}

func (b *Benchmark) cpuLUD() {
	n := b.MatrixDim

	// Re-generate the same matrix
	rng := rand.New(rand.NewSource(42)) //nolint:gosec
	b.cpuMatrix = make([]float32, n*n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			b.cpuMatrix[i*n+j] = rng.Float32()*2.0 - 1.0
		}
		b.cpuMatrix[i*n+i] += float32(n)
	}

	// Standard LU decomposition (no pivoting)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			b.cpuMatrix[j*n+i] /= b.cpuMatrix[i*n+i]
			for k := i + 1; k < n; k++ {
				b.cpuMatrix[j*n+k] -= b.cpuMatrix[j*n+i] * b.cpuMatrix[i*n+k]
			}
		}
	}
}
