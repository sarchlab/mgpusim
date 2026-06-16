// Package backprop implements the Rodinia Backpropagation benchmark, ported
// from sarchlab/gpu_benchmarks (tier2/rodinia_backprop) for the MGPUSim MI300A
// (CDNA3 / gfx942) model.
//
// It runs one forward + backward training epoch over a two-layer
// fully-connected network (INPUT_N -> HIDDEN_N -> OUTPUT_N), exercising six
// kernels: forward_hidden, forward_output, backward_output_delta,
// backward_hidden_delta, update_w1 (2D grid) and update_w2.  The kernel binary
// is compiled for gfx942 only (see native/), so the benchmark must be run with
// `-arch cdna3` (the MI300A configuration).
package backprop

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

const (
	blockSz = 64 // 1D block width (matches BLOCK_SZ in the kernel)
	tile2D  = 16 // 2D block dim for update_w1 (matches TILE2D in the kernel)
	lr      = float32(0.1)
)

// fwHiddenArgs matches forward_hidden (kernarg_segment_size = 40).
type fwHiddenArgs struct {
	Input   driver.Ptr // offset 0
	W1      driver.Ptr // offset 8
	B1      driver.Ptr // offset 16
	Hidden  driver.Ptr // offset 24
	InputN  int32      // offset 32
	HiddenN int32      // offset 36
}

// fwOutputArgs matches forward_output (kernarg_segment_size = 40).
type fwOutputArgs struct {
	Hidden  driver.Ptr // offset 0
	W2      driver.Ptr // offset 8
	B2      driver.Ptr // offset 16
	Output  driver.Ptr // offset 24
	HiddenN int32      // offset 32
	OutputN int32      // offset 36
}

// bwOutputDeltaArgs matches backward_output_delta (kernarg_segment_size = 28).
type bwOutputDeltaArgs struct {
	Output   driver.Ptr // offset 0
	Target   driver.Ptr // offset 8
	DeltaOut driver.Ptr // offset 16
	OutputN  int32      // offset 24
}

// bwHiddenDeltaArgs matches backward_hidden_delta (kernarg_segment_size = 40).
type bwHiddenDeltaArgs struct {
	Hidden   driver.Ptr // offset 0
	W2       driver.Ptr // offset 8
	DeltaOut driver.Ptr // offset 16
	DeltaHid driver.Ptr // offset 24
	HiddenN  int32      // offset 32
	OutputN  int32      // offset 36
}

// updateW1Args matches update_w1 (kernarg_segment_size = 36).
type updateW1Args struct {
	W1       driver.Ptr // offset 0
	Input    driver.Ptr // offset 8
	DeltaHid driver.Ptr // offset 16
	InputN   int32      // offset 24
	HiddenN  int32      // offset 28
	LR       float32    // offset 32
}

// updateW2Args matches update_w2 (kernarg_segment_size = 36).
type updateW2Args struct {
	W2       driver.Ptr // offset 0
	Hidden   driver.Ptr // offset 8
	DeltaOut driver.Ptr // offset 16
	HiddenN  int32      // offset 24
	OutputN  int32      // offset 28
	LR       float32    // offset 32
}

// Benchmark defines the Backprop benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	gpus    []int

	kForwardHidden       *insts.KernelCodeObject
	kForwardOutput       *insts.KernelCodeObject
	kBackwardOutputDelta *insts.KernelCodeObject
	kBackwardHiddenDelta *insts.KernelCodeObject
	kUpdateW1            *insts.KernelCodeObject
	kUpdateW2            *insts.KernelCodeObject

	Arch    arch.Type
	InputN  int
	HiddenN int
	OutputN int

	// Host-side reference inputs (deterministic, reproduced in Verify).
	hInput  []float32
	hW1     []float32
	hW2     []float32
	hB1     []float32
	hB2     []float32
	hTarget []float32

	gInput    driver.Ptr
	gW1       driver.Ptr
	gB1       driver.Ptr
	gHidden   driver.Ptr
	gW2       driver.Ptr
	gB2       driver.Ptr
	gOutput   driver.Ptr
	gTarget   driver.Ptr
	gDeltaOut driver.Ptr
	gDeltaHid driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new Backprop benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	load := func(name string) *insts.KernelCodeObject {
		k := insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, name)
		if k == nil {
			log.Panicf("Failed to load kernel %q from binary", name)
		}
		return k
	}

	b.kForwardHidden = load("forward_hidden")
	b.kForwardOutput = load("forward_output")
	b.kBackwardOutputDelta = load("backward_output_delta")
	b.kBackwardHiddenDelta = load("backward_hidden_delta")
	b.kUpdateW1 = load("update_w1")
	b.kUpdateW2 = load("update_w2")
}

