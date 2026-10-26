// Package nw defines the Needleman–Wunsch benchmark
package nw

import (
	// embed hsaco files
	_ "embed"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// KernelArgs defines kernel arguments for GCN3 architecture
type KernelArgs struct {
	Reference          driver.Ptr
	InputItemSets      driver.Ptr
	OutputItemSets     driver.Ptr
	LocalInputItemSets driver.LocalPtr
	LocalReference     driver.LocalPtr
	Cols               int32
	Penalty            int32
	Blk                int32
	BlockSize          int32
	BlockWidth         int32
	WorkSize           int32
	OffsetR            int32
	OffsetC            int32
}

// CDNA3KernelArgs defines kernel arguments for CDNA3 architecture (gfx942)
// No LocalInputItemSets/LocalReference - those are __shared__ inside the kernel.
// KernargSegmentByteSize=56 (no hidden args needed - kernel uses AQL packet for dispatch dims)
type CDNA3KernelArgs struct {
	Reference      driver.Ptr // offset 0
	InputItemSets  driver.Ptr // offset 8
	OutputItemSets driver.Ptr // offset 16
	Cols           int32      // offset 24
	Penalty        int32      // offset 28
	Blk            int32      // offset 32
	BlockSize      int32      // offset 36
	BlockWidth     int32      // offset 40
	WorkSize       int32      // offset 44
	OffsetR        int32      // offset 48
	OffsetC        int32      // offset 52
}

var blosum62 = [][]int32{
	{4, -1, -2, -2, 0, -1, -1, 0, -2, -1, -1, -1, -1, -2, -1, 1, 0, -3, -2, 0, -2, -1, 0, -4},
	{-1, 5, 0, -2, -3, 1, 0, -2, 0, -3, -2, 2, -1, -3, -2, -1, -1, -3, -2, -3, -1, 0, -1, -4},
	{-2, 0, 6, 1, -3, 0, 0, 0, 1, -3, -3, 0, -2, -3, -2, 1, 0, -4, -2, -3, 3, 0, -1, -4},
	{-2, -2, 1, 6, -3, 0, 2, -1, -1, -3, -4, -1, -3, -3, -1, 0, -1, -4, -3, -3, 4, 1, -1, -4},
	{0, -3, -3, -3, 9, -3, -4, -3, -3, -1, -1, -3, -1, -2, -3, -1, -1, -2, -2, -1, -3, -3, -2, -4},
	{-1, 1, 0, 0, -3, 5, 2, -2, 0, -3, -2, 1, 0, -3, -1, 0, -1, -2, -1, -2, 0, 3, -1, -4},
	{-1, 0, 0, 2, -4, 2, 5, -2, 0, -3, -3, 1, -2, -3, -1, 0, -1, -3, -2, -2, 1, 4, -1, -4},
	{0, -2, 0, -1, -3, -2, -2, 6, -2, -4, -4, -2, -3, -3, -2, 0, -2, -2, -3, -3, -1, -2, -1, -4},
	{-2, 0, 1, -1, -3, 0, 0, -2, 8, -3, -3, -1, -2, -1, -2, -1, -2, -2, 2, -3, 0, 0, -1, -4},
	{-1, -3, -3, -3, -1, -3, -3, -4, -3, 4, 2, -3, 1, 0, -3, -2, -1, -3, -1, 3, -3, -3, -1, -4},
	{-1, -2, -3, -4, -1, -2, -3, -4, -3, 2, 4, -2, 2, 0, -3, -2, -1, -2, -1, 1, -4, -3, -1, -4},
	{-1, 2, 0, -1, -3, 1, 1, -2, -1, -3, -2, 5, -1, -3, -1, 0, -1, -3, -2, -2, 0, 1, -1, -4},
	{-1, -1, -2, -3, -1, 0, -2, -3, -2, 1, 2, -1, 5, 0, -2, -1, -1, -1, -1, 1, -3, -1, -4},
	{-2, -3, -3, -3, -2, -3, -3, -3, -1, 0, 0, -3, 0, 6, -4, -2, -2, 1, 3, -1, -3, -3, -1, -4},
	{-1, -2, -2, -1, -3, -1, -1, -2, -2, -3, -3, -1, -2, -4, 7, -1, -1, -4, -3, -2, -2, -1, -2, -4},
	{1, -1, 1, 0, -1, 0, 0, 0, -1, -2, -2, 0, -1, -2, -1, 4, 1, -3, -2, -2, 0, 0, 0, -4},
	{0, -1, 0, -1, -1, -1, -1, -2, -2, -1, -1, -1, -1, -2, -1, 1, 5, -2, -2, 0, -1, -1, 0, -4},
	{-3, -3, -4, -4, -2, -2, -3, -2, -2, -3, -2, -3, -1, 1, -4, -3, -2, 11, 2, -3, -4, -3, -2, -4},
	{-2, -2, -2, -3, -2, -1, -2, -3, 2, -1, -1, -2, -1, 3, -3, -2, -2, 2, 7, -1, -3, -2, -1, -4},
	{0, -3, -3, -3, -1, -2, -2, -3, -3, 3, 1, -2, 1, -1, -2, -2, 0, -3, -1, 4, -3, -2, -1, -4},
	{-2, -1, 3, 4, -3, 0, 1, -1, 0, -3, -4, 0, -3, -3, -2, 0, -1, -4, -3, -3, 4, 1, -1, -4},
	{-1, 0, 0, 1, -3, 3, 4, -2, 0, -3, -3, 1, -1, -3, -1, 0, -1, -3, -2, -2, 1, 4, -1, -4},
	{0, -1, -1, -1, -2, -1, -1, -1, -1, -1, -1, -1, -1, -1, -2, 0, 0, -2, -1, -1, -1, -1, -1, -4},
	{-4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, -4, 1},
}

// Benchmark defines a benchmark
type Benchmark struct {
	*State
	benchmarks.Saved[checkpointConfig, State]

	driver *driver.Runtime

	kernel1, kernel2 *insts.KernelCodeObject
}

// NewBenchmark creates a new benchmark
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Saved = benchmarks.NewSaved(b.State, b.checkpointConfig, b.bind)

	b.driver = driver
	b.Context = driver.Init()
	b.Queue = driver.CreateCommandQueue(b.Context)

	b.BlockSize = 64
	b.SetLength(256)
	b.Penalty = 10

	return b
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

	b.kernel1 = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "nw_kernel1")
	if b.kernel1 == nil {
		log.Panic("Failed to load kernel binary nw_kernel1")
	}

	b.kernel2 = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "nw_kernel2")
	if b.kernel2 == nil {
		log.Panic("Failed to load kernel binary nw_kernel2")
	}
}

