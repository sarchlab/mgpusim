package floydwarshall

import (
	"fmt"
	"log"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"

	"math/rand"
)

type FloydWarshallKernelArgs struct {
	OutputPathDistanceMatrix driver.GPUPtr
	OutputPathMatrix         driver.GPUPtr

	NumNodes uint32
	Pass     uint32
}

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
	dOutputPathMatrix         driver.GPUPtr
	dOutputPathDistanceMatrix driver.GPUPtr

	hVerificationPathMatrix         []uint32
	hVerificationPathDistanceMatrix []uint32
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

	b.kernel = kernels.LoadProgramFromMemory(hsacoBytes, "floydWarshallPass")
	if b.kernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

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
			b.hOutputPathMatrix[i*numNodes+j] = uint32(i)
			b.hOutputPathMatrix[j*numNodes+i] = uint32(j)
		}
		b.hOutputPathMatrix[i*numNodes+i] = uint32(i)
	}

	b.hVerificationPathMatrix = make([]uint32, numNodes*numNodes)
	b.hVerificationPathDistanceMatrix = make([]uint32, numNodes*numNodes)

	copy(b.hVerificationPathDistanceMatrix, b.hOutputPathDistanceMatrix)
	copy(b.hVerificationPathMatrix, b.hOutputPathMatrix)

	b.dOutputPathMatrix = b.driver.AllocateMemoryWithAlignment(b.context, uint64(numNodes*numNodes*4), 4096)
	b.dOutputPathDistanceMatrix = b.driver.AllocateMemoryWithAlignment(b.context, uint64(numNodes*numNodes*4), 4096)

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
	numNodes := uint32(b.NumNodes)
	blockSize := uint32(8)

	if numNodes%blockSize != 0 {
		numNodes = (numNodes/blockSize + 1) * blockSize
	}

	for k := uint32(0); k < b.NumIterations; k++ {
		pass := k

		kernArg := FloydWarshallKernelArgs{
			b.dOutputPathDistanceMatrix,
			b.dOutputPathMatrix,
			uint32(numNodes),
			uint32(pass),
		}

		b.driver.LaunchKernel(
			b.context,
			b.kernel,
			[3]uint32{uint32(numNodes), uint32(numNodes), 1},
			[3]uint16{uint16(blockSize), uint16(blockSize), 1},
			&kernArg,
		)
	}

	b.driver.MemCopyD2H(b.context, b.hOutputPathMatrix, b.dOutputPathMatrix)
	b.driver.MemCopyD2H(b.context, b.hOutputPathDistanceMatrix, b.dOutputPathDistanceMatrix)

}

func (b *Benchmark) Verify() {

	numNodes := b.NumNodes
	var distanceYtoX, distanceYtoK, distanceKtoX, indirectDistance uint32
	width := numNodes
	var yXwidth uint32

	for k := uint32(0); k < b.NumIterations; k++ {
		for y := uint32(0); y < numNodes; y++ {
			yXwidth = uint32(y * numNodes)
			for x := uint32(0); x < numNodes; x++ {
				distanceYtoX = b.hVerificationPathDistanceMatrix[yXwidth+uint32(x)]
				distanceYtoK = b.hVerificationPathDistanceMatrix[yXwidth+uint32(k)]
				distanceKtoX = b.hVerificationPathDistanceMatrix[k*width+x]

				indirectDistance = distanceYtoK + distanceKtoX

				if indirectDistance < distanceYtoX {
					b.hVerificationPathDistanceMatrix[yXwidth+uint32(x)] = indirectDistance
					b.hVerificationPathMatrix[yXwidth+uint32(x)] = uint32(k)
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
