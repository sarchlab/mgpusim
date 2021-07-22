// Package bfs implements the bfs benchmark from the SHOC suite.
package bfs

import (
	"fmt"
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"gitlab.com/akita/mgpusim/v2/driver"
	"gitlab.com/akita/mgpusim/v2/insts"
	"gitlab.com/akita/mgpusim/v2/kernels"
)

// KernelArg represents the arguments to pass to the kernel
type KernelArg struct {
	Levels       driver.Ptr
	EdgeArray    driver.Ptr
	EdgeArrayAux driver.Ptr
	WSize        int32
	ChunkSize    int32
	NumNodes     uint32
	Curr         int32
	Flag         driver.Ptr
}

// Benchmark is the BFS benchmark
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue
	kernel  *insts.HsaCo

	Path          string
	NumNode       int
	Degree        int
	MaxDepth      int
	graph         graph
	sourceNode    int
	hEdgeOffsets  []int32
	hEdgeList     []int32
	hFrontier     []uint32
	hCost         []uint32
	cpuCost       []uint32
	dFrontier     driver.Ptr
	dEdgeArray    driver.Ptr
	dEdgeArrayAux driver.Ptr
	dFlag         driver.Ptr

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

//go:embed kernels.hsaco
var hsacoBytes []byte

func (b *Benchmark) loadProgram() {
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
	if b.Path == "" {
		b.graph.generate(b.NumNode, b.Degree)
	} else {
		b.graph.loadGraph(b.Path)
		b.NumNode = len(b.graph.nodes)
	}

	b.hEdgeOffsets, b.hEdgeList = b.graph.asEdgeList()
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
			uint64(len(b.hEdgeList)*4))
		b.dFlag = b.driver.AllocateUnifiedMemory(b.context, 4)
	} else {
		b.dFrontier = b.driver.AllocateMemory(b.context,
			uint64(b.NumNode*4))
		b.dEdgeArray = b.driver.AllocateMemory(b.context,
			uint64((b.NumNode+1)*4))
		b.dEdgeArrayAux = b.driver.AllocateMemory(b.context,
			uint64(len(b.hEdgeList)*4))
		b.dFlag = b.driver.AllocateMemory(b.context, 4)
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dFrontier, b.hFrontier)
	b.driver.MemCopyH2D(b.context, b.dEdgeArray, b.hEdgeOffsets)
	b.driver.MemCopyH2D(b.context, b.dEdgeArrayAux, b.hEdgeList)
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

		fmt.Printf("Depth %d\n", i)

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

		if b.cpuCost[n] >= uint32(b.MaxDepth) {
			break
		}

		for i := b.hEdgeOffsets[n]; i < b.hEdgeOffsets[n+1]; i++ {
			next := b.hEdgeList[i]
			if b.cpuCost[next] == math.MaxUint32 {
				b.cpuCost[next] = b.cpuCost[n] + 1
				queue = append(queue, int(next))
			}
		}
	}
}
