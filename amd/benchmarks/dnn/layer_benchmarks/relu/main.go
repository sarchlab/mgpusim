// Package relu implements the relu algorithm as a benchmark.
package relu

import (
	"log"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/arch"
	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments for GCN3
type KernelArgs struct {
	Count               uint32
	Padding             uint32
	Input               driver.Ptr
	Output              driver.Ptr
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// CDNA3KernelArgs defines kernel arguments for CDNA3 (gfx942)
// Total size: 280 bytes matching HSACO metadata
type CDNA3KernelArgs struct {
	Count               uint32     // offset 0, size 4
	Pad0                uint32     // offset 4 (alignment padding to reach offset 8)
	Input               driver.Ptr // offset 8, size 8
	Output              driver.Ptr // offset 16, size 8
	HiddenBlockCountX   uint32     // offset 24, size 4
	HiddenBlockCountY   uint32     // offset 28, size 4
	HiddenBlockCountZ   uint32     // offset 32, size 4
	HiddenGroupSizeX    uint16     // offset 36, size 2
	HiddenGroupSizeY    uint16     // offset 38, size 2
	HiddenGroupSizeZ    uint16     // offset 40, size 2
	HiddenRemainderX    uint16     // offset 42, size 2
	HiddenRemainderY    uint16     // offset 44, size 2
	HiddenRemainderZ    uint16     // offset 46, size 2
	Pad1                [16]byte   // offset 48, padding to reach offset 64
	HiddenGlobalOffsetX int64      // offset 64, size 8
	HiddenGlobalOffsetY int64      // offset 72, size 8
	HiddenGlobalOffsetZ int64      // offset 80, size 8
	HiddenGridDims      uint16     // offset 88, size 2
	Pad2                [190]byte  // offset 90, padding to reach 280 bytes total
}

// Benchmark defines a benchmark
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	hsaco   *insts.KernelCodeObject

	Arch        arch.Type
	Length      int
	inputData   []float32
	outputData  []float32
	gInputData  driver.Ptr
	gOutputData driver.Ptr

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
	b.context = driver.Init()

	return b
}

func (b *Benchmark) loadProgram() {
	var hsacoBytes []byte
	if b.Arch == arch.CDNA3 {
		hsacoBytes = cdna3HSACOBytes
	} else {
		hsacoBytes = gcn3HSACOBytes
	}
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "ReLUForward")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects GPU
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
	if b.useUnifiedMemory {
		b.gInputData = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.Length*4))
		b.gOutputData = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.Length*4))
	} else {
		b.gInputData = b.driver.AllocateMemory(b.context, uint64(b.Length*4))
		b.driver.Distribute(b.context, b.gInputData, uint64(b.Length*4), b.gpus)

		b.gOutputData = b.driver.AllocateMemory(b.context, uint64(b.Length*4))
		b.driver.Distribute(b.context, b.gOutputData,
			uint64(b.Length*4), b.gpus)
	}

	b.inputData = make([]float32, b.Length)
	b.outputData = make([]float32, b.Length)
	for i := 0; i < b.Length; i++ {
		b.inputData[i] = float32(i) - 0.5
	}

	b.driver.MemCopyH2D(b.context, b.gInputData, b.inputData)
}

func (b *Benchmark) createKernelArgs(
	globalSize [3]uint32,
	localSize [3]uint16,
	globalOffsetX int64,
) interface{} {
	if b.Arch == arch.CDNA3 {
		// Calculate grid dimensions for CDNA3
		gridDimX := (globalSize[0] + uint32(localSize[0]) - 1) / uint32(localSize[0])
		gridDimY := (globalSize[1] + uint32(localSize[1]) - 1) / uint32(localSize[1])
		gridDimZ := (globalSize[2] + uint32(localSize[2]) - 1) / uint32(localSize[2])

		remainderX := globalSize[0] % uint32(localSize[0])
		remainderY := globalSize[1] % uint32(localSize[1])
		remainderZ := globalSize[2] % uint32(localSize[2])

		return &CDNA3KernelArgs{
			Count:               uint32(b.Length),
			Pad0:                0,
			Input:               b.gInputData,
			Output:              b.gOutputData,
			HiddenBlockCountX:   gridDimX,
			HiddenBlockCountY:   gridDimY,
			HiddenBlockCountZ:   gridDimZ,
			HiddenGroupSizeX:    localSize[0],
			HiddenGroupSizeY:    localSize[1],
			HiddenGroupSizeZ:    localSize[2],
			HiddenRemainderX:    uint16(remainderX),
			HiddenRemainderY:    uint16(remainderY),
			HiddenRemainderZ:    uint16(remainderZ),
			Pad1:                [16]byte{},
			HiddenGlobalOffsetX: globalOffsetX,
			HiddenGlobalOffsetY: 0,
			HiddenGlobalOffsetZ: 0,
			HiddenGridDims:      1,
		}
	}

	return &KernelArgs{
		Count:               uint32(b.Length),
		Padding:             0,
		Input:               b.gInputData,
		Output:              b.gOutputData,
		HiddenGlobalOffsetX: globalOffsetX,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
	}
}

func (b *Benchmark) exec() {
	queues := make([]*driver.CommandQueue, len(b.gpus))

	for i, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		q := b.driver.CreateCommandQueue(b.context)
		queues[i] = q

		numWI := b.Length / len(b.gpus)
		globalSize := [3]uint32{uint32(numWI), 1, 1}
		localSize := [3]uint16{64, 1, 1}

		kernArg := b.createKernelArgs(globalSize, localSize, int64(numWI*i))

		b.driver.EnqueueLaunchKernel(
			q,
			b.hsaco,
			globalSize,
			localSize,
			kernArg,
		)
	}

	for _, q := range queues {
		b.driver.DrainCommandQueue(q)
	}

	b.driver.MemCopyD2H(b.context, b.outputData, b.gOutputData)
}

// Verify verifies
func (b *Benchmark) Verify() {
	for i := 0; i < b.Length; i++ {
		if b.inputData[i] > 0 && b.outputData[i] != b.inputData[i] {
			log.Panicf("mismatch at %d, input %f, output %f", i,
				b.inputData[i], b.outputData[i])
		}

		if b.inputData[i] <= 0 && b.outputData[i] != 0 {
			log.Panicf("mismatch at %d, input %f, output %f", i,
				b.inputData[i], b.outputData[i])
		}
	}

	log.Printf("Passed!\n")
}
