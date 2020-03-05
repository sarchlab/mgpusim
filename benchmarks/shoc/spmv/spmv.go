// Package spmv include the benchmark of sparse matrix-vector matiplication.
package spmv

import (
	"log"
	"math/rand"

	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/kernels"
)

type SpmvKernelArgs struct {
	Val           driver.GPUPtr
	Vec           driver.GPUPtr
	Cols          driver.GPUPtr
	RowDelimiters driver.GPUPtr
	Dim           int32
	// VecWidth            int32
	// PartialSums         driver.LocalPtr
	Padding             int32
	Out                 driver.GPUPtr
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

type Benchmark struct {
	driver           *driver.Driver
	context          *driver.Context
	gpus             []int
	queues           []*driver.CommandQueue
	useUnifiedMemory bool
	spmvKernel       *insts.HsaCo

	Dim          int32
	dValData     driver.GPUPtr
	dVecData     driver.GPUPtr
	dColsData    driver.GPUPtr
	dRowDData    driver.GPUPtr
	dOutData     driver.GPUPtr
	nItems       int32
	val          []float32
	cols         []int32
	rowDelimiter []int32
	vec          []float32
	out          []float32
	maxval       float32
}

func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.loadProgram()
	b.maxval = 10
	return b
}

func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// Use Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) loadProgram() {
	hsacoBytes := _escFSMustByte(false, "/spmv.hsaco")

	b.spmvKernel = kernels.LoadProgramFromMemory(hsacoBytes, "spmv_csr_scalar_kernel")
	if b.spmvKernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

func (b *Benchmark) Run() {
	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues, b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	b.nItems = ((b.Dim * b.Dim) / 100) // 1% of entries will be non-zero
	b.val = make([]float32, b.nItems)
	b.vec = make([]float32, b.Dim)
	b.cols = make([]int32, b.nItems)
	b.out = make([]float32, b.Dim)
	b.rowDelimiter = make([]int32, b.Dim+1)
	b.fill()
	b.initRandomMatrix()

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
	b.driver.MemCopyH2D(b.context, b.dValData, b.val)
	b.driver.MemCopyH2D(b.context, b.dVecData, b.vec)
	b.driver.MemCopyH2D(b.context, b.dColsData, b.cols)
	b.driver.MemCopyH2D(b.context, b.dRowDData, b.rowDelimiter)
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

	args := SpmvKernelArgs{
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
		for j := b.rowDelimiter[i]; j < b.rowDelimiter[i+1]; j++ {
			col := b.cols[j]
			t += b.val[j] * b.vec[col]
		}
		cpuOutput[i] = t
	}
	return cpuOutput
}

func (b *Benchmark) fill() {
	// Seed random number generator
	rand.Seed(8675309)

	for j := int32(0); j < b.nItems; j++ {
		b.val[j] = (rand.Float32() * b.maxval)
	}

	for j := int32(0); j < b.Dim; j++ {
		b.vec[j] = (rand.Float32() * b.maxval)
	}
}

func (b *Benchmark) initRandomMatrix() {
	nnzAssigned := int32(0)
	// Figure out the probability that a nonzero should be assigned to a given
	// spot in the matrix
	prob := float64(b.nItems) / (float64(b.Dim) * float64(b.Dim))

	// Randomly decide whether entry i,j gets a value, but ensure b.nItems values
	// are assigned
	fillRemaining := false
	for i := int32(0); i < b.Dim; i++ {
		b.rowDelimiter[i] = nnzAssigned
		for j := int32(0); j < b.Dim; j++ {
			numEntriesLeft := (b.Dim * b.Dim) - ((i * b.Dim) + j)
			needToAssign := b.nItems - nnzAssigned
			if numEntriesLeft <= needToAssign {
				fillRemaining = true
			}
			if (nnzAssigned < b.nItems && rand.Float64() <= prob) || fillRemaining {
				// Assign (i,j) a value
				b.cols[nnzAssigned] = j
				nnzAssigned++
			}
		}
	}
	// Observe the convention to put the number of non zeroes at the end of the
	// row delimiters array
	b.rowDelimiter[b.Dim] = b.nItems
}
