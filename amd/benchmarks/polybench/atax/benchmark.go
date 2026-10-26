// Package atax implements the ATAX benchmark from Polybench.
package atax

import (
	// embed hsaco files
	_ "embed"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"math/rand"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// GCN3 Kernel Arguments

// Kernel1Args list first set of kernel arguments for GCN3
type Kernel1Args struct {
	A   driver.Ptr
	X   driver.Ptr
	Tmp driver.Ptr
	NX  int32
	NY  int32
}

// Kernel2Args list second set of kernel arguments for GCN3
type Kernel2Args struct {
	A      driver.Ptr
	Y      driver.Ptr
	Tmp    driver.Ptr
	NX, NY int32
}

// CDNA3 Kernel Arguments

// CDNA3Kernel1Args defines kernel arguments for CDNA3 architecture (GFX942)
type CDNA3Kernel1Args struct {
	A   driver.Ptr // offset 0
	X   driver.Ptr // offset 8
	Tmp driver.Ptr // offset 16
	NX  int32      // offset 24
	NY  int32      // offset 28
	// Implicit args expected by CDNA3 runtime
	Pad1             [12]byte  // offset 32-43 (padding)
	HiddenGroupSizeX uint16    // offset 44 (0x2c)
	HiddenGroupSizeY uint16    // offset 46
	HiddenGroupSizeZ uint16    // offset 48
	Pad2             [238]byte // offset 50-287 (rest of implicit args)
}

// CDNA3Kernel2Args defines kernel arguments for CDNA3 architecture (GFX942)
type CDNA3Kernel2Args struct {
	A   driver.Ptr // offset 0
	Y   driver.Ptr // offset 8
	Tmp driver.Ptr // offset 16
	NX  int32      // offset 24
	NY  int32      // offset 28
	// Implicit args expected by CDNA3 runtime
	Pad1             [12]byte  // offset 32-43 (padding)
	HiddenGroupSizeX uint16    // offset 44 (0x2c)
	HiddenGroupSizeY uint16    // offset 46
	HiddenGroupSizeZ uint16    // offset 48
	Pad2             [238]byte // offset 50-287 (rest of implicit args)
}

// Benchmark defines a benchmark
type Benchmark struct {
	*State
	benchmarks.Saved[checkpointConfig, State]

	driver *driver.Runtime

	kernel1, kernel2 *insts.KernelCodeObject
}

// NewBenchmark makes a new benchmark
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Saved = benchmarks.NewSaved(b.State, b.checkpointConfig, b.bind)

	b.driver = driver
	b.Context = driver.Init()
	return b
}

// SelectGPU selects GPU
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

	b.kernel1 = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "atax_kernel1")
	if b.kernel1 == nil {
		log.Panic("Failed to load kernel binary")
	}

	b.kernel2 = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "atax_kernel2")
	if b.kernel2 == nil {
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
	rand.Seed(1)
	b.A = make([]float32, b.NX*b.NY)
	b.X = make([]float32, b.NX)
	b.Y = make([]float32, b.NY)
	b.YOutput = make([]float32, b.NY)
	b.Tmp = make([]float32, b.NX)

	for i := 0; i < b.NX; i++ {
		b.X[i] = float32(i) * math.Pi
		for j := 0; j < b.NY; j++ {
			b.A[i*b.NY+j] = float32(i) * float32(j) / float32(b.NX)
		}
	}

	if b.UseUnifiedMemory {
		b.DA = b.driver.AllocateUnifiedMemory(b.Context,
			uint64(b.NY*b.NX*4))
		b.DX = b.driver.AllocateUnifiedMemory(b.Context,
			uint64(b.NY*4))
		b.DY = b.driver.AllocateUnifiedMemory(b.Context,
			uint64(b.NY*4))
		b.DTmp = b.driver.AllocateUnifiedMemory(b.Context,
			uint64(b.NX*4))
	} else {
		b.DA = b.driver.AllocateMemory(b.Context,
			uint64(b.NY*b.NX*4))
		b.DX = b.driver.AllocateMemory(b.Context,
			uint64(b.NY*4))
		b.DY = b.driver.AllocateMemory(b.Context,
			uint64(b.NY*4))
		b.DTmp = b.driver.AllocateMemory(b.Context,
			uint64(b.NX*4))
	}
}

