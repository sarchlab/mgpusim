// Package nw defines the Needleman–Wunsch benchmark
package nw

import (
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/arch"
	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments for GCN3 architecture
type KernelArgs struct {
	Reference          driver.Ptr
	InputItemSets      driver.Ptr
	OutputItemSets     driver.Ptr
	LocalInputItemSets driver.LocalPtr
	LocalReference     driver.LocalPtr
	Cols               int32
	Penalty            int32
	Blk                int32
	BlockSize          int32
	BlockWidth         int32
	WorkSize           int32
	OffsetR            int32
	OffsetC            int32
}

// CDNA3KernelArgs defines kernel arguments for CDNA3 architecture (gfx942)
// No LocalInputItemSets/LocalReference - those are __shared__ inside the kernel.
// KernargSegmentByteSize=56 (no hidden args needed - kernel uses AQL packet for dispatch dims)
type CDNA3KernelArgs struct {
	Reference      driver.Ptr // offset 0
	InputItemSets  driver.Ptr // offset 8
	OutputItemSets driver.Ptr // offset 16
	Cols           int32      // offset 24
	Penalty        int32      // offset 28
	Blk            int32      // offset 32
	BlockSize      int32      // offset 36
	BlockWidth     int32      // offset 40
	WorkSize       int32      // offset 44
	OffsetR        int32      // offset 48
	OffsetC        int32      // offset 52
}

var blosum62 = [][]int32{
	{4, -1, -2, -2, 0, -1, -1, 0, -2, -1, -1, -1, -1, -2, -1, 1, 0, -3, -2, 0, -2, -1, 0, -4},
	{-1, 5, 0, -2, -3, 1, 0, -2, 0, -3, -2, 2, -1, -3, -2, -1, -1, -3, -2, -3, -1, 0, -1, -4},
	{-2, 0, 6, 1, -3, 0, 0, 0, 1, -3, -3, 0, -2, -3, -2, 1, 0, -4, -2, -3, 3, 0, -1, -4},
	{-2, -2, 1, 6, -3, 0, 2, -1, -1, -3, -4, -1, -3, -3, -1, 0, -1, -4, -3, -3, 4, 1, -1, -4},
	{0, -3, -3, -3, 9, -3, -4, -3, -3, -1, -1, -3, -1, -2, -3, -1, -1, -2, -2, -1, -3, -3, -2, -4},
	{-1, 1, 0, 0, -3, 5, 2, -2, 0, -3, -2, 1, 0, -3, -1, 0, -1, -2, -1, -2, 0, 3, -1, -4},
	{-1, 0, 0, 2, -4, 2, 5, -2, 0, -3, -3, 1, -2, -3, -1, 0, -1, -3, -2, -2, 1, 4, -1, -4},
	{0, -2, 0, -1, -3, -2, -2, 6, -2, -4, -4, -2, -3, -3, -2, 0, -2, -2, -3, -3, -1, -2, -1, -4},
	{-2, 0, 1, -1, -3, 0, 0, -2, 8, -3, -3, -1, -2, -1, -2, -1, -2, -2, 2, -3, 0, 0, -1, -4},
	{-1, -3, -3, -3, -1, -3, -3, -4, -3, 4, 2, -3, 1, 0, -3, -2, -1, -3, -1, 3, -3, -3, -1, -4},
	{-1, -2, -3, -4, -1, -2, -3, -4, -3, 2, 4, -2, 2, 0, -3, -2, -1, -2, -1, 1, -4, -3, -1, -4},
	{-1, 2, 0, -1, -3, 1, 1, -2, -1, -3, -2, 5, -1, -3, -1, 0, -1, -3, -2, -2, 0, 1, -1, -4},
	{-1, -1, -2, -3, -1, 0, -2, -3, -2, 1, 2, -1, 5, 0, -2, -1, -1, -1, -1, 1, -3, -1, -4},
	{-2, -3, -3, -3, -2, -3, -3, -3, -1, 0, 0, -3, 0, 6, -4, -2, -2, 1, 3, -1, -3, -3, -1, -4},
	{-1, -2, -2, -1, -3, -1, -1, -2, -2, -3, -3, -1, -2, -4, 7, -1, -1, -4, -3, -2, -2, -1, -2, -4},
	{1, -1, 1, 0, -1, 0, 0, 0, -1, -2, -2, 0, -1, -2, -1, 4, 1, -3, -2, -2, 0, 0, 0, -4},
	{0, -1, 0, -1, -1, -1, -1, -2, -2, -1, -1, -1, -1, -2, -1, 1, 5, -2, -2, 0, -1, -1, 0, -4},
	{-3, -3, -4, -4, -2, -2, -3, -2, -2, -3, -2, -3, -1, 1, -4, -3, -2, 11, 2, -3, -4, -3, -2, -4},
	{-2, -2, -2, -3, -2, -1, -2, -3, 2, -1, -1, -2, -1, 3, -3, -2, -2, 2, 7, -1, -3, -2, -1, -4},
	{0, -3, -3, -3, -1, -2, -2, -3, -3, 3, 1, -2, 1, -1, -2, -2, 0, -3, -1, 4, -3, -2, -1, -4},
	{-2, -1, 3, 4, -3, 0, 1, -1, 0, -3, -4, 0, -3, -3, -2, 0, -1, -4, -3, -3, 4, 1, -1, -4},
	{-1, 0, 0, 1, -3, 3, 4, -2, 0, -3, -3, 1, -1, -3, -1, 0, -1, -3, -2, -2, 1, 4, -1, -4},
	{0, -1, -1, -1, -2, -1, -1, -1, -1, -1, -1, -1, -1, -1, -2, 0, 0, -2, -1, -1, -1, -1, -1, -4},
	{-4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, 1},
}

// Benchmark defines a benchmark
type Benchmark struct {
	driver           *driver.Driver
	context          *driver.Context
	gpuIDs           []int
	useUnifiedMemory bool
	kernel1, kernel2 *insts.KernelCodeObject
	queue            *driver.CommandQueue

	Arch arch.Type

	blockSize         int
	length            int
	penalty           int
	row, col          int
	reference         []int32
	inputItemSets     []int32
	outputItemSets    []int32
	cpuOutputItemSets []int32
	dInputItemSets    driver.Ptr
	dOutputItemSets   driver.Ptr
	dReference        driver.Ptr
}

// NewBenchmark creates a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	b.blockSize = 64
	b.SetLength(256)
	b.penalty = 10

	return b
}

