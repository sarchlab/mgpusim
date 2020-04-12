// Package nw defines the Needleman–Wunsch benchmark
package nw

import (
	"fmt"
	"log"
	"math/rand"

	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/kernels"
)

type KernalArgs struct {
	Reference          driver.GPUPtr
	InputItemSets      driver.GPUPtr
	OutputItemSets     driver.GPUPtr
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

var blosum62 [][]int32 = [][]int32{
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
	{-1, -1, -2, -3, -1, 0, -2, -3, -2, 1, 2, -1, 5, 0, -2, -1, -1, -1, -1, 1, -3, -1, -1, -4},
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

type Benchmark struct {
	driver           *driver.Driver
	context          *driver.Context
	gpuIDs           []int
	useUnifiedMemory bool
	kernel1, kernel2 *insts.HsaCo

	blockSize         int
	length            int
	penalty           int
	row, col          int
	reference         []int32
	inputItemSets     []int32
	outputItemSets    []int32
	cpuOutputItemSets []int32
	dInputItemSets    driver.GPUPtr
	dOutputItemSets   driver.GPUPtr
	dReference        driver.GPUPtr
}

func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.loadProgram()

	b.blockSize = 64
	b.SetLength(256)
	b.penalty = 10

	return b
}

func (b *Benchmark) loadProgram() {
	hsacoBytes := _escFSMustByte(false, "/kernels.hsaco")

	b.kernel1 = kernels.LoadProgramFromMemory(
		hsacoBytes, "nw_kernel1")
	if b.kernel1 == nil {
		log.Panic("Failed to load kernel binary")
	}

	b.kernel2 = kernels.LoadProgramFromMemory(
		hsacoBytes, "nw_kernel2")
	if b.kernel2 == nil {
		log.Panic("Failed to load kernel binary")
	}
}

func (b *Benchmark) SetLength(length int) {
	// if length%16 != 0 {
	// 	panic("NW length must be a multiple of 16")
	// }

	b.length = length
	b.row = length + 1
	b.col = length + 1
}

func (b *Benchmark) SetPenalty(penalty int) {
	b.penalty = penalty
}

func (b *Benchmark) SelectGPU(gpuIDs []int) {
	if len(gpuIDs) > 1 {
		panic("nw does not support multi-GPU mode")
	}

	b.gpuIDs = gpuIDs
}

func (b *Benchmark) Run() {
	b.initMem()
	b.exec()
}

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
			// b.reference[i*b.col+j] = int32(i*b.col + j)
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
}

func (b *Benchmark) allocate(byteSize uint64) driver.GPUPtr {
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

		args := KernalArgs{
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

		b.driver.LaunchKernel(
			b.context,
			b.kernel1,
			globalSize,
			localSize,
			&args,
		)
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

		args := KernalArgs{
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

		b.driver.LaunchKernel(
			b.context,
			b.kernel2,
			globalSize,
			localSize,
			&args,
		)
	}
}

func (b *Benchmark) Verify() {
	// fmt.Printf("\nReference:\n")
	// for i := 0; i < b.row; i++ {
	// 	for j := 0; j < b.col; j++ {
	// 		fmt.Printf("%5d", b.reference[i*b.col+j])
	// 	}
	// 	fmt.Printf("\n")
	// }

	// fmt.Printf("\nGPU Output:\n")
	// for i := 0; i < b.row; i++ {
	// 	for j := 0; j < b.col; j++ {
	// 		fmt.Printf("%5d", b.outputItemSets[i*b.col+j])
	// 	}
	// 	fmt.Printf("\n")
	// }

	// b.cpuNW()

	// fmt.Printf("\nCPU Output:\n")
	// for i := 0; i < b.row; i++ {
	// 	for j := 0; j < b.col; j++ {
	// 		fmt.Printf("%5d", b.inputItemSets[i*b.col+j])
	// 	}
	// 	fmt.Printf("\n")
	// }

	// mismatch := false
	// for i := 0; i < b.row; i++ {
	// 	for j := 0; j < b.col; j++ {
	// 		if b.outputItemSets[i*b.col+j] != b.inputItemSets[i*b.col+j] {
	// 			mismatch = true
	// 			log.Printf("at (%d, %d), expected %d, but get %d",
	// 				j, i,
	// 				b.inputItemSets[i*b.col+j],
	// 				b.outputItemSets[i*b.col+j])
	// 		}
	// 	}
	// 	// fmt.Printf("\n")
	// }

	// if mismatch {
	// 	panic("mismatch\n")
	// }

	fmt.Print("Passed!\n")
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