func (b *Benchmark) launchKernel1(localSize [3]uint16, globalSize [3]uint32) {
	if b.Arch == arch.CDNA3 {
		kernel1Arg := CDNA3Kernel1Args{
			A:                b.DA,
			X:                b.DX,
			Tmp:              b.DTmp,
			NX:               int32(b.NX),
			NY:               int32(b.NY),
			HiddenGroupSizeX: localSize[0],
			HiddenGroupSizeY: localSize[1],
			HiddenGroupSizeZ: localSize[2],
		}
		b.driver.EnqueueLaunchKernel(b.Queue, b.kernel1,
			globalSize, localSize, &kernel1Arg)
	} else {
		kernel1Arg := Kernel1Args{
			A:   b.DA,
			X:   b.DX,
			Tmp: b.DTmp,
			NX:  int32(b.NX),
			NY:  int32(b.NY),
		}
		b.driver.EnqueueLaunchKernel(b.Queue, b.kernel1,
			globalSize, localSize, &kernel1Arg)
	}
}

func (b *Benchmark) launchKernel2(localSize [3]uint16, globalSize [3]uint32) {
	if b.Arch == arch.CDNA3 {
		kernel2Arg := CDNA3Kernel2Args{
			A:                b.DA,
			Y:                b.DY,
			Tmp:              b.DTmp,
			NX:               int32(b.NX),
			NY:               int32(b.NY),
			HiddenGroupSizeX: localSize[0],
			HiddenGroupSizeY: localSize[1],
			HiddenGroupSizeZ: localSize[2],
		}
		b.driver.EnqueueLaunchKernel(b.Queue, b.kernel2,
			globalSize, localSize, &kernel2Arg)
	} else {
		kernel2Arg := Kernel2Args{
			A:   b.DA,
			Y:   b.DY,
			Tmp: b.DTmp,
			NX:  int32(b.NX),
			NY:  int32(b.NY),
		}
		b.driver.EnqueueLaunchKernel(b.Queue, b.kernel2,
			globalSize, localSize, &kernel2Arg)
	}
}

func (b *Benchmark) exec() {
	b.driver.EnqueueMemCopyH2D(b.Queue, b.DA, b.A)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.DX, b.X)

	localSize := [3]uint16{256, 1, 1}
	globalSizeX := uint32(((b.NX-1)/256 + 1) * 256)
	globalSize := [3]uint32{globalSizeX, 1, 1}

	b.launchKernel1(localSize, globalSize)

	globalSizeX = uint32(((b.NY-1)/256 + 1) * 256)
	globalSize = [3]uint32{globalSizeX, 1, 1}

	b.launchKernel2(localSize, globalSize)

	b.Readbacks = append(b.Readbacks, benchmarks.Readback{Source: b.DY, Target: 0})
}

// Verify verifies
func (b *Benchmark) verify() error {
	b.cpuAtax()

	for i := 0; i < b.NY; i++ {
		if b.CpuY[i] != b.YOutput[i] {
			return fmt.Errorf("Mismatch at %d, expected %f, but get %f",
				i, b.CpuY[i], b.YOutput[i])
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) cpuAtax() {
	b.CpuY = make([]float32, b.NY)
	tmp := make([]float32, b.NX)

	for i := 0; i < b.NY; i++ {
		b.CpuY[i] = 0
	}

	for i := 0; i < b.NX; i++ {
		tmp[i] = 0

		for j := 0; j < b.NY; j++ {
			tmp[i] += b.A[i*b.NY+j] * b.X[j]
		}

		for j := 0; j < b.NY; j++ {
			b.CpuY[j] += b.A[i*b.NY+j] * tmp[i]
		}
	}
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
				dst = b.YOutput
			}
			b.Readbacks[i].Command = b.driver.EnqueueMemCopyD2H(b.Queue, b.Readbacks[i].Source, binary.Size(dst))
		}
		b.Phase = 3
	case 3:
		for _, copy := range b.Readbacks {
			var dst any
			switch copy.Target {
			case 0:
				dst = b.YOutput
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
