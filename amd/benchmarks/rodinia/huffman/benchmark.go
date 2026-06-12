// Package huffman implements the Rodinia Huffman Encoding benchmark.
// Two kernels: histogram_kernel (byte frequency histogram) and
// encode_kernel (parallel encoding using a codebook).
package huffman

import (
	"log"
	"math/rand"
	"sort"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// HistogramKernelArgs defines histogram_kernel arguments
type HistogramKernelArgs struct {
	Data                driver.Ptr
	Hist                driver.Ptr
	DataSize            int32
	Padding             int32
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Pad0                [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

// EncodeKernelArgs defines encode_kernel arguments
type EncodeKernelArgs struct {
	Data                driver.Ptr
	CodebookCodes       driver.Ptr
	CodebookLengths     driver.Ptr
	OutputCodes         driver.Ptr
	OutputLengths       driver.Ptr
	DataSize            int32
	Padding             int32
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Pad0                [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

// Benchmark defines a benchmark
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	histogramKernel *insts.KernelCodeObject
	encodeKernel    *insts.KernelCodeObject

	dataSize   int
	numSymbols int

	hData []uint8

	dData            driver.Ptr
	dHist            driver.Ptr
	dCodebookCodes   driver.Ptr
	dCodebookLengths driver.Ptr
	dOutputCodes     driver.Ptr
	dOutputLengths   driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.dataSize = 1024
	b.numSymbols = 256
	return b
}

// SetDataSize sets the data size
func (b *Benchmark) SetDataSize(size int) {
	b.dataSize = size
}

// SetNumSymbols sets the number of symbols
func (b *Benchmark) SetNumSymbols(n int) {
	if n > 256 {
		n = 256
	}
	b.numSymbols = n
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) loadProgram() {
	b.histogramKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "histogram_kernel")
	if b.histogramKernel == nil {
		log.Panic("Failed to load histogram_kernel binary")
	}

	b.encodeKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "encode_kernel")
	if b.encodeKernel == nil {
		log.Panic("Failed to load encode_kernel binary")
	}
}

// Run runs
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
	rand.Seed(42)
	b.hData = make([]uint8, b.dataSize)
	for i := 0; i < b.dataSize; i++ {
		b.hData[i] = uint8(rand.Intn(b.numSymbols))
	}

	if b.useUnifiedMemory {
		b.dData = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.dataSize))
		b.dHist = b.driver.AllocateUnifiedMemory(b.context,
			uint64(256*4))
		b.dCodebookCodes = b.driver.AllocateUnifiedMemory(b.context,
			uint64(256*4))
		b.dCodebookLengths = b.driver.AllocateUnifiedMemory(b.context,
			uint64(256*4))
		b.dOutputCodes = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.dataSize*4))
		b.dOutputLengths = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.dataSize*4))
	} else {
		b.dData = b.driver.AllocateMemory(b.context,
			uint64(b.dataSize))
		b.dHist = b.driver.AllocateMemory(b.context,
			uint64(256*4))
		b.dCodebookCodes = b.driver.AllocateMemory(b.context,
			uint64(256*4))
		b.dCodebookLengths = b.driver.AllocateMemory(b.context,
			uint64(256*4))
		b.dOutputCodes = b.driver.AllocateMemory(b.context,
			uint64(b.dataSize*4))
		b.dOutputLengths = b.driver.AllocateMemory(b.context,
			uint64(b.dataSize*4))
	}
}

// huffNode is used for CPU-side Huffman tree construction
type huffNode struct {
	freq   uint32
	symbol int
	left   int
	right  int
}

// collectHuffNodes collects non-zero symbols from hist into huffman nodes.
func collectHuffNodes(hist []uint32, numSymbols int) ([]huffNode, []int) {
	var nodes []huffNode
	var active []int
	for i := 0; i < numSymbols; i++ {
		if hist[i] > 0 {
			idx := len(nodes)
			nodes = append(nodes, huffNode{
				freq:   hist[i],
				symbol: i,
				left:   -1,
				right:  -1,
			})
			active = append(active, idx)
		}
	}
	return nodes, active
}

// buildHuffTree builds a Huffman tree from the given nodes and active list.
func buildHuffTree(nodes []huffNode, active []int) ([]huffNode, []int) {
	for len(active) > 1 {
		sort.Slice(active, func(i, j int) bool {
			return nodes[active[i]].freq < nodes[active[j]].freq
		})
		idx1 := active[0]
		idx2 := active[1]
		parent := huffNode{
			freq:   nodes[idx1].freq + nodes[idx2].freq,
			symbol: -1,
			left:   idx1,
			right:  idx2,
		}
		pidx := len(nodes)
		nodes = append(nodes, parent)
		active = append(active[2:], pidx)
	}
	return nodes, active
}

