// Package fp16throughput implements the fp16_throughput microbenchmark,
// ported from sarchlab/gpu_benchmarks (tier1/fp16_throughput) for the
// MGPUSim MI300A (CDNA3 / gfx942) model.
//
// Each work-item runs a long chain of packed half2 fused-multiply-add (FMA)
// operations entirely in registers. Only work-item (0,0) of work-group 0
// writes the accumulated half2 result to a single output element, which the
// benchmark reads back and verifies against a CPU reference.
//
// The kernel binary is compiled for gfx942 only (see native/), so the
// benchmark must be run with `-arch cdna3` (the MI300A configuration).
package fp16throughput

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout is verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 12): one 8-byte global_buffer pointer followed by
// one 4-byte by_value scalar, packed with no padding (mgpusim serializes
// args with binary.Write, which does not insert alignment padding). The
// kernel reads only blockIdx.x / threadIdx.x (no blockDim), so no hidden
// ABI arguments are emitted.
type KernelArgs struct {
	Out           driver.Ptr // offset 0
	FmasPerThread int32      // offset 8
}

// Benchmark defines the fp16_throughput benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch arch.Type

	// FmasPerThread is the number of __hfma2 FMAs each thread performs
	// (rounded down to a multiple of 4). NumBlocks and ThreadsPerBlock
	// define the launch geometry.
	FmasPerThread   int
	NumBlocks       int
	ThreadsPerBlock int

	gOut driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new fp16_throughput benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

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
	b.gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// Run runs the benchmark.
func (b *Benchmark) Run() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the fp16_throughput benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
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

	// Output is a single __half2 (4 bytes). Allocate a full dword.
	if b.useUnifiedMemory {
		b.gOut = b.driver.AllocateUnifiedMemory(b.context, 4)
	} else {
		b.gOut = b.driver.AllocateMemory(b.context, 4)
	}

	// Initialize to zero so a missing write is detectable.
	zero := []uint32{0}
	b.driver.MemCopyH2D(b.context, b.gOut, zero)
}

func (b *Benchmark) exec() {
	globalX := uint32(b.NumBlocks * b.ThreadsPerBlock)

	args := KernelArgs{
		Out:           b.gOut,
		FmasPerThread: int32(b.FmasPerThread),
	}

	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsaco,
		[3]uint32{globalX, 1, 1},
		[3]uint16{uint16(b.ThreadsPerBlock), 1, 1},
		&args,
	)

	b.driver.DrainCommandQueue(b.queue)
}

// half2ToFloats unpacks a packed __half2 dword into its two fp32 lane values.
func half2ToFloats(packed uint32) (float32, float32) {
	lo := float16ToFloat32(uint16(packed & 0xFFFF))
	hi := float16ToFloat32(uint16((packed >> 16) & 0xFFFF))
	return lo, hi
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

// Verify checks the GPU result against a CPU reference computation.
//
// The reference reproduces work-item (0,0): base = 1.0, so a0..a3 start at
// 1.0, 1.1, 1.2, 1.3. The multiplier (1.0000001) rounds to exactly 1.0 in
// fp16 and the addend (0.0000001) rounds to 0.0, so each __hfma2 leaves the
// accumulators unchanged. The written value is a0+a1+a2+a3 per lane.
func (b *Benchmark) Verify() {
	out := make([]uint32, 1)
	b.driver.MemCopyD2H(b.context, out, b.gOut)

	lo, hi := half2ToFloats(out[0])

	// CPU reference, evaluated in fp16-rounded arithmetic.
	const base = float32(1.0)
	a0 := roundToHalf(base)
	a1 := roundToHalf(base + 0.1)
	a2 := roundToHalf(base + 0.2)
	a3 := roundToHalf(base + 0.3)

	mul := roundToHalf(1.0000001)
	add := roundToHalf(0.0000001)
	for i := 0; i < b.FmasPerThread; i += 4 {
		a0 = roundToHalf(roundToHalf(a0*mul) + add)
		a1 = roundToHalf(roundToHalf(a1*mul) + add)
		a2 = roundToHalf(roundToHalf(a2*mul) + add)
		a3 = roundToHalf(roundToHalf(a3*mul) + add)
	}
	ref := roundToHalf(roundToHalf(a0+a1) + roundToHalf(a2+a3))

	checkLane := func(name string, got float32) {
		denom := math.Abs(float64(ref))
		if denom < 1.0 {
			denom = 1.0
		}
		if math.Abs(float64(ref-got))/denom > 1e-2 {
			log.Fatalf("Mismatch in %s lane: expected %f, but got %f.\n",
				name, ref, got)
		}
	}
	checkLane("lo", lo)
	checkLane("hi", hi)

	log.Printf("Passed!\n")
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
