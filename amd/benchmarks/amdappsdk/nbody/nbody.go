// Package nbody include the benchmark of NBody sample Derived from SDKSample base class
package nbody

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

// KernelArgs defines kernel arguments
type KernelArgs struct {
	Pos                 gpuruntime.Ptr
	Vel                 gpuruntime.Ptr
	NumBodies           int32
	DeltaTime           float32
	EpsSqr              float32
	LocalPos            gpuruntime.LocalPtr
	NewPosition         gpuruntime.Ptr
	NewVelocity         gpuruntime.Ptr
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// CDNA3KernelArgs defines kernel arguments for CDNA3 architecture (GFX942)
type CDNA3KernelArgs struct {
	Pos                 gpuruntime.Ptr
	Vel                 gpuruntime.Ptr
	NumBodies           int32
	DeltaTime           float32
	EpsSqr              float32
	Padding1            uint32
	LocalPos            gpuruntime.Ptr
	NewPosition         gpuruntime.Ptr
	NewVelocity         gpuruntime.Ptr
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
	benchmarks.Checkpoint[checkpointConfig, State]
	rt *gpuruntime.Runtime

	nbodyKernel *insts.KernelCodeObject
}

// NewBenchmark returns a benchmark
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.checkpointConfig, b.bind)

	b.rt = rt
	b.Context = rt.Init()
	b.GroupSize = 256
	b.DelT = 0.005
	b.EspSqr = 500.0
	b.Exchange = true

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

//go:embed nbody.hsaco
var gcn3HSACOBytes []byte

//go:embed nbody_gfx942.hsaco
var cdna3HSACOBytes []byte

func (b *Benchmark) loadProgram() {
	var hsacoBytes []byte
	if b.Arch == arch.CDNA3 {
		hsacoBytes = cdna3HSACOBytes
	} else {
		hsacoBytes = gcn3HSACOBytes
	}

	b.nbodyKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "nbody_sim")
	if b.nbodyKernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) setup() {
	b.rt.SelectGPU(b.Context, b.Gpus[len(b.Gpus)-1])
	b.Queue = b.rt.CreateCommandQueue(b.Context)

	if b.NumParticles < b.GroupSize {
		b.NumParticles = b.GroupSize
	}
	b.NumParticles = (b.NumParticles / b.GroupSize) * b.GroupSize
	b.NumBodies = b.NumParticles

	b.loadProgram()
	b.initMem()
}

func (b *Benchmark) initMem() {
	b.InitPos = make([]float32, b.NumBodies*4)
	b.InitVel = make([]float32, b.NumBodies*4)
	b.Pos = make([]float32, b.NumBodies*4)
	b.Vel = make([]float32, b.NumBodies*4)

	b.fill()

	if b.UseUnifiedMemory {
		b.CurrPos = b.rt.AllocateUnifiedMemory(b.Context,
			uint64(b.NumBodies*4*4))
		b.NewPos = b.rt.AllocateUnifiedMemory(b.Context,
			uint64(b.NumBodies*4*4))
		b.CurrVel = b.rt.AllocateUnifiedMemory(b.Context,
			uint64(b.NumBodies*4*4))
		b.NewVel = b.rt.AllocateUnifiedMemory(b.Context,
			uint64(b.NumBodies*4*4))
	} else {
		b.CurrPos = b.rt.AllocateMemory(b.Context,
			uint64(b.NumBodies*4*4))
		b.NewPos = b.rt.AllocateMemory(b.Context,
			uint64(b.NumBodies*4*4))
		b.CurrVel = b.rt.AllocateMemory(b.Context,
			uint64(b.NumBodies*4*4))
		b.NewVel = b.rt.AllocateMemory(b.Context,
			uint64(b.NumBodies*4*4))
	}
	b.Upload(b.rt, b.Queue, b.CurrPos, b.Pos)
	b.Upload(b.rt, b.Queue, b.CurrVel, b.Vel)

	b.DPos = b.CurrPos
	b.DVel = b.CurrVel
	b.DNewPos = b.NewPos
	b.DNewVel = b.NewVel

	if b.Arch == arch.CDNA3 {
		// CDNA3 HIP kernel uses global memory for localPos instead of LDS
		b.LocalPosBuf = b.rt.AllocateMemory(b.Context,
			uint64(b.GroupSize*4*4))
	}
}

