// Package fp16throughput implements the GPU Benchmarks v1.1 packed FP16
// throughput workload for MI300X (gfx942).
package fp16throughput

import (
	// embed hsaco files
	_ "embed"
	"fmt"
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// KernelArgs is the device ABI for the version 1.1 HIP kernel.
type KernelArgs struct {
	Out           gpuruntime.Ptr
	FmasPerThread int32
	_             [4]byte
	Launch        benchmarks.GFX942LaunchDimensions
}

// Benchmark defines the fp16_throughput benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new fp16_throughput benchmark.
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)

	b.rt = rt
	b.Context = b.rt.Init()

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "fp16_fma_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. This benchmark uses a single GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.Gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) submit() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the fp16_throughput benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
}

func (b *Benchmark) initParams() {
	if b.FmasPerThread <= 0 {
		b.FmasPerThread = 64
	}
	// Match the host: round down to a multiple of 4, with a floor of 4.
	b.FmasPerThread = (b.FmasPerThread / 4) * 4
	if b.FmasPerThread == 0 {
		b.FmasPerThread = 4
	}

	if b.NumBlocks <= 0 {
		b.NumBlocks = 2
	}
	if b.ThreadsPerBlock <= 0 {
		b.ThreadsPerBlock = 64
	}
}

func (b *Benchmark) initMem() {
	b.initParams()
	outBytes := uint64(b.NumBlocks * b.ThreadsPerBlock * 4)
	if b.UseUnifiedMemory {
		b.GOut = b.rt.AllocateUnifiedMemory(b.Context, outBytes)
	} else {
		b.GOut = b.rt.AllocateMemory(b.Context, outBytes)
	}
}

func (b *Benchmark) exec() {
	grid := [3]uint32{uint32(b.NumBlocks * b.ThreadsPerBlock), 1, 1}
	group := [3]uint16{uint16(b.ThreadsPerBlock), 1, 1}
	args := KernelArgs{
		Out: b.GOut, FmasPerThread: int32(b.FmasPerThread),
		Launch: benchmarks.NewGFX942LaunchDimensions(grid, group),
	}
	b.rt.EnqueueLaunchKernel(b.Queue, b.hsaco, grid, group, &args)
}

// float16ToFloat32 converts an IEEE-754 half-precision value to float32.
func float16ToFloat32(h uint16) float32 {
	sign := uint32(h&0x8000) << 16
	exp := uint32(h>>10) & 0x1F
	mant := uint32(h & 0x03FF)

	switch {
	case exp == 0 && mant == 0:
		return math.Float32frombits(sign)
	case exp == 0x1F:
		// Inf / NaN
		return math.Float32frombits(sign | 0x7F800000 | (mant << 13))
	case exp == 0:
		// Subnormal half: normalize.
		e := -1
		m := mant
		for (m & 0x0400) == 0 {
			m <<= 1
			e--
		}
		m &= 0x03FF
		exp32 := uint32(127-15+e+1) << 23
		return math.Float32frombits(sign | exp32 | (m << 13))
	default:
		exp32 := (exp + (127 - 15)) << 23
		return math.Float32frombits(sign | exp32 | (mant << 13))
	}
}

func (b *Benchmark) verify() error {
	out := make([]uint32, b.NumBlocks*b.ThreadsPerBlock)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], out); err != nil {
		return err
	}

	reference := make([]uint32, b.ThreadsPerBlock)
	for lane := range reference {
		base := float32(1) + float32(lane)*0.001
		a0 := roundToHalf(base)
		a1 := roundToHalf(base + 0.1)
		a2 := roundToHalf(base + 0.2)
		a3 := roundToHalf(base + 0.3)
		const mul, add = 0.9990234375, 0.0009765625
		for i := 0; i < b.FmasPerThread; i += 4 {
			a0 = roundToHalf(a0*mul + add)
			a1 = roundToHalf(a1*mul + add)
			a2 = roundToHalf(a2*mul + add)
			a3 = roundToHalf(a3*mul + add)
		}
		sum := float32ToFloat16(roundToHalf(a0+a1) + roundToHalf(a2+a3))
		reference[lane] = uint32(sum) | uint32(sum)<<16
	}
	for tid, got := range out {
		ref := reference[tid%b.ThreadsPerBlock]
		if got != ref {
			return fmt.Errorf("at work-item %d: expected packed half2 %#08x, got %#08x", tid, ref, got)
		}
	}

	log.Printf("Passed!\n")
	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GOut, 4*b.NumBlocks*b.ThreadsPerBlock))
}

// roundToHalf rounds a float32 value to the nearest representable fp16 value
// (round-to-nearest-even) and returns it back as a float32, modeling the
// precision of the GPU's half-precision arithmetic.
func roundToHalf(f float32) float32 {
	return float16ToFloat32(float32ToFloat16(f))
}

// float32ToFloat16 converts a float32 to IEEE-754 half precision with
// round-to-nearest-even.
func float32ToFloat16(f float32) uint16 {
	bits := math.Float32bits(f)
	sign := uint16((bits >> 16) & 0x8000)
	exp := int32((bits>>23)&0xFF) - 127 + 15
	mant := bits & 0x7FFFFF

	if (bits & 0x7FFFFFFF) == 0 {
		return sign
	}
	if ((bits >> 23) & 0xFF) == 0xFF {
		// Inf / NaN
		if mant != 0 {
			return sign | 0x7E00 // NaN
		}
		return sign | 0x7C00 // Inf
	}

	if exp >= 0x1F {
		return sign | 0x7C00 // overflow -> Inf
	}
	if exp <= 0 {
		if exp < -10 {
			return sign // underflow to zero
		}
		// Subnormal half.
		mant |= 0x800000
		shift := uint32(14 - exp)
		half := mant >> shift
		// round-to-nearest-even
		rem := mant & ((1 << shift) - 1)
		halfway := uint32(1) << (shift - 1)
		if rem > halfway || (rem == halfway && (half&1) == 1) {
			half++
		}
		return sign | uint16(half)
	}

	half := uint16(exp<<10) | uint16(mant>>13)
	rem := mant & 0x1FFF
	if rem > 0x1000 || (rem == 0x1000 && (half&1) == 1) {
		half++
	}
	return sign | half
}

// Step submits computation, then verification readback, then consumes results.
func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if err := b.ReleaseUploads(b.rt); err != nil {
		return err
	}

	switch b.Phase {
	case 0:
		b.submit()
		b.Phase = 1
	case 1:
		if !b.Verification {
			b.Ended = true
			return nil
		}
		b.enqueueVerification()
		b.Phase = 2
	case 2:
		if err := b.verify(); err != nil {
			return err
		}
		b.Readbacks = nil
		b.Ended = true
	}
	return nil
}
