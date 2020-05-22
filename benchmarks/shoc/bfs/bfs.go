// Package bfs implements the bfs benchmark from the SHOC suite.
package bfs

import (
	"log"
	"math"

	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/kernels"
)

// KernelArg represents the arguments to pass to the kernel
type KernelArg struct {
	Levels       driver.GPUPtr
	EdgeArray    driver.GPUPtr
	EdgeArrayAux driver.GPUPtr
	WSize        int32
	ChunkSize    int32
	NumNodes     uint32
	Curr         int32
	Flag         driver.GPUPtr
}

// Benchmark is the BFS benchmark
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue
	kernel  *insts.HsaCo

	NumNode       int
	Degree        int
	MaxDepth      int
	graph         graph
	sourceNode    int
	hFrontier     []uint32
	hCost         []uint32
	cpuCost       []uint32
	dFrontier     driver.GPUPtr
	dEdgeArray    driver.GPUPtr
	dEdgeArrayAux driver.GPUPtr
	dFlag         driver.GPUPtr

	useUnifiedMemory bool
}

// NewBenchmark creates a new BFS benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.loadProgram()
	return b
}

// SelectGPU configures which GPU the benchmark can use
func (b *Benchmark) SelectGPU(gpus []int) {
	if len(gpus) > 1 {
		panic("BFS does not support multi-GPU execution yet.")
	}
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) loadProgram() {
	hsacoBytes := _escFSMustByte(false, "/kernels.hsaco")

	b.kernel = kernels.LoadProgramFromMemory(
		hsacoBytes, "BFS_kernel_warp")
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

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	b.graph.generate(b.NumNode, b.Degree)

	b.hFrontier = make([]uint32, b.NumNode)
	b.hCost = make([]uint32, b.NumNode)

	for i := 0; i < b.NumNode; i++ {
		b.hFrontier[i] = math.MaxUint32
		b.hCost[i] = math.MaxUint32
	}

	b.hFrontier[b.sourceNode] = 0
	b.hCost[b.sourceNode] = 0

	if b.useUnifiedMemory {
		b.dFrontier = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.NumNode*4))
		b.dEdgeArray = b.driver.AllocateUnifiedMemory(b.context,
			uint64((b.NumNode+1)*4))
		b.dEdgeArrayAux = b.driver.AllocateUnifiedMemory(b.context,
			uint64(len(b.graph.edgeList)*4))
		b.dFlag = b.driver.AllocateUnifiedMemory(b.context,
			4)
	} else {
		b.dFrontier = b.driver.AllocateMemory(b.context,
			uint64(b.NumNode*4))
		b.dEdgeArray = b.driver.AllocateMemory(b.context,
			uint64((b.NumNode+1)*4))
		b.dEdgeArrayAux = b.driver.AllocateMemory(b.context,
			uint64(len(b.graph.edgeList)*4))
		b.dFlag = b.driver.AllocateMemory(b.context,
			4)
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dFrontier, b.hFrontier)
	b.driver.MemCopyH2D(b.context, b.dEdgeArray, b.graph.edgeOffsets)
	b.driver.MemCopyH2D(b.context, b.dEdgeArrayAux, b.graph.edgeList)
	maxTheadsPerCore := 1024
	globalSize := uint32(((b.NumNode-1)/maxTheadsPerCore + 1) * maxTheadsPerCore)
	localSize := uint16(maxTheadsPerCore)

	args := KernelArg{
		Levels:       b.dFrontier,
		EdgeArray:    b.dEdgeArray,
		EdgeArrayAux: b.dEdgeArrayAux,
		WSize:        32,
		ChunkSize:    32,
		NumNodes:     uint32(b.NumNode),
		Flag:         b.dFlag,
	}

	for i := 0; i < b.MaxDepth; i++ {
		flag := int32(0)
		b.driver.MemCopyH2D(b.context, b.dFlag, flag)
		args.Curr = int32(i)

		b.driver.LaunchKernel(b.context,
			b.kernel,
			[3]uint32{globalSize, 1, 1},
			[3]uint16{localSize, 1, 1},
			&args)

		b.driver.MemCopyD2H(b.context, &flag, b.dFlag)
		if flag == 0 {
			break
		}
	}

	b.driver.MemCopyD2H(b.context, b.hCost, b.dFrontier)
}

// Verify runs the benchmark on the CPU and compares if the result matches
func (b *Benchmark) Verify() {
	b.cpuBFS()

	for i := 0; i < b.NumNode; i++ {
		if b.cpuCost[i] != b.hCost[i] {
			log.Panicf(
				"mismatch at node %d, expected cost %d, but get %d\n",
				i, b.cpuCost[i], b.hCost[i])
		}
	}
	log.Printf("Passed!\n")
}

func (b *Benchmark) cpuBFS() {
	b.cpuCost = make([]uint32, b.NumNode)
	for i := 0; i < b.NumNode; i++ {
		b.cpuCost[i] = math.MaxUint32
	}

	queue := make([]int, 0)
	queue = append(queue, b.sourceNode)
	b.cpuCost[b.sourceNode] = 0

	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for i := b.graph.edgeOffsets[n]; i < b.graph.edgeOffsets[n+1]; i++ {
			next := b.graph.edgeList[i]
			if b.cpuCost[next] == math.MaxUint32 {
				b.cpuCost[next] = b.cpuCost[n] + 1
				queue = append(queue, int(next))
			}
		}
	}
}
