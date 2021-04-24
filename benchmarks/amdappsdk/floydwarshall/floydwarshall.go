// Package floydwarshall implements the Floyd-Warshall benchmark from AMDAPPSDK.
package floydwarshall

import (
	"fmt"
	"log"
	"math/rand"

	"gitlab.com/akita/mgpusim/v2/driver"
	"gitlab.com/akita/mgpusim/v2/insts"
	"gitlab.com/akita/mgpusim/v2/kernels"
)

// KernelArgs defines kernel arguments
type KernelArgs struct {
	OutputPathDistanceMatrix driver.Ptr
	OutputPathMatrix         driver.Ptr

	NumNodes uint32
	Pass     uint32
}

// Benchmark defines a benchmark
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue
	kernel  *insts.HsaCo

	NumNodes                  uint32
	NumIterations             uint32
	hOutputPathMatrix         []uint32
	hOutputPathDistanceMatrix []uint32
	dOutputPathMatrix         driver.Ptr
	dOutputPathDistanceMatrix driver.Ptr

	hVerificationPathMatrix         []uint32
	hVerificationPathDistanceMatrix []uint32

	useUnifiedMemory bool
}

// NewBenchmark creates a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.loadProgram()
	return b
}

// SelectGPU selects gpu
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory Use Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) loadProgram() {
	hsacoBytes := _escFSMustByte(false, "/kernels.hsaco")

	b.kernel = kernels.LoadProgramFromMemory(hsacoBytes, "floydWarshallPass")
	if b.kernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// Run runs the benchmark
func (b *Benchmark) Run() {
	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues, b.driver.CreateCommandQueue(b.context))
	}

	if b.NumIterations == 0 || b.NumIterations > b.NumNodes {
		b.NumIterations = b.NumNodes
	}

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	rand.Seed(1)

	numNodes := b.NumNodes
	b.hOutputPathMatrix = make([]uint32, numNodes*numNodes)
	b.hOutputPathDistanceMatrix = make([]uint32, numNodes*numNodes)

	for i := uint32(0); i < numNodes; i++ {
		for j := uint32(0); j < i; j++ {
			temp := uint32(rand.Int31n(10))
			b.hOutputPathDistanceMatrix[i*numNodes+j] = temp
			b.hOutputPathDistanceMatrix[j*numNodes+i] = temp
		}
	}

	for i := uint32(0); i < numNodes; i++ {
		iXWidth := i * numNodes
		b.hOutputPathDistanceMatrix[iXWidth+i] = 0
	}

	for i := uint32(0); i < numNodes; i++ {
		for j := uint32(0); j < i; j++ {
			b.hOutputPathMatrix[i*numNodes+j] = i
			b.hOutputPathMatrix[j*numNodes+i] = j
		}
		b.hOutputPathMatrix[i*numNodes+i] = i
	}

	b.hVerificationPathMatrix = make([]uint32, numNodes*numNodes)
	b.hVerificationPathDistanceMatrix = make([]uint32, numNodes*numNodes)

	copy(b.hVerificationPathDistanceMatrix, b.hOutputPathDistanceMatrix)
	copy(b.hVerificationPathMatrix, b.hOutputPathMatrix)

	if b.useUnifiedMemory {
		b.dOutputPathMatrix = b.driver.AllocateUnifiedMemory(
			b.context,
			uint64(numNodes*numNodes*4))
		b.dOutputPathDistanceMatrix = b.driver.AllocateUnifiedMemory(
			b.context,
			uint64(numNodes*numNodes*4))
	} else {
		b.dOutputPathMatrix = b.driver.AllocateMemory(
			b.context,
			uint64(numNodes*numNodes*4))
		b.dOutputPathDistanceMatrix = b.driver.AllocateMemory(
			b.context,
			uint64(numNodes*numNodes*4))
	}

	b.driver.MemCopyH2D(b.context, b.dOutputPathMatrix, b.hOutputPathMatrix)
	b.driver.MemCopyH2D(b.context, b.dOutputPathDistanceMatrix, b.hOutputPathDistanceMatrix)
}

func printMatrix(matrix []uint32, n uint32) {
	for i := uint32(0); i < n; i++ {
		for j := uint32(0); j < n; j++ {
			fmt.Printf("%d ", matrix[i*n+j])
		}
		fmt.Printf("\n")
	}
}

func (b *Benchmark) exec() {
	numNodes := b.NumNodes
	blockSize := uint32(8)

	if numNodes%blockSize != 0 {
		numNodes = (numNodes/blockSize + 1) * blockSize
	}

	for k := uint32(0); k < b.NumIterations; k++ {
		pass := k

		kernArg := KernelArgs{
			b.dOutputPathDistanceMatrix,
			b.dOutputPathMatrix,
			numNodes,
			pass,
		}

		b.driver.LaunchKernel(
			b.context,
			b.kernel,
			[3]uint32{numNodes, numNodes, 1},
			[3]uint16{uint16(blockSize), uint16(blockSize), 1},
			&kernArg,
		)
	}

	b.driver.MemCopyD2H(b.context, b.hOutputPathMatrix, b.dOutputPathMatrix)
	b.driver.MemCopyD2H(b.context,
		b.hOutputPathDistanceMatrix, b.dOutputPathDistanceMatrix)
}

// Verify verifies
func (b *Benchmark) Verify() {
	numNodes := b.NumNodes
	var distanceYtoX, distanceYtoK, distanceKtoX, indirectDistance uint32
	width := numNodes
	var yXwidth uint32

	for k := uint32(0); k < b.NumIterations; k++ {
		for y := uint32(0); y < numNodes; y++ {
			yXwidth = y * numNodes
			for x := uint32(0); x < numNodes; x++ {
				distanceYtoX = b.hVerificationPathDistanceMatrix[yXwidth+x]
				distanceYtoK = b.hVerificationPathDistanceMatrix[yXwidth+k]
				distanceKtoX = b.hVerificationPathDistanceMatrix[k*width+x]

				indirectDistance = distanceYtoK + distanceKtoX

				if indirectDistance < distanceYtoX {
					b.hVerificationPathDistanceMatrix[yXwidth+x] = indirectDistance
					b.hVerificationPathMatrix[yXwidth+x] = k
				}
			}
		}
	}

	n := numNodes
	for i := uint32(0); i < n; i++ {
		for j := uint32(0); j < n; j++ {
			if b.hOutputPathMatrix[i*n+j] != b.hVerificationPathMatrix[i*n+j] {
				panic(fmt.Sprintf("Mismatch at row %d col %d, expected %d got %d", i, j,
					b.hVerificationPathMatrix[i*n+j],
					b.hOutputPathMatrix[i*n+j]))
			}
			if b.hOutputPathDistanceMatrix[i*n+j] != b.hVerificationPathDistanceMatrix[i*n+j] {
				panic(fmt.Sprintf("Mismatch at row %d col %d, expected %d got %d", i, j,
					b.hVerificationPathDistanceMatrix[i*n+j],
					b.hOutputPathDistanceMatrix[i*n+j]))
			}
		}
	}

	log.Printf("Passed!\n")
}