//go:embed kernels.hsaco
var gcn3HSACOBytes []byte

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

func (b *Benchmark) loadProgram() {
	var hsacoBytes []byte
	if b.Arch == arch.CDNA3 {
		hsacoBytes = cdna3HSACOBytes
	} else {
		hsacoBytes = gcn3HSACOBytes
	}

	b.kernel1 = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "nw_kernel1")
	if b.kernel1 == nil {
		log.Panic("Failed to load kernel binary nw_kernel1")
	}

	b.kernel2 = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "nw_kernel2")
	if b.kernel2 == nil {
		log.Panic("Failed to load kernel binary nw_kernel2")
	}
}

// SetLength sets length
func (b *Benchmark) SetLength(length int) {
	b.length = length
	b.row = length + 1
	b.col = length + 1
}

// SetPenalty sets penalty
func (b *Benchmark) SetPenalty(penalty int) {
	b.penalty = penalty
}

// SelectGPU selects gpu
func (b *Benchmark) SelectGPU(gpuIDs []int) {
	if len(gpuIDs) > 1 {
		panic("nw does not support multi-GPU mode")
	}

	b.gpuIDs = gpuIDs
}

// Run runs
func (b *Benchmark) Run() {
	b.loadProgram()
	b.driver.SelectGPU(b.context, b.gpuIDs[0])
	b.initMem()
	b.exec()
}

