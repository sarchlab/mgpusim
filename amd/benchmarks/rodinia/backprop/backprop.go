// Package backprop runs the Rodinia hybrid input-to-hidden-to-output training epoch.
package backprop

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

const hidden = 16
const width = hidden + 1

type forwardArgs struct {
	Input, Hidden, Weights, Partials gpuruntime.Ptr
	Inputs, HiddenSize               int32
}

type adjustArgs struct {
	Delta                    gpuruntime.Ptr
	HiddenSize               int32
	_                        uint32
	Input                    gpuruntime.Ptr
	Inputs                   int32
	_                        uint32
	Weights, PreviousWeights gpuruntime.Ptr
}

// Benchmark owns both GPU launches and the intermediate CPU phase.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt              *gpuruntime.Runtime
	forward, adjust *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := &Benchmark{rt: rt, State: &State{}}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)
	b.Context = rt.Init()
	return b
}

func (b *Benchmark) loadProgram() {
	b.forward = insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, "bpnn_layerforward_CUDA")
	b.adjust = insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, "bpnn_adjust_weights_cuda")
	if b.forward == nil || b.adjust == nil {
		log.Panic("could not load the two Backprop GPU kernels")
	}
}

func (b *Benchmark) SelectGPU(gpus []int) { b.Gpus = gpus }
func (b *Benchmark) SetUnifiedMemory()    { b.UseUnifiedMemory = true }

func (b *Benchmark) submit() {
	if b.Arch != arch.CDNA3 {
		panic("the Rodinia Backprop kernels require CDNA3")
	}
	b.loadProgram()
	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	if b.InputN <= 0 {
		b.InputN = 64
	}
	b.InputN = max(16, b.InputN/16*16)
	if (b.HiddenN != 0 && b.HiddenN != hidden) || (b.OutputN != 0 && b.OutputN != 1) {
		panic("Rodinia Backprop requires 16 hidden units and one output")
	}
	b.HiddenN, b.OutputN = hidden, 1
	b.Input = make([]float32, b.InputN+1)
	b.Weights = make([]float32, (b.InputN+1)*width)
	b.Partials = make([]float32, b.InputN)
	rand := benchmarks.NewGlibcRand(7)
	draw := func() float32 { return float32(rand.Next()) / 2147483647 }
	for i := range b.Weights {
		b.Weights[i] = draw()
	}
	for j := range b.InitialOutputWeights {
		_ = draw()
		b.InitialOutputWeights[j] = draw()
	}
	b.Input[0] = 1
	for i := 1; i < len(b.Input); i++ {
		b.Input[i] = draw()
	}
	b.initDevice()
	grid, group := b.geometry()
	args := forwardArgs{b.GInput, b.GHidden, b.GWeights, b.GPartials, int32(b.InputN), hidden}
	b.rt.EnqueueLaunchKernel(b.Queue, b.forward, grid, group, &args)
}

func (b *Benchmark) initDevice() {
	alloc := b.rt.AllocateMemory
	if b.UseUnifiedMemory {
		alloc = b.rt.AllocateUnifiedMemory
	}
	weightsBytes := uint64(len(b.Weights) * 4)
	b.GInput = alloc(b.Context, uint64(len(b.Input)*4))
	b.GHidden = alloc(b.Context, width*4)
	b.GWeights = alloc(b.Context, weightsBytes)
	b.GPrevious = alloc(b.Context, weightsBytes)
	b.GDelta = alloc(b.Context, width*4)
	b.GPartials = alloc(b.Context, uint64(len(b.Partials)*4))
	b.Upload(b.rt, b.Queue, b.GInput, b.Input)
	b.Upload(b.rt, b.Queue, b.GWeights, b.Weights)
	b.Upload(b.rt, b.Queue, b.GPrevious, make([]float32, len(b.Weights)))
}

func (b *Benchmark) geometry() ([3]uint32, [3]uint16) {
	return [3]uint32{hidden, uint32(b.InputN), 1}, [3]uint16{hidden, 16, 1}
}

func sigmoid(x float64) float64 { return 1 / (1 + math.Exp(-x)) }

func (b *Benchmark) finishForward() {
	b.Hidden[0] = 1
	for j := 1; j < width; j++ {
		var sum float32
		for block := 0; block < b.InputN/16; block++ {
			sum += b.Partials[block*hidden+j-1]
		}
		sum += b.Weights[j]
		b.Hidden[j] = float32(sigmoid(float64(sum)))
	}
	var sum float32
	for j, value := range b.Hidden {
		// Match the HIP host's separately rounded float32 product.
		sum += float32(value * b.InitialOutputWeights[j])
	}
	b.Output = float32(sigmoid(float64(sum)))
	output := float64(b.Output)
	b.OutputDelta = float32(output * (1 - output) * float64(float32(0.1)-b.Output))
	for j := 1; j < width; j++ {
		h := float64(b.Hidden[j])
		propagated := b.OutputDelta * b.InitialOutputWeights[j]
		b.HiddenDelta[j] = float32(h * (1 - h) * float64(propagated))
	}
	for j, value := range b.Hidden {
		change := float32(0.3 * float64(b.OutputDelta) * float64(value))
		b.OutputMomentum[j] = change
		b.OutputWeights[j] = b.InitialOutputWeights[j] + change
	}
}

