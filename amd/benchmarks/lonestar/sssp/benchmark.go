// Package sssp implements the Single-Source Shortest Path benchmark
// from the Lonestar suite. It uses Bellman-Ford style iterative edge
// relaxation on a random weighted CSR graph with atomicMin.
package sssp

import (
	"log"
	"math"

	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

const (
	inf          = math.MaxInt32
	edgesPerNode = 6
	maxWeight    = 100
)

// KernelArgs defines the kernel arguments for sssp_relax_kernel.
type KernelArgs struct {
	RowOffsets  driver.Ptr // offset 0
	ColIndices  driver.Ptr // offset 8
	EdgeWeights driver.Ptr // offset 16
	Dist        driver.Ptr // offset 24
	Updated     driver.Ptr // offset 32
	NumNodes    int32      // offset 40
	Pad0        int32      // offset 44
}

// Benchmark defines the SSSP benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.KernelCodeObject

	NumNodes  int
	MaxIters  int
	BlockSize int

	useUnifiedMemory bool

	hRowOffsets  []int32
	hColIndices  []int32
	hEdgeWeights []int32
	hDist        []int32

	dRowOffsets  driver.Ptr
	dColIndices  driver.Ptr
	dEdgeWeights driver.Ptr
	dDist        driver.Ptr
	dUpdated     driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new SSSP benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumNodes = 65536
	b.MaxIters = 100
	b.BlockSize = 256
	return b
}

// SelectGPU selects GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "sssp_relax_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load sssp_relax_kernel")
	}
}

// simpleRand implements a basic LCG to match srand(42)/rand() sequence.
type simpleRand struct {
	state uint64
}

func newSimpleRand(seed uint64) *simpleRand {
	return &simpleRand{state: seed}
}

func (r *simpleRand) next() int32 {
	// glibc LCG: state = state * 1103515245 + 12345
	r.state = r.state*1103515245 + 12345
	return int32((r.state / 65536) % 2147483648)
}

func (b *Benchmark) generateGraph() {
	n := b.NumNodes
	numEdges := n * edgesPerNode

	b.hRowOffsets = make([]int32, n+1)
	b.hColIndices = make([]int32, numEdges)
	b.hEdgeWeights = make([]int32, numEdges)

	rng := newSimpleRand(42)
	offset := 0
	for i := 0; i < n; i++ {
		b.hRowOffsets[i] = int32(offset)
		for j := 0; j < edgesPerNode; j++ {
			b.hColIndices[offset+j] = rng.next() % int32(n)
			b.hEdgeWeights[offset+j] = (rng.next() % maxWeight) + 1
		}
		offset += edgesPerNode
	}
	b.hRowOffsets[n] = int32(offset)
}

// Run runs the benchmark.
func (b *Benchmark) Run() {
	b.loadProgram()

	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues,
			b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	b.generateGraph()

	n := b.NumNodes
	numEdges := n * edgesPerNode

	b.hDist = make([]int32, n)
	for i := 0; i < n; i++ {
		b.hDist[i] = int32(inf)
	}
	b.hDist[0] = 0

	if b.useUnifiedMemory {
		b.dRowOffsets = b.driver.AllocateUnifiedMemory(b.context,
			uint64((n+1)*4))
		b.dColIndices = b.driver.AllocateUnifiedMemory(b.context,
			uint64(numEdges*4))
		b.dEdgeWeights = b.driver.AllocateUnifiedMemory(b.context,
			uint64(numEdges*4))
		b.dDist = b.driver.AllocateUnifiedMemory(b.context,
			uint64(n*4))
		b.dUpdated = b.driver.AllocateUnifiedMemory(b.context, 4)
	} else {
		b.dRowOffsets = b.driver.AllocateMemory(b.context,
			uint64((n+1)*4))
		b.dColIndices = b.driver.AllocateMemory(b.context,
			uint64(numEdges*4))
		b.dEdgeWeights = b.driver.AllocateMemory(b.context,
			uint64(numEdges*4))
		b.dDist = b.driver.AllocateMemory(b.context,
			uint64(n*4))
		b.dUpdated = b.driver.AllocateMemory(b.context, 4)
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dRowOffsets, b.hRowOffsets)
	b.driver.MemCopyH2D(b.context, b.dColIndices, b.hColIndices)
	b.driver.MemCopyH2D(b.context, b.dEdgeWeights, b.hEdgeWeights)
	b.driver.MemCopyH2D(b.context, b.dDist, b.hDist)

	n := b.NumNodes
	blockSize := b.BlockSize
	grid := (n + blockSize - 1) / blockSize

	globalSize := [3]uint32{uint32(grid * blockSize), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	args := KernelArgs{
		RowOffsets:  b.dRowOffsets,
		ColIndices:  b.dColIndices,
		EdgeWeights: b.dEdgeWeights,
		Dist:        b.dDist,
		Updated:     b.dUpdated,
		NumNodes:    int32(n),
	}

	// Iterative relaxation until convergence
	for iter := 0; iter < b.MaxIters; iter++ {
		flag := int32(0)
		b.driver.MemCopyH2D(b.context, b.dUpdated, flag)

		b.driver.LaunchKernel(b.context, b.kernel,
			globalSize, localSize, &args)

		b.driver.MemCopyD2H(b.context, &flag, b.dUpdated)
		if flag == 0 {
			break
		}
	}

	b.driver.MemCopyD2H(b.context, b.hDist, b.dDist)
}

// Verify verifies the result using Dijkstra's algorithm on the CPU.
func (b *Benchmark) Verify() {
	n := b.NumNodes
	cpuDist := make([]int32, n)
	for i := 0; i < n; i++ {
		cpuDist[i] = int32(inf)
	}
	cpuDist[0] = 0

	// Simple Bellman-Ford on CPU
	for iter := 0; iter < n; iter++ {
		updated := false
		for u := 0; u < n; u++ {
			if cpuDist[u] == int32(inf) {
				continue
			}
			start := b.hRowOffsets[u]
			end := b.hRowOffsets[u+1]
			for e := start; e < end; e++ {
				v := b.hColIndices[e]
				w := b.hEdgeWeights[e]
				newDist := cpuDist[u] + w
				if newDist < cpuDist[v] {
					cpuDist[v] = newDist
					updated = true
				}
			}
		}
		if !updated {
			break
		}
	}

	errors := 0
	for i := 0; i < n; i++ {
		if b.hDist[i] != cpuDist[i] {
			if errors < 10 {
				log.Printf("Mismatch at node %d: got %d, expected %d",
					i, b.hDist[i], cpuDist[i])
			}
			errors++
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in SSSP verification", errors)
	}
	log.Printf("Passed!")
}