// SetUnifiedMemory Use Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) initMem() {
	b.initData()
	b.allocateGPUMem()
}

func (b *Benchmark) initData() {
	b.reference = make([]int32, b.row*b.col)
	b.inputItemSets = make([]int32, b.row*b.col)
	b.outputItemSets = make([]int32, b.row*b.col)

	for i := 0; i < b.row; i++ {
		b.inputItemSets[i*b.col] = int32(rand.Int()%10 + 1)
	}

	for i := 0; i < b.col; i++ {
		b.inputItemSets[i] = int32(rand.Int()%10 + 1)
	}

	for i := 0; i < b.col; i++ {
		for j := 0; j < b.row; j++ {
			b.reference[i*b.col+j] =
				blosum62[b.inputItemSets[i*b.col]][b.inputItemSets[j]]
		}
	}

	b.inputItemSets[0] = 0

	for i := 1; i < b.row; i++ {
		b.inputItemSets[i*b.col] = int32(-i * b.penalty)
	}
	for j := 1; j < b.col; j++ {
		b.inputItemSets[j] = int32(-j * b.penalty)
	}
}

func (b *Benchmark) allocateGPUMem() {
	b.dInputItemSets = b.allocate(uint64(b.col * b.row * 4))
	b.dOutputItemSets = b.allocate(uint64(b.col * b.row * 4))
	b.dReference = b.allocate(uint64(b.col * b.row * 4))
	b.driver.Distribute(b.context, b.dInputItemSets, uint64(b.col*b.row*4), b.gpuIDs)
	b.driver.Distribute(b.context, b.dOutputItemSets, uint64(b.col*b.row*4), b.gpuIDs)
	b.driver.Distribute(b.context, b.dReference, uint64(b.col*b.row*4), b.gpuIDs)
}

func (b *Benchmark) allocate(byteSize uint64) driver.Ptr {
	if b.useUnifiedMemory {
		return b.driver.AllocateUnifiedMemory(b.context, byteSize)
	}

	return b.driver.AllocateMemory(b.context, byteSize)
}

func (b *Benchmark) exec() {
	b.copyInputDataToGPU()
	b.runKernel1()
	b.runKernel2()
	b.copyOutputDataFromGPU()
}

func (b *Benchmark) copyInputDataToGPU() {
	b.driver.MemCopyH2D(b.context, b.dInputItemSets, b.inputItemSets)
	b.driver.MemCopyH2D(b.context, b.dReference, b.reference)
}

func (b *Benchmark) copyOutputDataFromGPU() {
	b.driver.MemCopyD2H(b.context, b.outputItemSets, b.dInputItemSets)
}

func (b *Benchmark) runKernel1() {
	workSize := b.col - 1
	offsetR := 0
	offsetC := 0
	blockWidth := workSize / b.blockSize

	for blk := 1; blk <= workSize/b.blockSize; blk++ {
		globalSize := [3]uint32{uint32(b.blockSize * blk), 1, 1}
		localSize := [3]uint16{uint16(b.blockSize), 1, 1}

		if b.Arch == arch.CDNA3 {
			cdna3Args := CDNA3KernelArgs{
				Reference:      b.dReference,
				InputItemSets:  b.dInputItemSets,
				OutputItemSets: b.dOutputItemSets,
				Cols:           int32(b.col),
				Penalty:        int32(b.penalty),
				Blk:            int32(blk),
				BlockSize:      int32(b.blockSize),
				BlockWidth:     int32(blockWidth),
				WorkSize:       int32(workSize),
				OffsetR:        int32(offsetR),
				OffsetC:        int32(offsetC),
			}
			b.driver.LaunchKernel(b.context, b.kernel1, globalSize, localSize, &cdna3Args)
		} else {
			args := KernelArgs{
				Reference:          b.dReference,
				InputItemSets:      b.dInputItemSets,
				OutputItemSets:     b.dOutputItemSets,
				LocalInputItemSets: driver.LocalPtr((b.blockSize + 1) * (b.blockSize + 1) * 4),
				LocalReference:     driver.LocalPtr(b.blockSize * b.blockSize * 4),
				Cols:               int32(b.col),
				Penalty:            int32(b.penalty),
				Blk:                int32(blk),
				BlockSize:          int32(b.blockSize),
				BlockWidth:         int32(blockWidth),
				WorkSize:           int32(workSize),
				OffsetR:            int32(offsetR),
				OffsetC:            int32(offsetC),
			}
			b.driver.LaunchKernel(b.context, b.kernel1, globalSize, localSize, &args)
		}
	}
}

