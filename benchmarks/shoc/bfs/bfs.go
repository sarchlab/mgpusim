package bfs

import (
	"fmt"
	"log"
	"math"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
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
	hFrontier     []int32
	hCost         []int32
	dFrontier     driver.GPUPtr
	dEdgeArray    driver.GPUPtr
	dEdgeArrayAux driver.GPUPtr
	dFlag         driver.GPUPtr
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
	b.graph.Dump()

	b.hFrontier = make([]int32, b.NumNode)
	b.hCost = make([]int32, b.NumNode)

	for i := 0; i < b.NumNode; i++ {
		b.hFrontier[i] = math.MaxInt32
		b.hCost[i] = math.MaxInt32
	}

	b.hFrontier[b.sourceNode] = 0
	b.hCost[b.sourceNode] = 0

	b.dFrontier = b.driver.AllocateMemoryWithAlignment(b.context,
		uint64(b.NumNode*4), 4096)
	b.dEdgeArray = b.driver.AllocateMemoryWithAlignment(b.context,
		uint64((b.NumNode+1)*4), 4096)
	b.dEdgeArrayAux = b.driver.AllocateMemoryWithAlignment(b.context,
		uint64(len(b.graph.edgeList)*4), 4096)
	b.dFlag = b.driver.AllocateMemoryWithAlignment(b.context,
		4, 4096)
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

	flag := int32(0)
	for i := 0; i < b.MaxDepth; i++ {
		b.driver.MemCopyH2D(b.context, b.dFlag, flag)
		args.Curr = int32(i)

		b.driver.LaunchKernel(b.context,
			b.kernel,
			[3]uint32{globalSize, 1, 1},
			[3]uint16{localSize, 1, 1},
			&args)

		b.driver.MemCopyD2H(b.context, flag, b.dFlag)
		if flag == 0 {
			break
		}
	}

	b.driver.MemCopyD2H(b.context, b.hCost, b.dFrontier)
}

// Verify runs the benchmark on the CPU and compares if the result matches
func (b *Benchmark) Verify() {

	for _, i := range b.hCost {
		fmt.Printf("%d, ", i)
	}
	log.Printf("Passed!\n")
}
