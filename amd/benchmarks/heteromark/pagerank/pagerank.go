// Package pagerank implements the PageRank benchmark form Hetero-Mark.
package pagerank

import (
	"fmt"
	"log"
	"math"
	"os"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/arch"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/matrix/csr"
	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// GCN3 Kernel Arguments

// KernelArgs defines kernel arguments for GCN3
type KernelArgs struct {
	NumRows   uint32
	Padding   uint32
	RowOffset driver.Ptr
	Col       driver.Ptr
	Val       driver.Ptr
	Vals      driver.LocalPtr
	Padding2  uint32
	X         driver.Ptr
	Y         driver.Ptr
}

// CDNA3 Kernel Arguments

// CDNA3KernelArgs defines kernel arguments for CDNA3 architecture (GFX942)
type CDNA3KernelArgs struct {
	NumRows             uint32
	Padding             uint32
	RowOffset           driver.Ptr
	Col                 driver.Ptr
	Val                 driver.Ptr
	X                   driver.Ptr
	Y                   driver.Ptr
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Padding2            [16]byte
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
	kernel  *insts.KernelCodeObject

	Arch           arch.Type
	NumNodes       uint32
	NumConnections uint32
	MaxIterations  uint32

	hMatrix         csr.Matrix
	hPageRank       []float32
	verPageRank     []float32
	verPageRankTemp []float32

	dPageRank      driver.Ptr
	dPageRankTemp  driver.Ptr
	dRowOffsets    driver.Ptr
	dColumnNumbers driver.Ptr
	dValues        driver.Ptr
	dLocalValues   driver.LocalPtr

	useUnifiedMemory bool
}

// NewBenchmark returns a benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	return b
}

// SelectGPU select GPU
func (b *Benchmark) SelectGPU(gpus []int) {
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

	b.kernel = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "PageRankUpdateGpu")
	if b.kernel == nil {
		log.Panic("Failed to load kernel binary")
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
	initData := float32(1.0) / float32(b.NumNodes)
	b.hPageRank = make([]float32, b.NumNodes)
	b.verPageRank = make([]float32, b.NumNodes)
	b.verPageRankTemp = make([]float32, b.NumNodes)
	b.hMatrix = csr.MakeMatrixGenerator(b.NumNodes, b.NumConnections).
		GenerateMatrix()

	for i := uint32(0); i < b.NumNodes; i++ {
		b.hPageRank[i] = initData
		b.verPageRank[i] = initData
	}

	if b.useUnifiedMemory {
		b.dPageRank = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.NumNodes*4))
		b.dPageRankTemp = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.NumNodes*4))
		b.dRowOffsets = b.driver.AllocateUnifiedMemory(
			b.context, uint64((b.NumNodes+1)*4))
		b.dColumnNumbers = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.NumConnections*4))
		b.dValues = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.NumConnections*4))
	} else {
		b.dPageRank = b.driver.AllocateMemory(
			b.context, uint64(b.NumNodes*4))
		b.dPageRankTemp = b.driver.AllocateMemory(
			b.context, uint64(b.NumNodes*4))
		b.dRowOffsets = b.driver.AllocateMemory(
			b.context, uint64((b.NumNodes+1)*4))
		b.dColumnNumbers = b.driver.AllocateMemory(
			b.context, uint64(b.NumConnections*4))
		b.dValues = b.driver.AllocateMemory(
			b.context, uint64(b.NumConnections*4))
	}
}

func printMatrix(matrix [][]float32, n uint32) {
	for i := uint32(0); i < n; i++ {
		for j := uint32(0); j < n; j++ {
			fmt.Printf("%f ", matrix[i][j])
		}
		fmt.Printf("\n")
	}
}

