// Package bicg implements the bicg benchmark from Polybench.
package bicg

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
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// GCN3 Kernel Arguments

// Kernel1Args list first set of kernel arguments for GCN3
type Kernel1Args struct {
	A      gpuruntime.Ptr
	P      gpuruntime.Ptr
	Q      gpuruntime.Ptr
	NX, NY int32
}

// Kernel2Args list second set of kernel arguments for GCN3
type Kernel2Args struct {
	A      gpuruntime.Ptr
	R      gpuruntime.Ptr
	S      gpuruntime.Ptr
	NX, NY int32
}

// CDNA3 Kernel Arguments

// CDNA3Kernel1Args defines kernel arguments for CDNA3 architecture (GFX942)
type CDNA3Kernel1Args struct {
	A                   gpuruntime.Ptr
	P                   gpuruntime.Ptr
	Q                   gpuruntime.Ptr
	NX                  int32
	NY                  int32
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

// CDNA3Kernel2Args defines kernel arguments for CDNA3 architecture (GFX942)
type CDNA3Kernel2Args struct {
	A                   gpuruntime.Ptr
	R                   gpuruntime.Ptr
	S                   gpuruntime.Ptr
	NX                  int32
	NY                  int32
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
	*State
	benchmarks.Checkpoint[checkpointConfig, State]
	rt *gpuruntime.Runtime

	kernel1, kernel2 *insts.KernelCodeObject
}

// NewBenchmark makes a new benchmark
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.checkpointConfig, b.bind)

	b.rt = rt
	b.Context = rt.Init()
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
		hsacoBytes, "bicgKernel1")
	if b.kernel1 == nil {
		log.Panic("Failed to load kernel binary")
	}

	b.kernel2 = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "bicgKernel2")
	if b.kernel2 == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) setup() {
	b.rt.SelectGPU(b.Context, b.Gpus[len(b.Gpus)-1])
	b.Queue = b.rt.CreateCommandQueue(b.Context)

	b.loadProgram()

	b.initMem()
}

func (b *Benchmark) initMem() {
	rand.Seed(1)
	b.A = make([]float32, b.NX*b.NY)
	b.R = make([]float32, b.NX)
	b.S = make([]float32, b.NY)
	b.P = make([]float32, b.NY)
	b.Q = make([]float32, b.NX)
	b.SOutput = make([]float32, b.NY)
	b.QOutput = make([]float32, b.NX)

	for i := 0; i < b.NX; i++ {
		b.R[i] = float32(i) * math.Pi
		for j := 0; j < b.NY; j++ {
			b.A[i*b.NY+j] = float32(i) * float32(j) / float32(b.NX)
		}
	}

	for i := 0; i < b.NY; i++ {
		b.P[i] = float32(i) * math.Pi
	}

	if b.UseUnifiedMemory {
		b.DA = b.rt.AllocateUnifiedMemory(b.Context,
			uint64(b.NY*b.NX*4))
		b.DR = b.rt.AllocateUnifiedMemory(b.Context,
			uint64(b.NX*4))
		b.DS = b.rt.AllocateUnifiedMemory(b.Context,
			uint64(b.NY*4))
		b.DP = b.rt.AllocateUnifiedMemory(b.Context,
			uint64(b.NY*4))
		b.DQ = b.rt.AllocateUnifiedMemory(b.Context,
			uint64(b.NX*4))
	} else {
		b.DA = b.rt.AllocateMemory(b.Context,
			uint64(b.NY*b.NX*4))
		b.DR = b.rt.AllocateMemory(b.Context,
			uint64(b.NX*4))
		b.DS = b.rt.AllocateMemory(b.Context,
			uint64(b.NY*4))
		b.DP = b.rt.AllocateMemory(b.Context,
			uint64(b.NY*4))
		b.DQ = b.rt.AllocateMemory(b.Context,
			uint64(b.NX*4))
	}
}

func (b *Benchmark) launchKernel1(localSize [3]uint16, globalSize [3]uint32, globalSizeX uint32) {
	if b.Arch == arch.CDNA3 {
		kernel1Arg := CDNA3Kernel1Args{
			A:                   b.DA,
			P:                   b.DP,
			Q:                   b.DQ,
			NX:                  int32(b.NX),
			NY:                  int32(b.NY),
			HiddenBlockCountX:   globalSizeX / uint32(localSize[0]),
			HiddenBlockCountY:   1,
			HiddenBlockCountZ:   1,
			HiddenGroupSizeX:    localSize[0],
			HiddenGroupSizeY:    localSize[1],
			HiddenGroupSizeZ:    localSize[2],
			HiddenRemainderX:    uint16(globalSizeX % uint32(localSize[0])),
			HiddenRemainderY:    0,
			HiddenRemainderZ:    0,
			HiddenGlobalOffsetX: 0,
			HiddenGlobalOffsetY: 0,
			HiddenGlobalOffsetZ: 0,
			HiddenGridDims:      1,
		}
		b.rt.EnqueueLaunchKernel(b.Queue, b.kernel1,
			globalSize, localSize, &kernel1Arg)
	} else {
		kernel1Arg := Kernel1Args{
			A:  b.DA,
			P:  b.DP,
			Q:  b.DQ,
			NX: int32(b.NX),
			NY: int32(b.NY),
		}
		b.rt.EnqueueLaunchKernel(b.Queue, b.kernel1,
			globalSize, localSize, &kernel1Arg)
	}
}