// SetLength sets length
func (b *Benchmark) SetLength(length int) {
	b.Length = length
	b.Row = length + 1
	b.Col = length + 1
}

// SetPenalty sets penalty
func (b *Benchmark) SetPenalty(penalty int) {
	b.Penalty = penalty
}

// SelectGPU selects gpu
func (b *Benchmark) SelectGPU(gpuIDs []int) {
	if len(gpuIDs) > 1 {
		panic("nw does not support multi-GPU mode")
	}

	b.GpuIDs = gpuIDs
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) setup() {
	b.driver.SelectGPU(b.Context, b.GpuIDs[0])
	b.Queue = b.driver.CreateCommandQueue(b.Context)

	b.loadProgram()
	b.driver.SelectGPU(b.Context, b.GpuIDs[0])
	b.initMem()
}

// SetUnifiedMemory Use Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

func (b *Benchmark) initMem() {
	b.initData()
	b.allocateGPUMem()
}

func (b *Benchmark) initData() {
	rng := rand.New(rand.NewSource(1))
	b.Reference = make([]int32, b.Row*b.Col)
	b.InputItemSets = make([]int32, b.Row*b.Col)
	b.OutputItemSets = make([]int32, b.Row*b.Col)

	for i := 0; i < b.Row; i++ {
		b.InputItemSets[i*b.Col] = int32(rng.Int()%10 + 1)
	}

	for i := 0; i < b.Col; i++ {
		b.InputItemSets[i] = int32(rng.Int()%10 + 1)
	}

	for i := 0; i < b.Col; i++ {
		for j := 0; j < b.Row; j++ {
			b.Reference[i*b.Col+j] =
				blosum62[b.InputItemSets[i*b.Col]][b.InputItemSets[j]]
		}
	}

	b.InputItemSets[0] = 0

	for i := 1; i < b.Row; i++ {
		b.InputItemSets[i*b.Col] = int32(-i * b.Penalty)
	}
	for j := 1; j < b.Col; j++ {
		b.InputItemSets[j] = int32(-j * b.Penalty)
	}
}

