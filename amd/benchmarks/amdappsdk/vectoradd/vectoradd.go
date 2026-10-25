// Package vectoradd implements the VectorAdd benchmark from AMDAPPSDK.
package vectoradd

import (
	"encoding/binary"
	"fmt"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"log"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// KernelArgs defines kernel arguments
// Layout must match AMDGPU hidden kernel argument format for GFX942
type KernelArgs struct {
	A      driver.Ptr // offset 0 - 8 bytes
	B      driver.Ptr // offset 8 - 8 bytes
	C      driver.Ptr // offset 16 - 8 bytes
	Width  int32      // offset 24 - 4 bytes
	Height int32      // offset 28 - 4 bytes
	// Hidden kernel arguments (required by HIP runtime for GFX942)
	HiddenBlockCountX   uint32   // offset 32 - number of workgroups in X
	HiddenBlockCountY   uint32   // offset 36 - number of workgroups in Y
	HiddenBlockCountZ   uint32   // offset 40 - number of workgroups in Z
	HiddenGroupSizeX    uint16   // offset 44 - workgroup size X
	HiddenGroupSizeY    uint16   // offset 46 - workgroup size Y
	HiddenGroupSizeZ    uint16   // offset 48 - workgroup size Z
	HiddenRemainderX    uint16   // offset 50 - grid size % workgroup size X
	HiddenRemainderY    uint16   // offset 52 - grid size % workgroup size Y
	HiddenRemainderZ    uint16   // offset 54 - grid size % workgroup size Z
	Padding             [16]byte // offset 56-71 - reserved
	HiddenGlobalOffsetX int64    // offset 72 - global offset X
	HiddenGlobalOffsetY int64    // offset 80 - global offset Y
	HiddenGlobalOffsetZ int64    // offset 88 - global offset Z
	HiddenGridDims      uint16   // offset 96 - grid dimensions
}

// Benchmark defines a benchmark
type Benchmark struct {
	benchmarks.Progress
	initialConfig  checkpointConfig
	configCaptured bool
	readbacks      []benchmarks.Readback
	queue          driver.CommandQueue

	driver  *driver.Runtime
	context driver.Context
	kernel  *insts.KernelCodeObject
	gpus    []int

	Width  uint32
	Height uint32

	hA []float32
	hB []float32
	hC []float32

	dA driver.Ptr
	dB driver.Ptr
	dC driver.Ptr

	useUnifiedMemory bool
}

// NewBenchmark returns a benchmark
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.loadProgram()
	return b
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

//go:embed kernels.hsaco
var hsacoBytes []byte

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "_Z15vectoradd_floatPfPKfS1_ii")
	if b.kernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) setup() {
	b.driver.SelectGPU(b.context, b.gpus[0])
	b.queue = b.driver.CreateCommandQueue(b.context)

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
}

func (b *Benchmark) initMem() {
	numData := b.Width * b.Height

	b.hA = make([]float32, numData)
	b.hB = make([]float32, numData)
	b.hC = make([]float32, numData)

	for i := uint32(0); i < numData; i++ {
		b.hB[i] = float32(i)
		b.hC[i] = float32(i) * 100.0
	}

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context, uint64(numData*4))
		b.dB = b.driver.AllocateUnifiedMemory(b.context, uint64(numData*4))
		b.dC = b.driver.AllocateUnifiedMemory(b.context, uint64(numData*4))
	} else {
		b.dA = b.driver.AllocateMemory(b.context, uint64(numData*4))
		b.driver.Distribute(b.context, b.dA, uint64(numData*4), b.gpus)
		b.dB = b.driver.AllocateMemory(b.context, uint64(numData*4))
		b.driver.Distribute(b.context, b.dB, uint64(numData*4), b.gpus)
		b.dC = b.driver.AllocateMemory(b.context, uint64(numData*4))
		b.driver.Distribute(b.context, b.dC, uint64(numData*4), b.gpus)
	}

	b.driver.EnqueueMemCopyH2D(b.queue, b.dB, b.hB)
	b.driver.EnqueueMemCopyH2D(b.queue, b.dC, b.hC)
}

func (b *Benchmark) exec() {
	queues := make([]driver.CommandQueue, len(b.gpus))
	numData := b.Width * b.Height

	// Workgroup size is fixed at 64x1x1
	wgSizeX := uint16(64)
	wgSizeY := uint16(1)
	wgSizeZ := uint16(1)

	for i, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		queues[i] = b.driver.CreateCommandQueue(b.context)

		gridSize := numData / uint32(len(b.gpus))

		kernArg := KernelArgs{
			A:      b.dA,
			B:      b.dB,
			C:      b.dC,
			Width:  int32(b.Width),
			Height: int32(b.Height),
			// Hidden kernel arguments for GFX942 thread ID calculation
			HiddenBlockCountX:   gridSize / uint32(wgSizeX),
			HiddenBlockCountY:   1,
			HiddenBlockCountZ:   1,
			HiddenGroupSizeX:    wgSizeX,
			HiddenGroupSizeY:    wgSizeY,
			HiddenGroupSizeZ:    wgSizeZ,
			HiddenGlobalOffsetX: int64(gridSize * uint32(i)),
			HiddenGlobalOffsetY: 0,
			HiddenGlobalOffsetZ: 0,
		}

		b.driver.EnqueueLaunchKernel(
			queues[i],
			b.kernel,
			[3]uint32{gridSize, 1, 1},
			[3]uint16{64, 1, 1},
			&kernArg,
		)
	}

	b.readbacks = append(b.readbacks, benchmarks.Readback{Source: b.dA, Target: 0})
}

// Verify verifies
func (b *Benchmark) verify() error {
	numData := b.Width * b.Height
	mismatch := false

	for i := uint32(0); i < numData; i++ {
		expected := b.hB[i] + b.hC[i]
		if b.hA[i] != expected {
			log.Printf("mismatch at position %d. Expected %f, but got %f",
				i, expected, b.hA[i])
			mismatch = true
		}
	}

	if mismatch {
		return fmt.Errorf("verification failed")
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) Step() error {
	switch b.Phase {
	case 0:
		b.initialConfig = b.checkpointConfig()
		b.configCaptured = true
		b.setup()
		b.Phase = 1
	case 1:
		b.exec()
		b.Phase = 2
	case 2:
		for i := range b.readbacks {
			var dst any
			switch b.readbacks[i].Target {
			case 0:
				dst = b.hA
			}
			b.readbacks[i].Command = b.driver.EnqueueMemCopyD2H(b.queue, b.readbacks[i].Source, binary.Size(dst))
		}
		b.Phase = 3
	case 3:
		for _, copy := range b.readbacks {
			var dst any
			switch copy.Target {
			case 0:
				dst = b.hA
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
		b.readbacks = nil
		b.Ended = true
	}
	return nil
}