func (b *Benchmark) exec() {
	globalSize := [3]uint32{uint32(b.NumBodies), 1, 1}
	localSize := [3]uint16{uint16(b.GroupSize), 1, 1}

	for i := int32(0); i < b.NumIterations; i++ {
		if b.Arch == arch.CDNA3 {
			args := CDNA3KernelArgs{
				Pos:                 b.DPos,
				Vel:                 b.DVel,
				NumBodies:           b.NumBodies,
				DeltaTime:           b.DelT,
				EpsSqr:              b.EspSqr,
				LocalPos:            b.LocalPosBuf,
				NewPosition:         b.DNewPos,
				NewVelocity:         b.DNewVel,
				HiddenBlockCountX:   globalSize[0] / uint32(localSize[0]),
				HiddenBlockCountY:   1,
				HiddenBlockCountZ:   1,
				HiddenGroupSizeX:    localSize[0],
				HiddenGroupSizeY:    1,
				HiddenGroupSizeZ:    1,
				HiddenRemainderX:    uint16(globalSize[0] % uint32(localSize[0])),
				HiddenRemainderY:    0,
				HiddenRemainderZ:    0,
				HiddenGlobalOffsetX: 0,
				HiddenGlobalOffsetY: 0,
				HiddenGlobalOffsetZ: 0,
				HiddenGridDims:      1,
			}
			b.rt.EnqueueLaunchKernel(b.Queue,
				b.nbodyKernel,
				globalSize, localSize,
				&args,
			)
		} else {
			args := KernelArgs{
				Pos:                 b.DPos,
				Vel:                 b.DVel,
				NumBodies:           b.NumBodies,
				DeltaTime:           b.DelT,
				EpsSqr:              b.EspSqr,
				LocalPos:            gpuruntime.LocalPtr(b.GroupSize * 4 * 4),
				NewPosition:         b.DNewPos,
				NewVelocity:         b.DNewVel,
				HiddenGlobalOffsetX: 0,
				HiddenGlobalOffsetY: 0,
				HiddenGlobalOffsetZ: 0,
			}
			b.rt.EnqueueLaunchKernel(b.Queue,
				b.nbodyKernel,
				globalSize, localSize,
				&args,
			)
		}

		b.DPos, b.DNewPos = b.DNewPos, b.DPos
		b.DVel, b.DNewVel = b.DNewVel, b.DVel
	}

	b.Readbacks = append(b.Readbacks, benchmarks.Readback{Source: b.DPos, Target: 0})
}

// Verify verifies
func (b *Benchmark) verify() error {
	b.RefPos = make([]float32, b.NumBodies*4)
	b.RefVel = make([]float32, b.NumBodies*4)
	copy(b.RefPos, b.InitPos)
	copy(b.RefVel, b.InitVel)

	for i := int32(0); i < b.NumIterations; i++ {
		b.nbodyCPU()
	}

	mismatch := false
	for i := int32(0); i < (b.NumBodies * 4); i++ {
		if math.Abs(float64(b.RefPos[i]-b.Pos[i])) > 0.001 {
			mismatch = true
			log.Printf("not match at (%d), expected %g to equal %g\n",
				i,
				b.Pos[i], b.RefPos[i])
		}
	}

	if mismatch {
		return fmt.Errorf("Mismatch!\n")
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) nbodyCPU() {
	oldPos := make([]float32, len(b.RefPos))
	copy(oldPos, b.RefPos)

	for i := int32(0); i < b.NumBodies; i++ {
		myIndex := 4 * i
		acc := [3]float32{0.0, 0.0, 0.0}
		for j := int32(0); j < b.NumBodies; j++ {
			r := [3]float32{0.0, 0.0, 0.0}
			index := 4 * j

			distSqr := float32(0.0)
			for k := int32(0); k < 3; k++ {
				r[k] = oldPos[index+k] - oldPos[myIndex+k]
				distSqr += r[k] * r[k]
			}

			invDist := 1.0 / float32(math.Sqrt(float64(distSqr+b.EspSqr)))
			invDistCube := invDist * invDist * invDist
			s := oldPos[index+3] * invDistCube

			for k := int32(0); k < 3; k++ {
				acc[k] += s * r[k]
			}
		}

		for k := int32(0); k < 3; k++ {
			b.RefPos[myIndex+k] +=
				b.RefVel[myIndex+k]*b.DelT +
					0.5*acc[k]*b.DelT*b.DelT
			b.RefVel[myIndex+k] += acc[k] * b.DelT
		}
	}
}

func random(rng *rand.Rand, randMax float32, randMin float32) float32 {
	result := rng.Float32()
	result = ((1.0-result)*randMin + result*randMax)
	return result
}

func (b *Benchmark) fill() {
	// Use a local random source so the input is reproducible. The global
	// generator is seeded randomly at startup (rand.Seed is a no-op since
	// Go 1.24), which made this benchmark's input differ on every run.
	rng := rand.New(rand.NewSource(1))
	for i := int32(0); i < b.NumBodies; i++ {
		index := 4 * i

		for j := int32(0); j < 3; j++ {
			b.InitPos[index+j] = random(rng, 3, 50)
		}
		b.InitPos[index+3] = random(rng, 1, 1000)

		for j := int32(0); j < 3; j++ {
			b.InitVel[index+j] = 0.0
		}

		b.InitVel[3] = 0.0 // unused
	}

	copy(b.Pos, b.InitPos)
	copy(b.Vel, b.InitVel)
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
				dst = b.Pos
			}
			b.Readbacks[i].Buffer = benchmarks.Download(b.rt, b.Queue, b.Readbacks[i].Source, binary.Size(dst))
		}
		b.Phase = 3
	case 3:
		for _, copy := range b.Readbacks {
			var dst any
			switch copy.Target {
			case 0:
				dst = b.Pos
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