func (b *Benchmark) allocateGPUMem() {
	b.DInputItemSets = b.allocate(uint64(b.Col * b.Row * 4))
	b.DOutputItemSets = b.allocate(uint64(b.Col * b.Row * 4))
	b.DReference = b.allocate(uint64(b.Col * b.Row * 4))
	b.driver.Distribute(b.Context, b.DInputItemSets, uint64(b.Col*b.Row*4), b.GpuIDs)
	b.driver.Distribute(b.Context, b.DOutputItemSets, uint64(b.Col*b.Row*4), b.GpuIDs)
	b.driver.Distribute(b.Context, b.DReference, uint64(b.Col*b.Row*4), b.GpuIDs)
}

func (b *Benchmark) allocate(byteSize uint64) driver.Ptr {
	if b.UseUnifiedMemory {
		return b.driver.AllocateUnifiedMemory(b.Context, byteSize)
	}

	return b.driver.AllocateMemory(b.Context, byteSize)
}

func (b *Benchmark) exec() {
	b.copyInputDataToGPU()
	b.runKernel1()
	b.runKernel2()
	b.copyOutputDataFromGPU()
}

func (b *Benchmark) copyInputDataToGPU() {
	b.Upload(b.driver, b.Queue, b.DInputItemSets, b.InputItemSets)
	b.Upload(b.driver, b.Queue, b.DReference, b.Reference)
}

func (b *Benchmark) copyOutputDataFromGPU() {
	b.Readbacks = append(b.Readbacks, benchmarks.Readback{Source: b.DInputItemSets, Target: 0})
}

func (b *Benchmark) runKernel1() {
	workSize := b.Col - 1
	offsetR := 0
	offsetC := 0
	blockWidth := workSize / b.BlockSize

	for blk := 1; blk <= workSize/b.BlockSize; blk++ {
		globalSize := [3]uint32{uint32(b.BlockSize * blk), 1, 1}
		localSize := [3]uint16{uint16(b.BlockSize), 1, 1}

		if b.Arch == arch.CDNA3 {
			cdna3Args := CDNA3KernelArgs{
				Reference:      b.DReference,
				InputItemSets:  b.DInputItemSets,
				OutputItemSets: b.DOutputItemSets,
				Cols:           int32(b.Col),
				Penalty:        int32(b.Penalty),
				Blk:            int32(blk),
				BlockSize:      int32(b.BlockSize),
				BlockWidth:     int32(blockWidth),
				WorkSize:       int32(workSize),
				OffsetR:        int32(offsetR),
				OffsetC:        int32(offsetC),
			}
			b.driver.EnqueueLaunchKernel(b.Queue, b.kernel1, globalSize, localSize, &cdna3Args)
		} else {
			args := KernelArgs{
				Reference:          b.DReference,
				InputItemSets:      b.DInputItemSets,
				OutputItemSets:     b.DOutputItemSets,
				LocalInputItemSets: driver.LocalPtr((b.BlockSize + 1) * (b.BlockSize + 1) * 4),
				LocalReference:     driver.LocalPtr(b.BlockSize * b.BlockSize * 4),
				Cols:               int32(b.Col),
				Penalty:            int32(b.Penalty),
				Blk:                int32(blk),
				BlockSize:          int32(b.BlockSize),
				BlockWidth:         int32(blockWidth),
				WorkSize:           int32(workSize),
				OffsetR:            int32(offsetR),
				OffsetC:            int32(offsetC),
			}
			b.driver.EnqueueLaunchKernel(b.Queue, b.kernel1, globalSize, localSize, &args)
		}
	}
}