func (b *Benchmark) runKernel2() {
	workSize := b.col - 1
	offsetR := 0
	offsetC := 0
	blockWidth := workSize / b.blockSize

	for blk := 1; blk <= workSize/b.blockSize; blk++ {
		globalSize := [3]uint32{uint32(b.blockSize * blk), 1, 1}
		localSize := [3]uint16{uint16(b.blockSize), 1, 1}

		if b.Arch == arch.CDNA3 {
			cdna3Args := CDNA3KernelArgs{
				Reference:      b.dReference,
				InputItemSets:  b.dInputItemSets,
				OutputItemSets: b.dOutputItemSets,
				Cols:           int32(b.col),
				Penalty:        int32(b.penalty),
				Blk:            int32(blk),
				BlockSize:      int32(b.blockSize),
				BlockWidth:     int32(blockWidth),
				WorkSize:       int32(workSize),
				OffsetR:        int32(offsetR),
				OffsetC:        int32(offsetC),
			}
			b.driver.LaunchKernel(b.context, b.kernel2, globalSize, localSize, &cdna3Args)
		} else {
			args := KernelArgs{
				Reference:          b.dReference,
				InputItemSets:      b.dInputItemSets,
				OutputItemSets:     b.dOutputItemSets,
				LocalInputItemSets: driver.LocalPtr((b.blockSize + 1) * (b.blockSize + 1) * 4),
				LocalReference:     driver.LocalPtr(b.blockSize * b.blockSize * 4),
				Cols:               int32(b.col),
				Penalty:            int32(b.penalty),
				Blk:                int32(blk),
				BlockSize:          int32(b.blockSize),
				BlockWidth:         int32(blockWidth),
				WorkSize:           int32(workSize),
				OffsetR:            int32(offsetR),
				OffsetC:            int32(offsetC),
			}
			b.driver.LaunchKernel(b.context, b.kernel2, globalSize, localSize, &args)
		}
	}
}

// Verify verifies
func (b *Benchmark) Verify() {
	b.cpuNW()

	for i := 0; i < b.row; i++ {
		for j := 0; j < b.col; j++ {
			if b.outputItemSets[i*b.col+j] != b.inputItemSets[i*b.col+j] {
				log.Panicf(
					"mismatch at (%d,%d), expected %d, but got %d\n",
					i, j, b.inputItemSets[i*b.col+j],
					b.outputItemSets[i*b.col+j])
			}
		}
	}
}

func (b *Benchmark) cpuNW() {
	for i := 1; i < b.row; i++ {
		for j := 1; j < b.col; j++ {
			leftPenalty := b.inputItemSets[i*b.col+(j-1)] - int32(b.penalty)
			topPenalty := b.inputItemSets[(i-1)*b.col+j] - int32(b.penalty)
			refValue := b.reference[i*b.col+j]
			diagPenalty := b.inputItemSets[(i-1)*b.col+(j-1)] + refValue

			max := leftPenalty
			if topPenalty > max {
				max = topPenalty
			}
			if diagPenalty > max {
				max = diagPenalty
			}

			b.inputItemSets[i*b.col+j] = max
		}
	}
}