func (b *Benchmark) launchCDNA3PageRank(
	i uint32,
	globalSize [3]uint32,
	localSize [3]uint16,
) {
	var xPtr, yPtr driver.Ptr
	if i%2 == 0 {
		xPtr = b.dPageRank
		yPtr = b.dPageRankTemp
	} else {
		xPtr = b.dPageRankTemp
		yPtr = b.dPageRank
	}

	kernArg := CDNA3KernelArgs{
		NumRows:             b.NumNodes,
		RowOffset:           b.dRowOffsets,
		Col:                 b.dColumnNumbers,
		Val:                 b.dValues,
		X:                   xPtr,
		Y:                   yPtr,
		HiddenBlockCountX:   globalSize[0] / uint32(localSize[0]),
		HiddenBlockCountY:   1,
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(globalSize[0] % uint32(localSize[0])),
		HiddenRemainderY:    0,
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      1,
	}

	b.driver.LaunchKernel(
		b.context,
		b.kernel,
		globalSize,
		localSize,
		&kernArg,
	)
}

func (b *Benchmark) launchGCN3PageRank(
	i uint32,
	globalSize [3]uint32,
	localSize [3]uint16,
) {
	var kernArg KernelArgs
	if i%2 == 0 {
		kernArg = KernelArgs{
			NumRows:   b.NumNodes,
			RowOffset: b.dRowOffsets,
			Col:       b.dColumnNumbers,
			Val:       b.dValues,
			Vals:      b.dLocalValues,
			X:         b.dPageRank,
			Y:         b.dPageRankTemp,
		}
	} else {
		kernArg = KernelArgs{
			NumRows:   b.NumNodes,
			RowOffset: b.dRowOffsets,
			Col:       b.dColumnNumbers,
			Val:       b.dValues,
			Vals:      b.dLocalValues,
			X:         b.dPageRankTemp,
			Y:         b.dPageRank,
		}
	}

	b.driver.LaunchKernel(
		b.context,
		b.kernel,
		globalSize,
		localSize,
		&kernArg,
	)
}

func (b *Benchmark) launchPageRankKernel(
	i uint32,
	globalSize [3]uint32,
	localSize [3]uint16,
) {
	if b.Arch == arch.CDNA3 {
		b.launchCDNA3PageRank(i, globalSize, localSize)
	} else {
		b.launchGCN3PageRank(i, globalSize, localSize)
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dPageRank, b.hPageRank)
	b.driver.MemCopyH2D(b.context, b.dRowOffsets,
		b.hMatrix.RowOffsets)
	b.driver.MemCopyH2D(b.context, b.dColumnNumbers,
		b.hMatrix.ColumnNumbers)
	b.driver.MemCopyH2D(b.context, b.dValues,
		b.hMatrix.Values)

	b.dLocalValues = driver.LocalPtr(256)

	localWorkSize := uint16(64)
	globalSize := [3]uint32{b.NumNodes * 64, 1, 1}
	localSize := [3]uint16{localWorkSize, 1, 1}
	i := uint32(0)

	for i = 0; i < b.MaxIterations; i++ {
		b.launchPageRankKernel(i, globalSize, localSize)
	}

	if i%2 != 0 {
		b.driver.MemCopyD2H(b.context, b.hPageRank, b.dPageRankTemp)
	} else {
		b.driver.MemCopyD2H(b.context, b.hPageRank, b.dPageRank)
	}
}

// Verify verifies
func (b *Benchmark) Verify() {
	var i uint32
	m := b.hMatrix
	for i = 0; i < b.MaxIterations; i++ {
		for i := uint32(0); i < b.NumNodes; i++ {
			newValue := float32(0)
			for j := m.RowOffsets[i]; j < m.RowOffsets[i+1]; j++ {
				newValue += m.Values[j] * b.verPageRank[m.ColumnNumbers[j]]
			}
			b.verPageRankTemp[i] = newValue
		}
		copy(b.verPageRank, b.verPageRankTemp)
	}

	for i := uint32(0); i < b.NumNodes; i++ {
		if math.Abs(float64(b.verPageRank[i]-b.hPageRank[i])) > 1e-5 {
			log.Panicf("Mismatch at %d, expected %f, but get %f\n",
				i, b.verPageRank[i], b.hPageRank[i])
		}
	}

	fmt.Fprintf(os.Stderr, "Passed!\n")
}
