// Package fir implements the FIR benchmark form Hetero-Mark.
package fir

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/arch"
	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// GCN3KernelArgs defines kernel arguments for GCN3 architecture
type GCN3KernelArgs struct {
	Output              driver.Ptr
	Filter              driver.Ptr
	Input               driver.Ptr
	History             driver.Ptr
	NumTaps             uint32
	Padding             uint32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// CDNA3KernelArgs defines kernel arguments for CDNA3 architecture (GFX942)
type CDNA3KernelArgs struct {
	Output  driver.Ptr // offset 0
	Filter  driver.Ptr // offset 8
	Input   driver.Ptr // offset 16
	History driver.Ptr // offset 24
	NumTaps uint32     // offset 32
	Pad     uint32     // offset 36 - alignment padding
	// Hidden kernel arguments (required by HIP runtime for GFX942)
	HiddenBlockCountX   uint32   // offset 40
	HiddenBlockCountY   uint32   // offset 44
	HiddenBlockCountZ   uint32   // offset 48
	HiddenGroupSizeX    uint16   // offset 52
	HiddenGroupSizeY    uint16   // offset 54
	HiddenGroupSizeZ    uint16   // offset 56
	HiddenRemainderX    uint16   // offset 58
	HiddenRemainderY    uint16   // offset 60
	HiddenRemainderZ    uint16   // offset 62
	Padding             [16]byte // offset 64-79 - reserved
	HiddenGlobalOffsetX int64    // offset 80
	HiddenGlobalOffsetY int64    // offset 88
	HiddenGlobalOffsetZ int64    // offset 96
	HiddenGridDims      uint16   // offset 104
}

// Benchmark defines a benchmark
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch         arch.Type
	Length       int
	NumTapsParam int
	numTaps      int
	inputData    []float32
	filterData   []float32
	gFilterData  []driver.Ptr
	gHistoryData driver.Ptr
	gInputData   driver.Ptr
	gOutputData  driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels.hsaco
var gcn3HSACOBytes []byte

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	var hsacoBytes []byte
	if b.Arch == arch.CDNA3 {
		hsacoBytes = cdna3HSACOBytes
	} else {
		hsacoBytes = gcn3HSACOBytes
	}
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "FIR")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU select GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// Run runs
func (b *Benchmark) Run() {
	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.NumTapsParam > 0 {
		b.numTaps = b.NumTapsParam
	} else if b.numTaps == 0 {
		b.numTaps = 16
	}

	b.filterData = make([]float32, b.numTaps)
	for i := 0; i < b.numTaps; i++ {
		b.filterData[i] = float32(i)
	}

	b.inputData = make([]float32, b.Length)
	for i := 0; i < b.Length; i++ {
		b.inputData[i] = float32(i)
	}

	if b.useUnifiedMemory {
		b.gFilterData = make([]driver.Ptr, len(b.gpus))
		b.gHistoryData = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.numTaps*4))
		b.gInputData = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.Length*4))
		b.gOutputData = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.Length*4))
	} else {
		b.gFilterData = make([]driver.Ptr, len(b.gpus))
		b.gHistoryData = b.driver.AllocateMemory(
			b.context, uint64(b.numTaps*4))
		b.gInputData = b.driver.AllocateMemory(
			b.context, uint64(b.Length*4))
		b.driver.Distribute(b.context,
			b.gInputData, uint64(b.Length*4), b.gpus)
		b.gOutputData = b.driver.AllocateMemory(
			b.context, uint64(b.Length*4))
		b.driver.Distribute(b.context,
			b.gOutputData, uint64(b.Length*4), b.gpus)
	}

	b.driver.MemCopyH2D(b.context, b.gInputData, b.inputData)

	for i, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		if b.useUnifiedMemory {
			b.gFilterData[i] = b.driver.AllocateUnifiedMemory(
				b.context, uint64(b.numTaps*4))
		} else {
			b.gFilterData[i] = b.driver.AllocateMemory(
				b.context, uint64(b.numTaps*4))
		}
		b.driver.MemCopyH2D(b.context, b.gFilterData[i], b.filterData)
	}
}

func (b *Benchmark) enqueueKernel(queue *driver.CommandQueue, gpuIndex, numGPUs int) {
	numWi := b.Length
	gridSize := uint32(numWi / numGPUs)

	if b.Arch == arch.CDNA3 {
		wgSizeX := uint16(256)
		wgSizeY := uint16(1)
		wgSizeZ := uint16(1)

		kernArg := CDNA3KernelArgs{
			Output:  b.gOutputData,
			Filter:  b.gFilterData[gpuIndex],
			Input:   b.gInputData,
			History: b.gHistoryData,
			NumTaps: uint32(b.numTaps),
			// Hidden kernel arguments for GFX942
			HiddenBlockCountX:   gridSize / uint32(wgSizeX),
			HiddenBlockCountY:   1,
			HiddenBlockCountZ:   1,
			HiddenGroupSizeX:    wgSizeX,
			HiddenGroupSizeY:    wgSizeY,
			HiddenGroupSizeZ:    wgSizeZ,
			HiddenRemainderX:    uint16(gridSize % uint32(wgSizeX)),
			HiddenRemainderY:    0,
			HiddenRemainderZ:    0,
			HiddenGlobalOffsetX: int64(gpuIndex * numWi / numGPUs),
			HiddenGlobalOffsetY: 0,
			HiddenGlobalOffsetZ: 0,
			HiddenGridDims:      1,
		}

		b.driver.EnqueueLaunchKernel(
			queue,
			b.hsaco,
			[3]uint32{gridSize, 1, 1},
			[3]uint16{256, 1, 1}, &kernArg,
		)
	} else {
		kernArg := GCN3KernelArgs{
			b.gOutputData,
			b.gFilterData[gpuIndex],
			b.gInputData,
			b.gHistoryData,
			uint32(b.numTaps),
			0,
			int64(gpuIndex * numWi / numGPUs), 0, 0,
		}

		b.driver.EnqueueLaunchKernel(
			queue,
			b.hsaco,
			[3]uint32{gridSize, 1, 1},
			[3]uint16{256, 1, 1}, &kernArg,
		)
	}
}

func (b *Benchmark) exec() {
	queues := make([]*driver.CommandQueue, len(b.gpus))

	for i, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		queues[i] = b.driver.CreateCommandQueue(b.context)
		b.enqueueKernel(queues[i], i, len(b.gpus))
	}

	for i := range b.gpus {
		b.driver.DrainCommandQueue(queues[i])
	}
}

// Verify verifies
func (b *Benchmark) Verify() {
	gpuOutput := make([]float32, b.Length)
	b.driver.MemCopyD2H(b.context, gpuOutput, b.gOutputData)

	for i := 0; i < b.Length; i++ {
		var sum float32
		sum = 0

		for j := 0; j < b.numTaps; j++ {
			if i < j {
				continue
			}
			sum += b.inputData[i-j] * b.filterData[j]
		}

		if math.Abs(float64(sum-gpuOutput[i])) >= 1e-5 {
			log.Fatalf("At position %d, expected %f, but get %f.\n",
				i, sum, gpuOutput[i])
		}
	}

	log.Printf("Passed!\n")
}
