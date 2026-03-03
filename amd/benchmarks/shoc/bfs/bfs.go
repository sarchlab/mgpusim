// Package bfs implements the bfs benchmark from the SHOC suite.
package bfs

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/arch"
	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
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

// CDNA3KernelArg represents the arguments for CDNA3 (gfx942) architecture
type CDNA3KernelArg struct {
	Levels       driver.Ptr // offset 0
	EdgeArray    driver.Ptr // offset 8
	EdgeArrayAux driver.Ptr // offset 16
	WSize        int32      // offset 24
	ChunkSize    int32      // offset 28
	NumNodes     uint32     // offset 32
	Curr         int32      // offset 36
	Flag         driver.Ptr // offset 40
	// Hidden args required by HIP runtime for gfx942
	HiddenBlockCountX   uint32   // offset 48
	HiddenBlockCountY   uint32   // offset 52
	HiddenBlockCountZ   uint32   // offset 56
	HiddenGroupSizeX    uint16   // offset 60
	HiddenGroupSizeY    uint16   // offset 62
	HiddenGroupSizeZ    uint16   // offset 64
	HiddenRemainderX    uint16   // offset 66
	HiddenRemainderY    uint16   // offset 68
	HiddenRemainderZ    uint16   // offset 70
	Padding             [16]byte // offset 72-87
	HiddenGlobalOffsetX int64    // offset 88
	HiddenGlobalOffsetY int64    // offset 96
	HiddenGlobalOffsetZ int64    // offset 104
	HiddenGridDims      uint16   // offset 112
}

// Benchmark is the BFS benchmark
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue
	kernel  *insts.KernelCodeObject

	Arch          arch.Type
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
	b.kernel = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "BFS_kernel_warp")
	if b.kernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// Run runs the benchmark
func (b *Benchmark) Run() {
	b.loadProgram()

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

	cdna3Args := CDNA3KernelArg{
		Levels:            b.dFrontier,
		EdgeArray:         b.dEdgeArray,
		EdgeArrayAux:      b.dEdgeArrayAux,
		WSize:             32,
		ChunkSize:         32,
		NumNodes:          uint32(b.NumNode),
		Flag:              b.dFlag,
		HiddenBlockCountX: globalSize / uint32(localSize),
		HiddenBlockCountY: 1,
		HiddenBlockCountZ: 1,
		HiddenGroupSizeX:  localSize,
		HiddenGroupSizeY:  1,
		HiddenGroupSizeZ:  1,
	}

	for i := 0; i < b.MaxDepth; i++ {
		flag := int32(0)
		b.driver.MemCopyH2D(b.context, b.dFlag, flag)

		if b.Arch == arch.CDNA3 {
			cdna3Args.Curr = int32(i)
			b.driver.LaunchKernel(b.context, b.kernel,
				[3]uint32{globalSize, 1, 1},
				[3]uint16{localSize, 1, 1},
				&cdna3Args)
		} else {
			args.Curr = int32(i)
			b.driver.LaunchKernel(b.context, b.kernel,
				[3]uint32{globalSize, 1, 1},
				[3]uint16{localSize, 1, 1},
				&args)
		}

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