func (b *Benchmark) runKernel2() {
	workSize := b.Col - 1
	offsetR := 0
	offsetC := 0
	blockWidth := workSize / b.BlockSize

	for blk := blockWidth - 1; blk >= 1; blk-- {
		globalSize := [3]uint32{uint32(b.BlockSize * blk), 1, 1}
		localSize := [3]uint16{uint16(b.BlockSize), 1, 1}

		if b.Arch == arch.CDNA3 {
			cdna3Args := CDNA3KernelArgs{
				Reference:      b.DReference,
				InputItemSets:  b.DInputItemSets,
				OutputItemSets: b.DOutputItemSets,
				Cols:           int32(b.Col),
				Penalty:        int32(b.Penalty),
				Blk:            int32(blk),
				BlockSize:      int32(b.BlockSize),
				BlockWidth:     int32(blockWidth),
				WorkSize:       int32(workSize),
				OffsetR:        int32(offsetR),
				OffsetC:        int32(offsetC),
			}
			b.driver.EnqueueLaunchKernel(b.Queue, b.kernel2, globalSize, localSize, &cdna3Args)
		} else {
			args := KernelArgs{
				Reference:          b.DReference,
				InputItemSets:      b.DInputItemSets,
				OutputItemSets:     b.DOutputItemSets,
				LocalInputItemSets: driver.LocalPtr((b.BlockSize + 1) * (b.BlockSize + 1) * 4),
				LocalReference:     driver.LocalPtr(b.BlockSize * b.BlockSize * 4),
				Cols:               int32(b.Col),
				Penalty:            int32(b.Penalty),
				Blk:                int32(blk),
				BlockSize:          int32(b.BlockSize),
				BlockWidth:         int32(blockWidth),
				WorkSize:           int32(workSize),
				OffsetR:            int32(offsetR),
				OffsetC:            int32(offsetC),
			}
			b.driver.EnqueueLaunchKernel(b.Queue, b.kernel2, globalSize, localSize, &args)
		}
	}
}

// Verify verifies
func (b *Benchmark) verify() error {
	b.cpuNW()

	for i := 0; i < b.Row; i++ {
		for j := 0; j < b.Col; j++ {
			if b.OutputItemSets[i*b.Col+j] != b.InputItemSets[i*b.Col+j] {
				return fmt.Errorf(
					"mismatch at (%d,%d), expected %d, but got %d\n",
					i, j, b.InputItemSets[i*b.Col+j],
					b.OutputItemSets[i*b.Col+j])
			}
		}
	}

	return nil
}

func (b *Benchmark) cpuNW() {
	for i := 1; i < b.Row; i++ {
		for j := 1; j < b.Col; j++ {
			leftPenalty := b.InputItemSets[i*b.Col+(j-1)] - int32(b.Penalty)
			topPenalty := b.InputItemSets[(i-1)*b.Col+j] - int32(b.Penalty)
			refValue := b.Reference[i*b.Col+j]
			diagPenalty := b.InputItemSets[(i-1)*b.Col+(j-1)] + refValue

			max := leftPenalty
			if topPenalty > max {
				max = topPenalty
			}
			if diagPenalty > max {
				max = diagPenalty
			}

			b.InputItemSets[i*b.Col+j] = max
		}
	}
}

func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if err := b.ReleaseUploads(b.driver); err != nil {
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
				dst = b.OutputItemSets
			}
			b.Readbacks[i].Buffer = benchmarks.Download(b.driver, b.Queue, b.Readbacks[i].Source, binary.Size(dst))
		}
		b.Phase = 3
	case 3:
		for _, copy := range b.Readbacks {
			var dst any
			switch copy.Target {
			case 0:
				dst = b.OutputItemSets
			}
			if err := benchmarks.Read(b.driver, copy.Buffer, dst); err != nil {
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
