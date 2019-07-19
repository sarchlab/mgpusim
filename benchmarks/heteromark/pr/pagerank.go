package pagerank

import (
	"fmt"
	"log"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"

	"math/rand"
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

	hPageRank       []float32
	hRowOffsets     []uint32
	hColumnNumbers  []uint32
	hValues         []uint32
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
	hsacoBytes := FSMustByte(false, "/kernels.hsaco")

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
	b.Verify()
}

func (b *Benchmark) initMem() {

	b.InitializeMatrix()

	initData := float32(1.0) / float32(b.NumNodes)
	b.hPageRank = make([]float32, b.NumNodes)
	b.verPageRank = make([]float32, b.NumNodes)
	b.verPageRankTemp = make([]float32, b.NumNodes)

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

	b.driver.MemCopyH2D(b.context, b.dPageRank, b.hPageRank)
	b.driver.MemCopyH2D(b.context, b.dPageRankTemp, b.hPageRank)
}

func (b *Benchmark) InitializeMatrix() {

	rand.Seed(123)

	m1 := make([][]uint32, b.NumNodes)
	for i := range m1 {
		m1[i] = make([]uint32, b.NumNodes)
	}

	for i := uint32(0); i < b.NumConnections; i++ {
		row := rand.Uint32() % b.NumNodes
		col := rand.Uint32() % b.NumNodes
		if m1[row][col] != 0 {
			i--
			continue
		}
		m1[row][col] = rand.Uint32()%b.NumNodes + 1
	}

	PrintMatrix(m1, b.NumNodes)

	b.hRowOffsets = make([]uint32, 0)
	b.hColumnNumbers = make([]uint32, 0)
	b.hValues = make([]uint32, 0)

	var offsetCount uint32
	offsetCount = 0
	b.hRowOffsets = append(b.hRowOffsets, offsetCount)

	for i := uint32(0); i < b.NumNodes; i++ {
		for j := uint32(0); j < b.NumNodes; j++ {
			if m1[i][j] != 0 {
				offsetCount++
				b.hColumnNumbers = append(b.hColumnNumbers, j)
				b.hValues = append(b.hValues, m1[i][j])
			}
		}
		b.hRowOffsets = append(b.hRowOffsets, offsetCount)
	}
	//b.hRowOffsets = append(b.hRowOffsets, b.hRowOffsets[len(b.hRowOffsets)-1]+1)

}

func PrintMatrix(matrix [][]uint32, n uint32) {
	for i := uint32(0); i < n; i++ {
		for j := uint32(0); j < n; j++ {
			fmt.Printf("%d ", matrix[i][j])
		}
		fmt.Printf("\n")
	}
}

func (b *Benchmark) exec() {

	b.dLocalValues = driver.LocalPtr(256)

	localWorkSize := 64
	i := uint32(0)

	for _, queue := range b.queues {

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

			b.driver.EnqueueLaunchKernel(
				queue,
				b.kernel,
				[3]uint32{uint32(b.NumNodes), uint32(64), 1},
				[3]uint16{uint16(localWorkSize), 1, 1},
				&kernArg,
			)
		}
	}

	for _, q := range b.queues {
		b.driver.DrainCommandQueue(q)
	}

	if i%2 != 0 {
		b.driver.MemCopyD2H(b.context, b.hPageRank, b.dPageRank)
	} else {
		b.driver.MemCopyD2H(b.context, b.hPageRank, b.dPageRankTemp)
	}

	/*
		for i := 0; i < len(b.hPageRank); i++ {
			fmt.Printf("%d: %f\n", i, b.hPageRank[i])
		}
	*/
}

func (b *Benchmark) Verify() {

	fmt.Printf("\nRow Offsets:\n")
	for i := 0; i < len(b.hRowOffsets); i++ {
		fmt.Printf("%d ", b.hRowOffsets[i])
	}
	fmt.Printf("\nColumn Numbers:\n")
	for i := 0; i < len(b.hColumnNumbers); i++ {
		fmt.Printf("%d ", b.hColumnNumbers[i])
	}
	fmt.Printf("\nValues:\n")
	for i := 0; i < len(b.hValues); i++ {
		fmt.Printf("%d ", b.hValues[i])
	}
	fmt.Printf("\n\n")

	var i uint32
	for i = 0; i < b.MaxIterations; i++ {
		fmt.Printf("Iteration %d:\n", i)
		if i%2 == 0 {
			for i := uint32(0); i < b.NumNodes; i++ {
				newValue := float32(0)
				for j := uint32(b.hRowOffsets[i]); j < b.hRowOffsets[i+1]; j++ {
					newValue += float32(b.hValues[j]) * b.verPageRank[b.hColumnNumbers[j]]
				}
				b.verPageRankTemp[i] = newValue
			}
			for i := 0; i < len(b.verPageRank); i++ {
				fmt.Printf("%d: %f\n", i, b.verPageRankTemp[i])
			}
			fmt.Printf("\n")
		} else {
			for i := uint32(0); i < b.NumNodes; i++ {
				newValue := float32(0)
				for j := uint32(b.hRowOffsets[i]); j < b.hRowOffsets[i+1]; j++ {
					newValue += float32(b.hValues[j]) * b.verPageRankTemp[b.hColumnNumbers[j]]
				}
				b.verPageRank[i] = newValue
			}
			for i := 0; i < len(b.verPageRank); i++ {
				fmt.Printf("%d: %f\n", i, b.verPageRank[i])
			}
			fmt.Printf("\n")
		}
	}

	if i%2 == 0 {
		copy(b.verPageRank, b.verPageRankTemp)
	}

	/*
		for i := 0; i < len(b.verPageRank); i++ {
			fmt.Printf("%d: %f\n", i, b.verPageRank[i])
		}
		for i := 0; i < len(b.verPageRankTemp); i++ {
			fmt.Printf("%d: %f\n", i, b.verPageRankTemp[i])
		}
	*/

}
