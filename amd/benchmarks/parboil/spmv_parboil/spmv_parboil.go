// Package spmv_parboil implements the Parboil SpMV (Sparse Matrix-Vector
// Multiplication) benchmark. Computes y = A*x using CSR format.
package spmv_parboil

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// Matches: spmv_csr(const int *row_ptr, const int *col_idx,
//
//	const float *values, const float *x, float *y, int num_rows)
// KernelArgs defines kernel arguments.
type KernelArgs struct {
	RowPtr              driver.Ptr
	ColIdx              driver.Ptr
	Values              driver.Ptr
	X                   driver.Ptr
	Y                   driver.Ptr
	NumRows             int32
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

// Benchmark defines the SpMV benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	spmvKernel *insts.KernelCodeObject

	NumRows   int
	BlockSize int

	useUnifiedMemory bool

	hRowPtr []int32
	hColIdx []int32
	hValues []float32
	hX      []float32
	hY      []float32
	hRef    []float32

	totalNNZ int

	dRowPtr driver.Ptr
	dColIdx driver.Ptr
	dValues driver.Ptr
	dX      driver.Ptr
	dY      driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new SpMV benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumRows = 65536
	b.BlockSize = 256
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

func (b *Benchmark) loadProgram() {
	b.spmvKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "spmv_csr")
	if b.spmvKernel == nil {
		log.Panic("Failed to load spmv_csr binary")
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
	b.initHostData()
	b.allocGPUMem()

	b.driver.MemCopyH2D(b.context, b.dRowPtr, b.hRowPtr)
	b.driver.MemCopyH2D(b.context, b.dColIdx, b.hColIdx)
	b.driver.MemCopyH2D(b.context, b.dValues, b.hValues)
	b.driver.MemCopyH2D(b.context, b.dX, b.hX)
}

func (b *Benchmark) initHostData() {
	n := b.NumRows

	// Build a tridiagonal CSR matrix:
	// row i has entries at columns max(0,i-1), i, min(n-1,i+1)
	b.hRowPtr = make([]int32, n+1)

	// First pass: count nnz per row
	totalNNZ := 0
	for i := 0; i < n; i++ {
		nnz := 1 // diagonal
		if i > 0 {
			nnz++ // sub-diagonal
		}
		if i < n-1 {
			nnz++ // super-diagonal
		}
		totalNNZ += nnz
	}
	b.totalNNZ = totalNNZ

	b.hColIdx = make([]int32, totalNNZ)
	b.hValues = make([]float32, totalNNZ)

	// Build row_ptr, col_idx, values
	ptr := 0
	for i := 0; i < n; i++ {
		b.hRowPtr[i] = int32(ptr)
		if i > 0 {
			b.hColIdx[ptr] = int32(i - 1)
			b.hValues[ptr] = 0.1
			ptr++
		}
		b.hColIdx[ptr] = int32(i)
		b.hValues[ptr] = 0.8
		ptr++
		if i < n-1 {
			b.hColIdx[ptr] = int32(i + 1)
			b.hValues[ptr] = 0.1
			ptr++
		}
	}
	b.hRowPtr[n] = int32(ptr)

	// Initialize x vector
	b.hX = make([]float32, n)
	for i := 0; i < n; i++ {
		b.hX[i] = float32(i%100) * 0.01
	}

	b.hY = make([]float32, n)
	b.hRef = make([]float32, n)
}

func (b *Benchmark) allocGPUMem() {
	n := b.NumRows
	bytesRowPtr := uint64(n+1) * 4
	bytesColIdx := uint64(b.totalNNZ) * 4
	bytesValues := uint64(b.totalNNZ) * 4
	bytesX := uint64(n) * 4
	bytesY := uint64(n) * 4

	if b.useUnifiedMemory {
		b.dRowPtr = b.driver.AllocateUnifiedMemory(b.context, bytesRowPtr)
		b.dColIdx = b.driver.AllocateUnifiedMemory(b.context, bytesColIdx)
		b.dValues = b.driver.AllocateUnifiedMemory(b.context, bytesValues)
		b.dX = b.driver.AllocateUnifiedMemory(b.context, bytesX)
		b.dY = b.driver.AllocateUnifiedMemory(b.context, bytesY)
	} else {
		b.dRowPtr = b.driver.AllocateMemory(b.context, bytesRowPtr)
		b.dColIdx = b.driver.AllocateMemory(b.context, bytesColIdx)
		b.dValues = b.driver.AllocateMemory(b.context, bytesValues)
		b.dX = b.driver.AllocateMemory(b.context, bytesX)
		b.dY = b.driver.AllocateMemory(b.context, bytesY)
	}
}

func (b *Benchmark) exec() {
	numBlocks := (b.NumRows + b.BlockSize - 1) / b.BlockSize

	globalSize := [3]uint32{
		uint32(numBlocks * b.BlockSize),
		1,
		1,
	}
	localSize := [3]uint16{
		uint16(b.BlockSize),
		1,
		1,
	}

	b.launch(globalSize, localSize)

	b.driver.MemCopyD2H(b.context, b.hY, b.dY)
}

func (b *Benchmark) launch(
	globalSize [3]uint32, localSize [3]uint16,
) {
	args := KernelArgs{
		RowPtr:            b.dRowPtr,
		ColIdx:            b.dColIdx,
		Values:            b.dValues,
		X:                 b.dX,
		Y:                 b.dY,
		NumRows:           int32(b.NumRows),
		HiddenBlockCountX: globalSize[0] / uint32(localSize[0]),
		HiddenBlockCountY: 1,
		HiddenBlockCountZ: 1,
		HiddenGroupSizeX:  localSize[0],
		HiddenGroupSizeY:  1,
		HiddenGroupSizeZ:  1,
		HiddenRemainderX:  uint16(globalSize[0] % uint32(localSize[0])),
		HiddenRemainderY:  0,
		HiddenRemainderZ:  0,
		HiddenGridDims:    1,
	}
	b.driver.LaunchKernel(b.context,
		b.spmvKernel,
		globalSize, localSize,
		&args,
	)
}

// Verify verifies the result against CPU computation.
func (b *Benchmark) Verify() {
	b.cpuSpMV()
	b.checkResults()
}

func (b *Benchmark) cpuSpMV() {
	n := b.NumRows
	for i := 0; i < n; i++ {
		var sum float32
		rowStart := int(b.hRowPtr[i])
		rowEnd := int(b.hRowPtr[i+1])
		for j := rowStart; j < rowEnd; j++ {
			col := int(b.hColIdx[j])
			sum += b.hValues[j] * b.hX[col]
		}
		b.hRef[i] = sum
	}
}

func (b *Benchmark) checkResults() {
	errors := 0
	n := b.NumRows

	for i := 0; i < n; i++ {
		diff := math.Abs(float64(b.hY[i] - b.hRef[i]))
		tol := 1e-3*math.Abs(float64(b.hRef[i])) + 1e-5

		if diff > tol {
			if errors < 10 {
				log.Printf("Mismatch at %d: got %f, expected %f",
					i, b.hY[i], b.hRef[i])
			}
			errors++
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in SpMV verification", errors)
	}

	log.Printf("Passed!")
}