// SelectGPU selects the GPUs to run on. Backprop uses a single GPU.
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
		log.Panic("the rodinia backprop benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

// pseudoRand reproduces a simple deterministic LCG so the host init can be
// recomputed exactly in Verify() (independent of the C rand()).
func pseudoRand(seed *uint32) float32 {
	*seed = (*seed)*1664525 + 1013904223
	// take top 24 bits as a [0,1) fraction
	return float32(*seed>>8) / float32(1<<24)
}

func (b *Benchmark) initMem() { //nolint:funlen,gocognit
	if b.InputN <= 0 {
		b.InputN = 64
	}
	if b.HiddenN <= 0 {
		b.HiddenN = 32
	}
	if b.OutputN <= 0 {
		b.OutputN = 4
	}

	in := b.InputN
	hid := b.HiddenN
	out := b.OutputN

	b.hInput = make([]float32, in)
	b.hW1 = make([]float32, in*hid)
	b.hW2 = make([]float32, hid*out)
	b.hB1 = make([]float32, hid)
	b.hB2 = make([]float32, out)
	b.hTarget = make([]float32, out)

	for i := 0; i < in; i++ {
		b.hInput[i] = float32(i%256) / 256.0
	}
	var seed uint32 = 42
	for i := range b.hW1 {
		b.hW1[i] = (pseudoRand(&seed) - 0.5) * 0.2 // [-0.1, 0.1]
	}
	for i := range b.hW2 {
		b.hW2[i] = (pseudoRand(&seed) - 0.5) * 0.2
	}
	for i := range b.hB1 {
		b.hB1[i] = 0.0
	}
	for i := range b.hB2 {
		b.hB2[i] = 0.0
	}
	for i := range b.hTarget {
		b.hTarget[i] = 1.0
	}

	alloc := func(n int) driver.Ptr {
		if b.useUnifiedMemory {
			return b.driver.AllocateUnifiedMemory(b.context, uint64(n*4))
		}
		return b.driver.AllocateMemory(b.context, uint64(n*4))
	}

	b.gInput = alloc(in)
	b.gW1 = alloc(in * hid)
	b.gB1 = alloc(hid)
	b.gHidden = alloc(hid)
	b.gW2 = alloc(hid * out)
	b.gB2 = alloc(out)
	b.gOutput = alloc(out)
	b.gTarget = alloc(out)
	b.gDeltaOut = alloc(out)
	b.gDeltaHid = alloc(hid)

	b.driver.MemCopyH2D(b.context, b.gInput, b.hInput)
	b.driver.MemCopyH2D(b.context, b.gW1, b.hW1)
	b.driver.MemCopyH2D(b.context, b.gB1, b.hB1)
	b.driver.MemCopyH2D(b.context, b.gW2, b.hW2)
	b.driver.MemCopyH2D(b.context, b.gB2, b.hB2)
	b.driver.MemCopyH2D(b.context, b.gTarget, b.hTarget)
}

func grid1D(n int) uint32 {
	return uint32((n + blockSz - 1) / blockSz * blockSz)
}

func (b *Benchmark) exec() { //nolint:funlen,gocognit
	in := b.InputN
	hid := b.HiddenN
	out := b.OutputN

	// 1. forward_hidden  (one thread per hidden unit)
	b.driver.EnqueueLaunchKernel(
		b.queue, b.kForwardHidden,
		[3]uint32{grid1D(hid), 1, 1},
		[3]uint16{blockSz, 1, 1},
		&fwHiddenArgs{
			Input: b.gInput, W1: b.gW1, B1: b.gB1, Hidden: b.gHidden,
			InputN: int32(in), HiddenN: int32(hid),
		},
	)

	// 2. forward_output  (one thread per output unit)
	b.driver.EnqueueLaunchKernel(
		b.queue, b.kForwardOutput,
		[3]uint32{grid1D(out), 1, 1},
		[3]uint16{blockSz, 1, 1},
		&fwOutputArgs{
			Hidden: b.gHidden, W2: b.gW2, B2: b.gB2, Output: b.gOutput,
			HiddenN: int32(hid), OutputN: int32(out),
		},
	)

	// 3. backward_output_delta
	b.driver.EnqueueLaunchKernel(
		b.queue, b.kBackwardOutputDelta,
		[3]uint32{grid1D(out), 1, 1},
		[3]uint16{blockSz, 1, 1},
		&bwOutputDeltaArgs{
			Output: b.gOutput, Target: b.gTarget, DeltaOut: b.gDeltaOut,
			OutputN: int32(out),
		},
	)

	// 4. backward_hidden_delta
	b.driver.EnqueueLaunchKernel(
		b.queue, b.kBackwardHiddenDelta,
		[3]uint32{grid1D(hid), 1, 1},
		[3]uint16{blockSz, 1, 1},
		&bwHiddenDeltaArgs{
			Hidden: b.gHidden, W2: b.gW2, DeltaOut: b.gDeltaOut,
			DeltaHid: b.gDeltaHid,
			HiddenN:  int32(hid), OutputN: int32(out),
		},
	)

	// 5. update_w1  (2D grid: x->input i, y->hidden j)
	gx := uint32((in + tile2D - 1) / tile2D * tile2D)
	gy := uint32((hid + tile2D - 1) / tile2D * tile2D)
	b.driver.EnqueueLaunchKernel(
		b.queue, b.kUpdateW1,
		[3]uint32{gx, gy, 1},
		[3]uint16{tile2D, tile2D, 1},
		&updateW1Args{
			W1: b.gW1, Input: b.gInput, DeltaHid: b.gDeltaHid,
			InputN: int32(in), HiddenN: int32(hid), LR: lr,
		},
	)

	// 6. update_w2
	b.driver.EnqueueLaunchKernel(
		b.queue, b.kUpdateW2,
		[3]uint32{grid1D(hid), 1, 1},
		[3]uint16{blockSz, 1, 1},
		&updateW2Args{
			W2: b.gW2, Hidden: b.gHidden, DeltaOut: b.gDeltaOut,
			HiddenN: int32(hid), OutputN: int32(out), LR: lr,
		},
	)

	b.driver.DrainCommandQueue(b.queue)
}

func sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

// Verify recomputes the full epoch on the CPU and compares the updated weight
// matrices (w1, w2) against the GPU results.
func (b *Benchmark) Verify() { //nolint:funlen,gocognit
	in := b.InputN
	hid := b.HiddenN
	out := b.OutputN

	// CPU reference, all in float64 then compared with tolerance.
	hidden := make([]float64, hid)
	output := make([]float64, out)
	deltaOut := make([]float64, out)
	deltaHid := make([]float64, hid)

	// forward_hidden
	for j := 0; j < hid; j++ {
		sum := float64(b.hB1[j])
		for i := 0; i < in; i++ {
			sum += float64(b.hInput[i]) * float64(b.hW1[i*hid+j])
		}
		hidden[j] = sigmoid(sum)
	}
	// forward_output
	for k := 0; k < out; k++ {
		sum := float64(b.hB2[k])
		for j := 0; j < hid; j++ {
			sum += hidden[j] * float64(b.hW2[j*out+k])
		}
		output[k] = sigmoid(sum)
	}
	// backward_output_delta
	for k := 0; k < out; k++ {
		o := output[k]
		deltaOut[k] = o * (1.0 - o) * (float64(b.hTarget[k]) - o)
	}
	// backward_hidden_delta
	for j := 0; j < hid; j++ {
		sum := 0.0
		for k := 0; k < out; k++ {
			sum += float64(b.hW2[j*out+k]) * deltaOut[k]
		}
		deltaHid[j] = hidden[j] * (1.0 - hidden[j]) * sum
	}
	// update_w1
	refW1 := make([]float64, in*hid)
	for i := 0; i < in; i++ {
		for j := 0; j < hid; j++ {
			refW1[i*hid+j] = float64(b.hW1[i*hid+j]) +
				float64(lr)*float64(b.hInput[i])*deltaHid[j]
		}
	}
	// update_w2
	refW2 := make([]float64, hid*out)
	for j := 0; j < hid; j++ {
		for k := 0; k < out; k++ {
			refW2[j*out+k] = float64(b.hW2[j*out+k]) +
				float64(lr)*hidden[j]*deltaOut[k]
		}
	}

	gpuW1 := make([]float32, in*hid)
	gpuW2 := make([]float32, hid*out)
	b.driver.MemCopyD2H(b.context, gpuW1, b.gW1)
	b.driver.MemCopyD2H(b.context, gpuW2, b.gW2)

	check := func(name string, idx int, ref float64, got float64) {
		denom := math.Abs(ref)
		if denom < 1e-3 {
			denom = 1e-3
		}
		if math.Abs(ref-got)/denom > 1e-3 {
			log.Fatalf("%s[%d]: expected %f, but got %f.\n",
				name, idx, ref, got)
		}
	}

	for i := range refW1 {
		check("w1", i, refW1[i], float64(gpuW1[i]))
	}
	for i := range refW2 {
		check("w2", i, refW2[i], float64(gpuW2[i]))
	}

	log.Printf("Passed!\n")
}