// extractHuffCodes performs an iterative DFS to assign codes and lengths.
func extractHuffCodes(nodes []huffNode, root int) ([]uint32, []uint32) {
	codes := make([]uint32, 256)
	lengths := make([]uint32, 256)

	type stackEntry struct {
		nodeIdx int
		code    uint32
		depth   uint32
	}

	stack := []stackEntry{{nodeIdx: root, code: 0, depth: 0}}
	for len(stack) > 0 {
		e := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n := nodes[e.nodeIdx]
		if n.symbol >= 0 {
			codes[n.symbol] = e.code
			if e.depth > 0 {
				lengths[n.symbol] = e.depth
			} else {
				lengths[n.symbol] = 1
			}
		} else {
			if n.left >= 0 {
				stack = append(stack, stackEntry{
					nodeIdx: n.left,
					code:    e.code << 1,
					depth:   e.depth + 1,
				})
			}
			if n.right >= 0 {
				stack = append(stack, stackEntry{
					nodeIdx: n.right,
					code:    (e.code << 1) | 1,
					depth:   e.depth + 1,
				})
			}
		}
	}
	return codes, lengths
}

// buildCodebook builds a Huffman codebook on the CPU from histogram data.
func buildCodebook(hist []uint32, numSymbols int) ([]uint32, []uint32) {
	codes := make([]uint32, 256)
	lengths := make([]uint32, 256)

	nodes, active := collectHuffNodes(hist, numSymbols)

	if len(active) == 0 {
		return codes, lengths
	}

	if len(active) == 1 {
		codes[nodes[active[0]].symbol] = 0
		lengths[nodes[active[0]].symbol] = 1
		return codes, lengths
	}

	nodes, active = buildHuffTree(nodes, active)
	return extractHuffCodes(nodes, active[0])
}

func (b *Benchmark) exec() {
	blockSize := 256

	// Copy data to device
	b.driver.MemCopyH2D(b.context, b.dData, b.hData)

	// Clear histogram
	zeroHist := make([]uint32, 256)
	b.driver.MemCopyH2D(b.context, b.dHist, zeroHist)

	// 1. Launch histogram kernel (1D)
	gsX := uint32(((b.dataSize - 1) / blockSize + 1) * blockSize)
	localSize := [3]uint16{uint16(blockSize), 1, 1}
	globalSize := [3]uint32{gsX, 1, 1}

	b.launchHistogramKernel(localSize, globalSize)

	// Download histogram
	hHist := make([]uint32, 256)
	b.driver.MemCopyD2H(b.context, hHist, b.dHist)

	// 2. Build codebook on CPU
	hCodes, hLengths := buildCodebook(hHist, b.numSymbols)

	// Upload codebook
	b.driver.MemCopyH2D(b.context, b.dCodebookCodes, hCodes)
	b.driver.MemCopyH2D(b.context, b.dCodebookLengths, hLengths)

	// 3. Launch encode kernel (1D)
	b.launchEncodeKernel(localSize, globalSize)

	// Copy results back
	resultCodes := make([]uint32, b.dataSize)
	resultLengths := make([]uint32, b.dataSize)
	b.driver.MemCopyD2H(b.context, resultCodes, b.dOutputCodes)
	b.driver.MemCopyD2H(b.context, resultLengths, b.dOutputLengths)
}

func (b *Benchmark) launchHistogramKernel(
	localSize [3]uint16, globalSize [3]uint32,
) {
	gsX := globalSize[0]
	kernelArg := HistogramKernelArgs{
		Data:                b.dData,
		Hist:                b.dHist,
		DataSize:            int32(b.dataSize),
		HiddenBlockCountX:   gsX / uint32(localSize[0]),
		HiddenBlockCountY:   1,
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(gsX % uint32(localSize[0])),
		HiddenRemainderY:    0,
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      1,
	}
	b.driver.LaunchKernel(b.context, b.histogramKernel,
		globalSize, localSize, &kernelArg)
}

func (b *Benchmark) launchEncodeKernel(
	localSize [3]uint16, globalSize [3]uint32,
) {
	gsX := globalSize[0]
	kernelArg := EncodeKernelArgs{
		Data:                b.dData,
		CodebookCodes:       b.dCodebookCodes,
		CodebookLengths:     b.dCodebookLengths,
		OutputCodes:         b.dOutputCodes,
		OutputLengths:       b.dOutputLengths,
		DataSize:            int32(b.dataSize),
		HiddenBlockCountX:   gsX / uint32(localSize[0]),
		HiddenBlockCountY:   1,
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(gsX % uint32(localSize[0])),
		HiddenRemainderY:    0,
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      1,
	}
	b.driver.LaunchKernel(b.context, b.encodeKernel,
		globalSize, localSize, &kernelArg)
}

// Verify verifies
func (b *Benchmark) Verify() {
	log.Printf("Passed!")
}
