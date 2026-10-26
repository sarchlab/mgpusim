// Package backprop implements the Rodinia Backpropagation benchmark, ported
// from sarchlab/gpu_benchmarks (tier2/rodinia_backprop) for the MGPUSim MI300X
// (CDNA3 / gfx942) model.
//
// It runs one forward + backward training epoch over a two-layer
// fully-connected network (INPUT_N -> HIDDEN_N -> OUTPUT_N), exercising six
// kernels: forward_hidden, forward_output, backward_output_delta,
// backward_hidden_delta, update_w1 (2D grid) and update_w2.  The kernel binary
// is compiled for gfx942 only (see native/), so the benchmark must be run with
// `-arch cdna3` (the MI300X configuration).
package backprop

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

const (
	blockSz = 256 // 1D block width (matches BLOCK_SZ in the kernel; HW uses 256)
	tile2D  = 16  // 2D block dim for update_w1 (matches TILE2D in the kernel)
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
	*State
	benchmarks.Saved[checkpointConfig, State]

	driver *driver.Runtime

	kForwardHidden       *insts.KernelCodeObject
	kForwardOutput       *insts.KernelCodeObject
	kBackwardOutputDelta *insts.KernelCodeObject
	kBackwardHiddenDelta *insts.KernelCodeObject
	kUpdateW1            *insts.KernelCodeObject
	kUpdateW2            *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new Backprop benchmark.
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Saved = benchmarks.NewSaved(b.State,
		b.checkpointConfig, b.bind)

	b.driver = driver
	b.Context = b.driver.Init()

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
	b.Gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) submit() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the rodinia backprop benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.driver.CreateCommandQueue(b.Context)
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

	b.HInput = make([]float32, in)
	b.HW1 = make([]float32, in*hid)
	b.HW2 = make([]float32, hid*out)
	b.HB1 = make([]float32, hid)
	b.HB2 = make([]float32, out)
	b.HTarget = make([]float32, out)

	for i := 0; i < in; i++ {
		b.HInput[i] = float32(i%256) / 256.0
	}
	var seed uint32 = 42
	for i := range b.HW1 {
		b.HW1[i] = (pseudoRand(&seed) - 0.5) * 0.2 // [-0.1, 0.1]
	}
	for i := range b.HW2 {
		b.HW2[i] = (pseudoRand(&seed) - 0.5) * 0.2
	}
	for i := range b.HB1 {
		b.HB1[i] = 0.0
	}
	for i := range b.HB2 {
		b.HB2[i] = 0.0
	}
	for i := range b.HTarget {
		b.HTarget[i] = 1.0
	}

	alloc := func(n int) driver.Ptr {
		if b.UseUnifiedMemory {
			return b.driver.AllocateUnifiedMemory(b.Context, uint64(n*4))
		}
		return b.driver.AllocateMemory(b.Context, uint64(n*4))
	}

	b.GInput = alloc(in)
	b.GW1 = alloc(in * hid)
	b.GB1 = alloc(hid)
	b.GHidden = alloc(hid)
	b.GW2 = alloc(hid * out)
	b.GB2 = alloc(out)
	b.GOutput = alloc(out)
	b.GTarget = alloc(out)
	b.GDeltaOut = alloc(out)
	b.GDeltaHid = alloc(hid)

	b.driver.EnqueueMemCopyH2D(b.Queue, b.GInput, b.HInput)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GW1, b.HW1)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GB1, b.HB1)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GW2, b.HW2)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GB2, b.HB2)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GTarget, b.HTarget)
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
		b.Queue, b.kForwardHidden,
		[3]uint32{grid1D(hid), 1, 1},
		[3]uint16{blockSz, 1, 1},
		&fwHiddenArgs{
			Input: b.GInput, W1: b.GW1, B1: b.GB1, Hidden: b.GHidden,
			InputN: int32(in), HiddenN: int32(hid),
		},
	)

	// 2. forward_output  (one thread per output unit)
	b.driver.EnqueueLaunchKernel(
		b.Queue, b.kForwardOutput,
		[3]uint32{grid1D(out), 1, 1},
		[3]uint16{blockSz, 1, 1},
		&fwOutputArgs{
			Hidden: b.GHidden, W2: b.GW2, B2: b.GB2, Output: b.GOutput,
			HiddenN: int32(hid), OutputN: int32(out),
		},
	)

	// 3. backward_output_delta
	b.driver.EnqueueLaunchKernel(
		b.Queue, b.kBackwardOutputDelta,
		[3]uint32{grid1D(out), 1, 1},
		[3]uint16{blockSz, 1, 1},
		&bwOutputDeltaArgs{
			Output: b.GOutput, Target: b.GTarget, DeltaOut: b.GDeltaOut,
			OutputN: int32(out),
		},
	)

	// 4. backward_hidden_delta
	b.driver.EnqueueLaunchKernel(
		b.Queue, b.kBackwardHiddenDelta,
		[3]uint32{grid1D(hid), 1, 1},
		[3]uint16{blockSz, 1, 1},
		&bwHiddenDeltaArgs{
			Hidden: b.GHidden, W2: b.GW2, DeltaOut: b.GDeltaOut,
			DeltaHid: b.GDeltaHid,
			HiddenN:  int32(hid), OutputN: int32(out),
		},
	)

	// 5. update_w1  (2D grid: x->input i, y->hidden j)
	gx := uint32((in + tile2D - 1) / tile2D * tile2D)
	gy := uint32((hid + tile2D - 1) / tile2D * tile2D)
	b.driver.EnqueueLaunchKernel(
		b.Queue, b.kUpdateW1,
		[3]uint32{gx, gy, 1},
		[3]uint16{tile2D, tile2D, 1},
		&updateW1Args{
			W1: b.GW1, Input: b.GInput, DeltaHid: b.GDeltaHid,
			InputN: int32(in), HiddenN: int32(hid), LR: lr,
		},
	)

	// 6. update_w2
	b.driver.EnqueueLaunchKernel(
		b.Queue, b.kUpdateW2,
		[3]uint32{grid1D(hid), 1, 1},
		[3]uint16{blockSz, 1, 1},
		&updateW2Args{
			W2: b.GW2, Hidden: b.GHidden, DeltaOut: b.GDeltaOut,
			HiddenN: int32(hid), OutputN: int32(out), LR: lr,
		},
	)
}

func sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

// Verify recomputes the full epoch on the CPU and compares the updated weight
// matrices (w1, w2) against the GPU results.
func (b *Benchmark) verify() error { //nolint:funlen,gocognit
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
		sum := float64(b.HB1[j])
		for i := 0; i < in; i++ {
			sum += float64(b.HInput[i]) * float64(b.HW1[i*hid+j])
		}
		hidden[j] = sigmoid(sum)
	}
	// forward_output
	for k := 0; k < out; k++ {
		sum := float64(b.HB2[k])
		for j := 0; j < hid; j++ {
			sum += hidden[j] * float64(b.HW2[j*out+k])
		}
		output[k] = sigmoid(sum)
	}
	// backward_output_delta
	for k := 0; k < out; k++ {
		o := output[k]
		deltaOut[k] = o * (1.0 - o) * (float64(b.HTarget[k]) - o)
	}
	// backward_hidden_delta
	for j := 0; j < hid; j++ {
		sum := 0.0
		for k := 0; k < out; k++ {
			sum += float64(b.HW2[j*out+k]) * deltaOut[k]
		}
		deltaHid[j] = hidden[j] * (1.0 - hidden[j]) * sum
	}
	// update_w1
	refW1 := make([]float64, in*hid)
	for i := 0; i < in; i++ {
		for j := 0; j < hid; j++ {
			refW1[i*hid+j] = float64(b.HW1[i*hid+j]) +
				float64(lr)*float64(b.HInput[i])*deltaHid[j]
		}
	}
	// update_w2
	refW2 := make([]float64, hid*out)
	for j := 0; j < hid; j++ {
		for k := 0; k < out; k++ {
			refW2[j*out+k] = float64(b.HW2[j*out+k]) +
				float64(lr)*hidden[j]*deltaOut[k]
		}
	}

	gpuW1 := make([]float32, in*hid)
	gpuW2 := make([]float32, hid*out)
	if err := b.driver.ReadResult(b.Readbacks[0], gpuW1); err != nil {
		return err
	}
	if err := b.driver.ReadResult(b.Readbacks[1], gpuW2); err != nil {
		return err
	}

	check := func(name string, idx int, ref float64, got float64) error {
		denom := math.Abs(ref)
		if denom < 1e-3 {
			denom = 1e-3
		}
		if math.Abs(ref-got)/denom > 1e-3 {
			return fmt.Errorf("%s[%d]: expected %f, but got %f.\n",
				name, idx, ref, got)
		}

		return nil
	}

	for i := range refW1 {
		if err := check("w1", i, refW1[i], float64(gpuW1[i])); err != nil {
			return err
		}
	}
	for i := range refW2 {
		if err := check("w2", i, refW2[i], float64(gpuW2[i])); err != nil {
			return err
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, b.driver.EnqueueMemCopyD2H(b.Queue, b.GW1, 4*((b.InputN)*(b.HiddenN))))
	b.Readbacks = append(b.Readbacks, b.driver.EnqueueMemCopyD2H(b.Queue, b.GW2, 4*((b.HiddenN)*(b.OutputN))))
}

// Step submits computation, then verification readback, then consumes results.
func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
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