func (b *Benchmark) launchKernel2(localSize [3]uint16, globalSize [3]uint32, globalSizeX uint32) {
	if b.Arch == arch.CDNA3 {
		kernel2Arg := CDNA3Kernel2Args{
			A:                   b.DA,
			R:                   b.DR,
			S:                   b.DS,
			NX:                  int32(b.NX),
			NY:                  int32(b.NY),
			HiddenBlockCountX:   globalSizeX / uint32(localSize[0]),
			HiddenBlockCountY:   1,
			HiddenBlockCountZ:   1,
			HiddenGroupSizeX:    localSize[0],
			HiddenGroupSizeY:    localSize[1],
			HiddenGroupSizeZ:    localSize[2],
			HiddenRemainderX:    uint16(globalSizeX % uint32(localSize[0])),
			HiddenRemainderY:    0,
			HiddenRemainderZ:    0,
			HiddenGlobalOffsetX: 0,
			HiddenGlobalOffsetY: 0,
			HiddenGlobalOffsetZ: 0,
			HiddenGridDims:      1,
		}
		b.rt.EnqueueLaunchKernel(b.Queue, b.kernel2,
			globalSize, localSize, &kernel2Arg)
	} else {
		kernel2Arg := Kernel2Args{
			A:  b.DA,
			R:  b.DR,
			S:  b.DS,
			NX: int32(b.NX),
			NY: int32(b.NY),
		}
		b.rt.EnqueueLaunchKernel(b.Queue, b.kernel2,
			globalSize, localSize, &kernel2Arg)
	}
}

func (b *Benchmark) exec() {
	b.Upload(b.rt, b.Queue, b.DA, b.A)
	b.Upload(b.rt, b.Queue, b.DR, b.R)
	b.Upload(b.rt, b.Queue, b.DP, b.P)

	localSize := [3]uint16{256, 1, 1}
	globalSizeX := uint32(((b.NX-1)/256 + 1) * 256)
	globalSize := [3]uint32{globalSizeX, 1, 1}

	b.launchKernel1(localSize, globalSize, globalSizeX)

	globalSizeX = uint32(((b.NY-1)/256 + 1) * 256)
	globalSize = [3]uint32{globalSizeX, 1, 1}

	b.launchKernel2(localSize, globalSize, globalSizeX)

	b.Readbacks = append(b.Readbacks, benchmarks.Readback{Source: b.DS, Target: 0})
	b.Readbacks = append(b.Readbacks, benchmarks.Readback{Source: b.DQ, Target: 1})
}

// Verify verifies
func (b *Benchmark) verify() error {
	b.cpuBicg()

	for i := 0; i < b.NY; i++ {
		if b.CpuS[i] != b.SOutput[i] {
			return fmt.Errorf("Mismatch in s at %d, expected %f, but get %f",
				i, b.CpuS[i], b.SOutput[i])
		}
	}

	for i := 0; i < b.NX; i++ {
		if b.CpuQ[i] != b.QOutput[i] {
			return fmt.Errorf("Mismatch in q at %d, expected %f, but get %f",
				i, b.CpuQ[i], b.QOutput[i])
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) cpuBicg() {
	b.CpuS = make([]float32, b.NY)
	b.CpuQ = make([]float32, b.NX)

	for i := 0; i < b.NX; i++ {
		for j := 0; j < b.NY; j++ {
			b.CpuS[j] += b.R[i] * b.A[i*b.NY+j]
			b.CpuQ[i] += b.P[j] * b.A[i*b.NY+j]
		}
	}
}

func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if err := b.ReleaseUploads(b.rt); err != nil {
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
				dst = b.SOutput
			case 1:
				dst = b.QOutput
			}
			b.Readbacks[i].Buffer = benchmarks.Download(b.rt, b.Queue, b.Readbacks[i].Source, binary.Size(dst))
		}
		b.Phase = 3
	case 3:
		for _, copy := range b.Readbacks {
			var dst any
			switch copy.Target {
			case 0:
				dst = b.SOutput
			case 1:
				dst = b.QOutput
			}
			if err := benchmarks.Read(b.rt, copy.Buffer, dst); err != nil {
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
