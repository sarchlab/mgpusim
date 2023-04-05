// Package spmv include the benchmark of sparse matrix-vector matiplication.
package spmv

import (
	"fmt"
	"log"
	"math/rand"
	"os"

	// embed hsaco files
	_ "embed"

	"gitlab.com/akita/mgpusim/v3/benchmarks/matrix/csr"
	"gitlab.com/akita/mgpusim/v3/driver"
	"gitlab.com/akita/mgpusim/v3/insts"
	"gitlab.com/akita/mgpusim/v3/kernels"
)

// KernelArgs sets up kernel arguments
type KernelArgs struct {
	Val           driver.Ptr
	Vec           driver.Ptr
	Cols          driver.Ptr
	RowDelimiters driver.Ptr
	Dim           int32
	// VecWidth            int32
	// PartialSums         driver.LocalPtr
	Padding             int32
	Out                 driver.Ptr
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

//Benchmark set up test parameters
type Benchmark struct {
	driver           *driver.Driver
	context          *driver.Context
	gpus             []int
	queues           []*driver.CommandQueue
	useUnifiedMemory bool
	spmvKernel       *insts.HsaCo

	Dim       int32
	Sparsity  float64
	dValData  driver.Ptr
	dVecData  driver.Ptr
	dColsData driver.Ptr
	dRowDData driver.Ptr
	dOutData  driver.Ptr
	nItems    int32
	vec       []float32
	out       []float32
	maxval    float32
	matrix    csr.Matrix
}

// NewBenchmark creates a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.loadProgram()
	b.maxval = 10
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

//go:embed spmv.hsaco
var hsacoBytes []byte

func (b *Benchmark) loadProgram() {
	b.spmvKernel = kernels.LoadProgramFromMemory(
		hsacoBytes, "spmv_csr_scalar_kernel")
	if b.spmvKernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// Run runs the benchmark
func (b *Benchmark) Run() {
	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues, b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	b.nItems = int32(float64(b.Dim) * float64(b.Dim) * b.Sparsity)
	fmt.Fprintf(os.Stderr, "Number of non-zero elements %d\n", b.nItems)

	b.matrix = csr.
		MakeMatrixGenerator(uint32(b.Dim), uint32(b.nItems)).
		GenerateMatrix()
	b.vec = make([]float32, b.Dim)
	b.out = make([]float32, b.Dim)

	for j := int32(0); j < b.Dim; j++ {
		b.vec[j] = (rand.Float32() * b.maxval)
	}

	if b.useUnifiedMemory {
		b.dValData = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.nItems*4))
		b.dVecData = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.Dim*4))
		b.dColsData = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.nItems*4))
		b.dRowDData = b.driver.AllocateUnifiedMemory(b.context,
			uint64((b.Dim+1)*4))
		b.dOutData = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.Dim*4))
	} else {
		b.dValData = b.driver.AllocateMemory(b.context,
			uint64(b.nItems*4))
		b.dVecData = b.driver.AllocateMemory(b.context,
			uint64(b.Dim*4))
		b.dColsData = b.driver.AllocateMemory(b.context,
			uint64(b.nItems*4))
		b.dRowDData = b.driver.AllocateMemory(b.context,
			uint64((b.Dim+1)*4))
		b.dOutData = b.driver.AllocateMemory(b.context,
			uint64(b.Dim*4))
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dValData, b.matrix.Values)
	b.driver.MemCopyH2D(b.context, b.dVecData, b.vec)
	b.driver.MemCopyH2D(b.context, b.dColsData, b.matrix.ColumnNumbers)
	b.driver.MemCopyH2D(b.context, b.dRowDData, b.matrix.RowOffsets)
	b.driver.MemCopyH2D(b.context, b.dOutData, b.out)

	//TODO: Review vecWidth, blockSize, and maxwidth
	// vecWidth := int32(64)    // PreferredWorkGroupSizeMultiple
	// maxLocal := int32(64)    // MaxWorkGroupSize
	blockSize := int32(128) // BLOCK_SIZE

	// localWorkSize := vecWidth
	// for ok := true; ok; ok = ((localWorkSize+vecWidth <= maxLocal) && localWorkSize+vecWidth <= blockSize) {
	//	localWorkSize += vecWidth
	// }

	// vectorGlobalWSize := b.Dim * vecWidth // 1 warp per row

	args := KernelArgs{
		Val:           b.dValData,
		Vec:           b.dVecData,
		Cols:          b.dColsData,
		RowDelimiters: b.dRowDData,
		Dim:           b.Dim,
		//VecWidth:            vecWidth,
		//PartialSums:         driver.LocalPtr(blockSize*4), //hardcoded value in spmv.cl
		Padding:             0,
		Out:                 b.dOutData,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
	}

	globalSize := [3]uint32{uint32(b.Dim), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}
	//globalSize := [3]uint32{uint32(vectorGlobalWSize), 1, 1}
	//localSize := [3]uint16{uint16(localWorkSize), 1, 1}

	b.driver.LaunchKernel(b.context,
		b.spmvKernel,
		globalSize, localSize,
		&args,
	)

	b.driver.MemCopyD2H(b.context, b.out, b.dOutData)
}

// Verify verifies results
func (b *Benchmark) Verify() {
	cpuOutput := b.spmvCPU()

	mismatch := false
	for i := int32(0); i < b.Dim; i++ {
		if b.out[i] != cpuOutput[i] {
			mismatch = true
			log.Printf("not match at (%d), expected %f to equal %f\n",
				i,
				b.out[i], cpuOutput[i])
		}
	}

	if mismatch {
		panic("Mismatch!\n")
	}

	log.Printf("Passed!\n")
}

func (b *Benchmark) spmvCPU() []float32 {
	cpuOutput := make([]float32, b.Dim)
	for i := int32(0); i < b.Dim; i++ {
		t := float32(0)
		for j := b.matrix.RowOffsets[i]; j < b.matrix.RowOffsets[i+1]; j++ {
			col := b.matrix.ColumnNumbers[j]
			t += b.matrix.Values[j] * b.vec[col]
		}
		cpuOutput[i] = t
	}
	return cpuOutput
}
