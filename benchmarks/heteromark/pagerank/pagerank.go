package pagerank

import (
	"fmt"
	"log"
	"math"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

type PageRankKernelArgs struct {
	NumRows   uint32
	Padding   uint32
	RowOffset driver.GPUPtr
	Col       driver.GPUPtr
	Val       driver.GPUPtr
	Vals      driver.LocalPtr
	Padding2  uint32
	X         driver.GPUPtr
	Y         driver.GPUPtr
}

type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue
	kernel  *insts.HsaCo

	NumNodes       uint32
	NumConnections uint32
	MaxIterations  uint32

	hMatrix         csrMatrix
	hPageRank       []float32
	verPageRank     []float32
	verPageRankTemp []float32

	dPageRank      driver.GPUPtr
	dPageRankTemp  driver.GPUPtr
	dRowOffsets    driver.GPUPtr
	dColumnNumbers driver.GPUPtr
	dValues        driver.GPUPtr
	dLocalValues   driver.LocalPtr
}

func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.loadProgram()
	return b
}

func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

func (b *Benchmark) loadProgram() {
	hsacoBytes := _escFSMustByte(false, "/kernels.hsaco")

	b.kernel = kernels.LoadProgramFromMemory(hsacoBytes, "PageRankUpdateGpu")
	if b.kernel == nil {
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

	initData := float32(1.0) / float32(b.NumNodes)
	b.hPageRank = make([]float32, b.NumNodes)
	b.verPageRank = make([]float32, b.NumNodes)
	b.verPageRankTemp = make([]float32, b.NumNodes)
	b.hMatrix = makeMatrixGenerator(b.NumNodes, b.NumConnections).
		generateMatrix()

	for i := uint32(0); i < b.NumNodes; i++ {
		b.hPageRank[i] = initData
		b.verPageRank[i] = initData
	}

	b.dPageRank = b.driver.AllocateMemoryWithAlignment(
		b.context, uint64(b.NumNodes*4), 4096)
	b.dPageRankTemp = b.driver.AllocateMemoryWithAlignment(
		b.context, uint64(b.NumNodes*4), 4096)
	b.dRowOffsets = b.driver.AllocateMemoryWithAlignment(
		b.context, uint64(b.NumNodes*4), 4096)
	b.dColumnNumbers = b.driver.AllocateMemoryWithAlignment(
		b.context, uint64(b.NumNodes*4), 4096)
	b.dValues = b.driver.AllocateMemoryWithAlignment(
		b.context, uint64(b.NumNodes*4), 4096)

}

func printMatrix(matrix [][]float32, n uint32) {
	for i := uint32(0); i < n; i++ {
		for j := uint32(0); j < n; j++ {
			fmt.Printf("%f ", matrix[i][j])
		}
		fmt.Printf("\n")
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dPageRank, b.hPageRank)
	b.driver.MemCopyH2D(b.context, b.dRowOffsets,
		b.hMatrix.rowOffsets)
	b.driver.MemCopyH2D(b.context, b.dColumnNumbers,
		b.hMatrix.columnNumbers)
	b.driver.MemCopyH2D(b.context, b.dValues,
		b.hMatrix.values)

	b.dLocalValues = driver.LocalPtr(256)

	localWorkSize := 64
	i := uint32(0)

	for i = 0; i < b.MaxIterations; i++ {
		var kernArg PageRankKernelArgs
		if i%2 == 0 {
			kernArg = PageRankKernelArgs{
				NumRows:   b.NumNodes,
				RowOffset: b.dRowOffsets,
				Col:       b.dColumnNumbers,
				Val:       b.dValues,
				Vals:      b.dLocalValues,
				X:         b.dPageRank,
				Y:         b.dPageRankTemp,
			}
		} else {
			kernArg = PageRankKernelArgs{
				NumRows:   b.NumNodes,
				RowOffset: b.dRowOffsets,
				Col:       b.dColumnNumbers,
				Val:       b.dValues,
				Vals:      b.dLocalValues,
				X:         b.dPageRankTemp,
				Y:         b.dPageRank,
			}
		}

		b.driver.LaunchKernel(
			b.context,
			b.kernel,
			[3]uint32{uint32(b.NumNodes) * 64, 1, 1},
			[3]uint16{uint16(localWorkSize), 1, 1},
			&kernArg,
		)
	}

	if i%2 != 0 {
		b.driver.MemCopyD2H(b.context, b.hPageRank, b.dPageRankTemp)
	} else {
		b.driver.MemCopyD2H(b.context, b.hPageRank, b.dPageRank)
	}
}

func (b *Benchmark) Verify() {
	var i uint32
	m := b.hMatrix
	for i = 0; i < b.MaxIterations; i++ {
		for i := uint32(0); i < b.NumNodes; i++ {
			newValue := float32(0)
			for j := uint32(m.rowOffsets[i]); j < m.rowOffsets[i+1]; j++ {
				newValue += float32(m.values[j]) *
					b.verPageRank[m.columnNumbers[j]]
			}
			b.verPageRankTemp[i] = newValue
		}
		copy(b.verPageRank, b.verPageRankTemp)
	}

	for i := uint32(0); i < b.NumNodes; i++ {
		if math.Abs(float64(b.verPageRank[i]-b.hPageRank[i])) > 1e-5 {
			log.Panicf("Mismatch at %d, expected %f, but get %f\n",
				i, b.verPageRank[i], b.hPageRank[i])
		}
	}

	log.Printf("Passed!\n")
}