func (b *Benchmark) adjustWeights() {
	b.finishForward()
	b.Upload(b.rt, b.Queue, b.GDelta, b.HiddenDelta)
	// The archived forward kernel uses the input weights as scratch.
	b.Upload(b.rt, b.Queue, b.GWeights, b.Weights)
	grid, group := b.geometry()
	args := adjustArgs{Delta: b.GDelta, HiddenSize: hidden, Input: b.GInput,
		Inputs: int32(b.InputN), Weights: b.GWeights, PreviousWeights: b.GPrevious}
	b.rt.EnqueueLaunchKernel(b.Queue, b.adjust, grid, group, &args)
}

func compare(actual, expected, absolute, relative float64, field string, index int) error {
	tolerance := math.Max(absolute, relative*math.Abs(expected))
	if math.IsNaN(actual) || math.IsInf(actual, 0) || math.Abs(actual-expected) > tolerance {
		return fmt.Errorf("%s[%d]: got %g, expected %g", field, index, actual, expected)
	}
	return nil
}

func (b *Benchmark) referenceForward() ([width]float64, error) {
	var activation [width]float64
	activation[0] = 1
	for j := 1; j < width; j++ {
		dot := float64(b.Weights[j])
		for block := 0; block < b.InputN/16; block++ {
			var sum, scale float64
			for row := 1; row <= 16; row++ {
				i := block*16 + row
				product := float64(b.Input[i]) * float64(b.Weights[i*width+j])
				sum += product
				scale += math.Abs(product)
			}
			index := block*hidden + j - 1
			if err := compare(float64(b.Partials[index]), sum, 1e-6*math.Max(1, scale), 0, "partial", index); err != nil {
				return activation, err
			}
			dot += sum
		}
		activation[j] = sigmoid(dot)
	}
	return activation, nil
}

func (b *Benchmark) verify() error {
	activation, err := b.referenceForward()
	if err != nil {
		return err
	}
	var dot float64
	for j, v := range activation {
		dot += v * float64(b.InitialOutputWeights[j])
	}
	output := sigmoid(dot)
	delta := output * (1 - output) * (float64(float32(0.1)) - output)
	if err = compare(float64(b.Output), output, 2e-5, 2e-5, "output", 0); err != nil {
		return err
	}
	if err = compare(float64(b.OutputDelta), delta, 1e-7, 2e-5, "output delta", 0); err != nil {
		return err
	}
	var hiddenDelta [width]float64
	for j, value := range activation {
		if j > 0 {
			hiddenDelta[j] = value * (1 - value) * delta * float64(b.InitialOutputWeights[j])
		}
		if err = compare(float64(b.Hidden[j]), value, 2e-5, 2e-5, "hidden", j); err != nil {
			return err
		}
		if err = compare(float64(b.HiddenDelta[j]), hiddenDelta[j], 1e-7, 2e-5, "hidden delta", j); err != nil {
			return err
		}
		change := 0.3 * delta * value
		if err = verifyWeight(float64(b.OutputWeights[j]), float64(b.InitialOutputWeights[j]),
			float64(b.OutputMomentum[j]), change, j); err != nil {
			return err
		}
	}
	weights, previous := make([]float32, len(b.Weights)), make([]float32, len(b.Weights))
	if err = benchmarks.Read(b.rt, b.Readbacks[0], weights); err != nil {
		return err
	}
	if err = benchmarks.Read(b.rt, b.Readbacks[1], previous); err != nil {
		return err
	}
	for index, weight := range weights {
		i, j := index/width, index%width
		old, momentum := float64(b.Weights[index]), float64(previous[index])
		if j == 0 {
			if weight != b.Weights[index] || momentum != 0 {
				return fmt.Errorf("unused input weight %d changed", index)
			}
			continue
		}
		change := 0.3 * hiddenDelta[j] * float64(b.Input[i])
		if err = verifyWeight(float64(weight), old, momentum, change, index); err != nil {
			return err
		}
	}
	log.Printf("Passed!\n")
	return nil
}

func verifyWeight(weight, old, momentum, change float64, index int) error {
	if err := compare(weight, old+change, 2e-5, 2e-5, "weight", index); err != nil {
		return err
	}
	if err := compare(momentum, change, 1e-7, 2e-5, "momentum", index); err != nil {
		return err
	}
	return compare(weight-old, change, 1e-7, 2e-5, "weight change", index)
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
		b.submit()
	case 1:
		b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GPartials, 4*len(b.Partials)))
	case 2:
		if err := benchmarks.Read(b.rt, b.Readbacks[0], b.Partials); err != nil {
			return err
		}
		b.Readbacks = nil
		b.adjustWeights()
	case 3:
		if !b.Verification {
			b.Ended = true
			return nil
		}
		bytes := 4 * len(b.Weights)
		b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GWeights, bytes),
			benchmarks.Download(b.rt, b.Queue, b.GPrevious, bytes))
	case 4:
		if err := b.verify(); err != nil {
			return err
		}
		b.Readbacks = nil
		b.Ended = true
	}
	b.Phase++
	return nil
}
