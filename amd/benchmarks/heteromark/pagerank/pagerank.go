// Package pagerank implements the PageRank benchmark form Hetero-Mark.
package pagerank

import (
	// embed hsaco files
	_ "embed"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/matrix/csr"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
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
	*State
	benchmarks.Saved[checkpointConfig, State]

	driver *driver.Runtime

	kernel *insts.KernelCodeObject
}

// NewBenchmark returns a benchmark
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Saved = benchmarks.NewSaved(b.State,
		b.checkpointConfig, b.bind)

	b.driver = driver
	b.Context = driver.Init()
	return b
}

// SelectGPU select GPU
func (b *Benchmark) SelectGPU(gpus []int) {
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

	b.kernel = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "PageRankUpdateGpu")
	if b.kernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) setup() {
	b.driver.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.driver.CreateCommandQueue(b.Context)

	b.loadProgram()

	for _, gpu := range b.Gpus {
		b.driver.SelectGPU(b.Context, gpu)
		b.Queues = append(b.Queues, b.driver.CreateCommandQueue(b.Context))
	}

	b.initMem()
}

func (b *Benchmark) initMem() {
	initData := float32(1.0) / float32(b.NumNodes)
	b.HPageRank = make([]float32, b.NumNodes)
	b.VerPageRank = make([]float32, b.NumNodes)
	b.VerPageRankTemp = make([]float32, b.NumNodes)
	b.HMatrix = csr.MakeMatrixGenerator(b.NumNodes, b.NumConnections).
		GenerateMatrix()

	for i := uint32(0); i < b.NumNodes; i++ {
		b.HPageRank[i] = initData
		b.VerPageRank[i] = initData
	}

	if b.UseUnifiedMemory {
		b.DPageRank = b.driver.AllocateUnifiedMemory(
			b.Context, uint64(b.NumNodes*4))
		b.DPageRankTemp = b.driver.AllocateUnifiedMemory(
			b.Context, uint64(b.NumNodes*4))
		b.DRowOffsets = b.driver.AllocateUnifiedMemory(
			b.Context, uint64((b.NumNodes+1)*4))
		b.DColumnNumbers = b.driver.AllocateUnifiedMemory(
			b.Context, uint64(b.NumConnections*4))
		b.DValues = b.driver.AllocateUnifiedMemory(
			b.Context, uint64(b.NumConnections*4))
	} else {
		b.DPageRank = b.driver.AllocateMemory(
			b.Context, uint64(b.NumNodes*4))
		b.DPageRankTemp = b.driver.AllocateMemory(
			b.Context, uint64(b.NumNodes*4))
		b.DRowOffsets = b.driver.AllocateMemory(
			b.Context, uint64((b.NumNodes+1)*4))
		b.DColumnNumbers = b.driver.AllocateMemory(
			b.Context, uint64(b.NumConnections*4))
		b.DValues = b.driver.AllocateMemory(
			b.Context, uint64(b.NumConnections*4))
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
		xPtr = b.DPageRank
		yPtr = b.DPageRankTemp
	} else {
		xPtr = b.DPageRankTemp
		yPtr = b.DPageRank
	}

	kernArg := CDNA3KernelArgs{
		NumRows:             b.NumNodes,
		RowOffset:           b.DRowOffsets,
		Col:                 b.DColumnNumbers,
		Val:                 b.DValues,
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

	b.driver.EnqueueLaunchKernel(b.Queue,
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
			RowOffset: b.DRowOffsets,
			Col:       b.DColumnNumbers,
			Val:       b.DValues,
			Vals:      b.DLocalValues,
			X:         b.DPageRank,
			Y:         b.DPageRankTemp,
		}
	} else {
		kernArg = KernelArgs{
			NumRows:   b.NumNodes,
			RowOffset: b.DRowOffsets,
			Col:       b.DColumnNumbers,
			Val:       b.DValues,
			Vals:      b.DLocalValues,
			X:         b.DPageRankTemp,
			Y:         b.DPageRank,
		}
	}

	b.driver.EnqueueLaunchKernel(b.Queue,
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
	b.driver.EnqueueMemCopyH2D(b.Queue, b.DPageRank, b.HPageRank)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.DRowOffsets,
		b.HMatrix.RowOffsets)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.DColumnNumbers,
		b.HMatrix.ColumnNumbers)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.DValues,
		b.HMatrix.Values)

	b.DLocalValues = driver.LocalPtr(256)

	localWorkSize := uint16(64)
	globalSize := [3]uint32{b.NumNodes * 64, 1, 1}
	localSize := [3]uint16{localWorkSize, 1, 1}
	i := uint32(0)

	for i = 0; i < b.MaxIterations; i++ {
		b.launchPageRankKernel(i, globalSize, localSize)
	}

	if i%2 != 0 {
		b.Readbacks = append(b.Readbacks, benchmarks.Readback{Source: b.DPageRankTemp, Target: 0})
	} else {
		b.Readbacks = append(b.Readbacks, benchmarks.Readback{Source: b.DPageRank, Target: 1})
	}
}

// Verify verifies
func (b *Benchmark) verify() error {
	var i uint32
	m := b.HMatrix
	for i = 0; i < b.MaxIterations; i++ {
		for i := uint32(0); i < b.NumNodes; i++ {
			newValue := float32(0)
			for j := m.RowOffsets[i]; j < m.RowOffsets[i+1]; j++ {
				newValue += m.Values[j] * b.VerPageRank[m.ColumnNumbers[j]]
			}
			b.VerPageRankTemp[i] = newValue
		}
		copy(b.VerPageRank, b.VerPageRankTemp)
	}

	for i := uint32(0); i < b.NumNodes; i++ {
		if math.Abs(float64(b.VerPageRank[i]-b.HPageRank[i])) > 1e-5 {
			return fmt.Errorf("Mismatch at %d, expected %f, but get %f\n",
				i, b.VerPageRank[i], b.HPageRank[i])
		}
	}

	fmt.Fprintf(os.Stderr, "Passed!\n")

	return nil
}

func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}

	switch b.Phase {
	case 0:
		b.setup()
		b.Phase = 1
	case 1:
		b.exec()
		b.Phase = 2
	case 2:
		for i := range b.Readbacks {
			var dst any
			switch b.Readbacks[i].Target {
			case 0:
				dst = b.HPageRank
			case 1:
				dst = b.HPageRank
			}
			b.Readbacks[i].Command = b.driver.EnqueueMemCopyD2H(b.Queue, b.Readbacks[i].Source, binary.Size(dst))
		}
		b.Phase = 3
	case 3:
		for _, copy := range b.Readbacks {
			var dst any
			switch copy.Target {
			case 0:
				dst = b.HPageRank
			case 1:
				dst = b.HPageRank
			}
			if err := b.driver.ReadResult(copy.Command, dst); err != nil {
				return err
			}
		}
		if b.Verification {
			if err := b.verify(); err != nil {
				return err
			}
		}
		b.Readbacks = nil
		b.Ended = true
	}
	return nil
}
