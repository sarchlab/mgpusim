// Package fir implements the FIR benchmark form Hetero-Mark.
package fir

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
	*State
	benchmarks.Saved[checkpointConfig, State]

	driver *driver.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels.hsaco
var gcn3HSACOBytes []byte

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a benchmark
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Saved = benchmarks.NewSaved(b.State, b.checkpointConfig, b.bind)

	b.driver = driver
	b.Context = b.driver.Init()

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
	b.Gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) submit() {
	b.loadProgram()

	b.driver.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.driver.CreateCommandQueue(b.Context)
	b.initMem()
}

func (b *Benchmark) initMem() {
	if b.NumTapsParam > 0 {
		b.NumTaps = b.NumTapsParam
	} else if b.NumTaps == 0 {
		b.NumTaps = 16
	}

	b.FilterData = make([]float32, b.NumTaps)
	for i := 0; i < b.NumTaps; i++ {
		b.FilterData[i] = float32(i)
	}

	b.InputData = make([]float32, b.Length)
	for i := 0; i < b.Length; i++ {
		b.InputData[i] = float32(i)
	}

	if b.UseUnifiedMemory {
		b.GFilterData = make([]driver.Ptr, len(b.Gpus))
		b.GHistoryData = b.driver.AllocateUnifiedMemory(
			b.Context, uint64(b.NumTaps*4))
		b.GInputData = b.driver.AllocateUnifiedMemory(
			b.Context, uint64(b.Length*4))
		b.GOutputData = b.driver.AllocateUnifiedMemory(
			b.Context, uint64(b.Length*4))
	} else {
		b.GFilterData = make([]driver.Ptr, len(b.Gpus))
		b.GHistoryData = b.driver.AllocateMemory(
			b.Context, uint64(b.NumTaps*4))
		b.GInputData = b.driver.AllocateMemory(
			b.Context, uint64(b.Length*4))
		b.driver.Distribute(b.Context,
			b.GInputData, uint64(b.Length*4), b.Gpus)
		b.GOutputData = b.driver.AllocateMemory(
			b.Context, uint64(b.Length*4))
		b.driver.Distribute(b.Context,
			b.GOutputData, uint64(b.Length*4), b.Gpus)
	}

	b.Upload(b.driver, b.Queue, b.GInputData, b.InputData)

	for i, gpu := range b.Gpus {
		b.driver.SelectGPU(b.Context, gpu)
		if b.UseUnifiedMemory {
			b.GFilterData[i] = b.driver.AllocateUnifiedMemory(
				b.Context, uint64(b.NumTaps*4))
		} else {
			b.GFilterData[i] = b.driver.AllocateMemory(
				b.Context, uint64(b.NumTaps*4))
		}
		b.Upload(b.driver, b.Queue, b.GFilterData[i], b.FilterData)
	}
}

func (b *Benchmark) enqueueKernel(queue driver.CommandQueue, gpuIndex, numGPUs int) {
	numWi := b.Length
	gridSize := uint32(numWi / numGPUs)

	if b.Arch == arch.CDNA3 {
		wgSizeX := uint16(256)
		wgSizeY := uint16(1)
		wgSizeZ := uint16(1)

		kernArg := CDNA3KernelArgs{
			Output:  b.GOutputData,
			Filter:  b.GFilterData[gpuIndex],
			Input:   b.GInputData,
			History: b.GHistoryData,
			NumTaps: uint32(b.NumTaps),
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
			b.GOutputData,
			b.GFilterData[gpuIndex],
			b.GInputData,
			b.GHistoryData,
			uint32(b.NumTaps),
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
	queues := make([]driver.CommandQueue, len(b.Gpus))

	for i, gpu := range b.Gpus {
		b.driver.SelectGPU(b.Context, gpu)
		queues[i] = b.driver.CreateCommandQueue(b.Context)
		b.enqueueKernel(queues[i], i, len(b.Gpus))
	}
}

// Verify verifies
func (b *Benchmark) verify() error {
	gpuOutput := make([]float32, b.Length)
	if err := benchmarks.Read(b.driver, b.Readbacks[0], gpuOutput); err != nil {
		return err
	}

	for i := 0; i < b.Length; i++ {
		var sum float32
		sum = 0

		for j := 0; j < b.NumTaps; j++ {
			if i < j {
				continue
			}
			sum += b.InputData[i-j] * b.FilterData[j]
		}

		if math.Abs(float64(sum-gpuOutput[i])) >= 1e-5 {
			return fmt.Errorf("At position %d, expected %f, but get %f.\n",
				i, sum, gpuOutput[i])
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.driver, b.Queue, b.GOutputData, 4*(b.Length)))
}

// Step submits computation, then verification readback, then consumes results.
func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if err := b.ReleaseUploads(b.driver); err != nil {
		return err
	}

	switch b.Phase {
	case 0:
		b.submit()
		b.Phase = 1
	case 1:
		b.exec()
		b.Phase = 2
	case 2:
		if !b.Verification {
			b.Ended = true
			return nil
		}
		b.enqueueVerification()
		b.Phase = 3
	case 3:
		if err := b.verify(); err != nil {
			return err
		}
		b.Readbacks = nil
		b.Ended = true
	}
	return nil
}
