// Package bfs implements the bfs benchmark from the SHOC suite.
package bfs

import (
	// embed hsaco files
	_ "embed"
	"fmt"
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
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
	*State
	benchmarks.Saved[checkpointConfig, State]

	driver *driver.Runtime

	kernel *insts.KernelCodeObject
}

// NewBenchmark creates a new BFS benchmark
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Saved = benchmarks.NewSaved(b.State, b.checkpointConfig, b.bind)

	b.driver = driver
	b.Context = driver.Init()
	return b
}

// SelectGPU configures which GPU the benchmark can use
func (b *Benchmark) SelectGPU(gpus []int) {
	if len(gpus) > 1 {
		panic("BFS does not support multi-GPU execution yet.")
	}
	b.Gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
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

// Step advances one breadth-first frontier after the previous global batch.
func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if err := b.ReleaseUploads(b.driver); err != nil {
		return err
	}

	switch b.Phase {
	case 0:
		b.loadProgram()
		b.driver.SelectGPU(b.Context, b.Gpus[0])
		b.Queue = b.driver.CreateCommandQueue(b.Context)
		b.initMem()
		b.Upload(b.driver, b.Queue, b.DFrontier, b.HFrontier)
		b.Upload(b.driver, b.Queue, b.DEdgeArray, b.HEdgeOffsets)
		b.Upload(b.driver, b.Queue, b.DEdgeArrayAux, b.HEdgeList)
		if b.MaxDepth > 0 {
			b.submitIteration()
		} else {
			b.finishTraversal()
		}
	case 1:
		var flag int32
		if err := benchmarks.Read(b.driver, b.FlagResult, &flag); err != nil {
			return err
		}
		if flag != 0 && b.Depth < b.MaxDepth {
			b.submitIteration()
		} else {
			b.finishTraversal()
		}
	case 2:
		if err := benchmarks.Read(b.driver, b.OutputResult, b.HCost); err != nil {
			return err
		}
		if b.Verification {
			if err := b.verify(); err != nil {
				return err
			}
		}
		b.Ended = true
	}
	return nil
}
func (b *Benchmark) finishTraversal() {
	b.OutputResult = benchmarks.Download(b.driver, b.Queue, b.DFrontier, len(b.HCost)*4)
	b.Phase = 2
}

func (b *Benchmark) initMem() {
	var graph graph
	if b.Path == "" {
		graph.generate(b.NumNode, b.Degree)
	} else {
		graph.loadGraph(b.Path)
		b.NumNode = len(graph.nodes)
	}

	b.HEdgeOffsets, b.HEdgeList = graph.asEdgeList()
	b.HFrontier = make([]uint32, b.NumNode)
	b.HCost = make([]uint32, b.NumNode)

	for i := 0; i < b.NumNode; i++ {
		b.HFrontier[i] = math.MaxUint32
		b.HCost[i] = math.MaxUint32
	}

	b.HFrontier[b.SourceNode] = 0
	b.HCost[b.SourceNode] = 0

	if b.UseUnifiedMemory {
		b.DFrontier = b.driver.AllocateUnifiedMemory(b.Context,
			uint64(b.NumNode*4))
		b.DEdgeArray = b.driver.AllocateUnifiedMemory(b.Context,
			uint64((b.NumNode+1)*4))
		b.DEdgeArrayAux = b.driver.AllocateUnifiedMemory(b.Context,
			uint64(len(b.HEdgeList)*4))
		b.DFlag = b.driver.AllocateUnifiedMemory(b.Context, 4)
	} else {
		b.DFrontier = b.driver.AllocateMemory(b.Context,
			uint64(b.NumNode*4))
		b.DEdgeArray = b.driver.AllocateMemory(b.Context,
			uint64((b.NumNode+1)*4))
		b.DEdgeArrayAux = b.driver.AllocateMemory(b.Context,
			uint64(len(b.HEdgeList)*4))
		b.DFlag = b.driver.AllocateMemory(b.Context, 4)
	}
}

func (b *Benchmark) submitIteration() {
	maxTheadsPerCore := 1024
	globalSize := uint32(((b.NumNode-1)/maxTheadsPerCore + 1) * maxTheadsPerCore)
	localSize := uint16(maxTheadsPerCore)

	args := KernelArg{
		Levels:       b.DFrontier,
		EdgeArray:    b.DEdgeArray,
		EdgeArrayAux: b.DEdgeArrayAux,
		WSize:        32,
		ChunkSize:    32,
		NumNodes:     uint32(b.NumNode),
		Flag:         b.DFlag,
	}

	cdna3Args := CDNA3KernelArg{
		Levels:            b.DFrontier,
		EdgeArray:         b.DEdgeArray,
		EdgeArrayAux:      b.DEdgeArrayAux,
		WSize:             32,
		ChunkSize:         32,
		NumNodes:          uint32(b.NumNode),
		Flag:              b.DFlag,
		HiddenBlockCountX: globalSize / uint32(localSize),
		HiddenBlockCountY: 1,
		HiddenBlockCountZ: 1,
		HiddenGroupSizeX:  localSize,
		HiddenGroupSizeY:  1,
		HiddenGroupSizeZ:  1,
	}

	flag := int32(0)
	b.Upload(b.driver, b.Queue, b.DFlag, flag)

	if b.Arch == arch.CDNA3 {
		cdna3Args.Curr = int32(b.Depth)
		b.driver.EnqueueLaunchKernel(b.Queue, b.kernel,
			[3]uint32{globalSize, 1, 1},
			[3]uint16{localSize, 1, 1},
			&cdna3Args)
	} else {
		args.Curr = int32(b.Depth)
		b.driver.EnqueueLaunchKernel(b.Queue, b.kernel,
			[3]uint32{globalSize, 1, 1},
			[3]uint16{localSize, 1, 1},
			&args)
	}

	b.FlagResult = benchmarks.Download(b.driver, b.Queue, b.DFlag, 4)
	b.Depth++
	b.Phase = 1
}

// Verify runs the benchmark on the CPU and compares if the result matches
func (b *Benchmark) verify() error {
	b.cpuBFS()

	for i := 0; i < b.NumNode; i++ {
		if b.CpuCost[i] != b.HCost[i] {
			return fmt.Errorf(
				"mismatch at node %d, expected cost %d, but get %d\n",
				i, b.CpuCost[i], b.HCost[i])
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) cpuBFS() {
	b.CpuCost = make([]uint32, b.NumNode)
	for i := 0; i < b.NumNode; i++ {
		b.CpuCost[i] = math.MaxUint32
	}

	queue := make([]int, 0)
	queue = append(queue, b.SourceNode)
	b.CpuCost[b.SourceNode] = 0

	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]

		if b.CpuCost[n] >= uint32(b.MaxDepth) {
			break
		}

		for i := b.HEdgeOffsets[n]; i < b.HEdgeOffsets[n+1]; i++ {
			next := b.HEdgeList[i]
			if b.CpuCost[next] == math.MaxUint32 {
				b.CpuCost[next] = b.CpuCost[n] + 1
				queue = append(queue, int(next))
			}
		}
	}
}
