package gputensor

import (
	"fmt"
	"math"
	"reflect"
	"slices"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// TensorState is the canonical mutable data for a live tensor. Device bytes
// belong to simulated memory; layer/trainer references save only tensor IDs.
type TensorState struct {
	HostEpoch   uint64
	Shape       []int
	Address     driver.Ptr
	Description string
	Host        []float64
	HostReady   bool
}

// OperatorState is used directly by the operator and owned by its benchmark.
type OperatorState struct {
	ReadbackEpochs             map[driver.CommandID]uint64
	MutationEpoch              uint64
	RandomState                uint64
	Context                    driver.Context
	CommandQueue               driver.CommandQueue
	Arch                       arch.Type
	Verification, ReportTiming bool
	PendingFrees               []driver.Ptr
	Checks                     []verification
	Timers                     []timerRecord
	Timer                      driver.Event
	NextTensor                 tensor.ID
	Tensors                    map[tensor.ID]*TensorState
}

// BindOperator reconnects saved state without allocation or submission.
func BindOperator(r *driver.Runtime, state *OperatorState) *GPUOperator {
	return &GPUOperator{OperatorState: state, driver: r, tensors: make(map[tensor.ID]*Tensor)}
}
func (o *GPUOperator) newTensor(state TensorState) *Tensor {
	if o.Tensors == nil {
		o.Tensors = make(map[tensor.ID]*TensorState)
	}
	o.NextTensor++
	o.Tensors[o.NextTensor] = &state
	return o.Resolve(o.NextTensor).(*Tensor)
}
func (o *GPUOperator) Reference(t tensor.Tensor) tensor.ID {
	if t == nil {
		return 0
	}
	v := t.(*Tensor)
	if v.owner != o {
		panic("gputensor: tensor belongs to another operator")
	}
	return v.id
}
func (o *GPUOperator) Resolve(id tensor.ID) tensor.Tensor {
	if id == 0 {
		return nil
	}
	state, ok := o.Tensors[id]
	if !ok {
		return nil
	} // A freed handle has no live tensor to bind.
	if t := o.tensors[id]; t != nil {
		return t
	}
	t := &Tensor{TensorState: state, owner: o, id: id}
	o.tensors[id] = t
	return t
}
func (o *GPUOperator) Queue() driver.CommandQueue   { return o.CommandQueue }
func (o *GPUOperator) SetVerification(enabled bool) { o.Verification = enabled }
func (o *GPUOperator) deferFree(ptr driver.Ptr)     { o.PendingFrees = append(o.PendingFrees, ptr) }

// EnqueueReadback snapshots the tensor at its position in the shared stream.
func (o *GPUOperator) EnqueueReadback(t tensor.Tensor) driver.CommandID {
	id := o.driver.EnqueueMemCopyD2H(o.CommandQueue, t.(*Tensor).Address, t.NumElement()*4)
	if o.ReadbackEpochs == nil {
		o.ReadbackEpochs = make(map[driver.CommandID]uint64)
	}
	o.ReadbackEpochs[id] = o.MutationEpoch
	return id
}

// ReadTensor copies a completed result into the tensor's explicit host cache.
func (o *GPUOperator) ReadTensor(id driver.CommandID, t tensor.Tensor) error {
	epoch, ok := o.ReadbackEpochs[id]
	if !ok {
		return fmt.Errorf("gputensor: readback %d was not submitted by this operator", id)
	}
	raw := make([]float32, t.NumElement())
	if err := o.driver.ReadResult(id, raw); err != nil {
		return err
	}
	v := t.(*Tensor)
	v.Host = make([]float64, len(raw))
	for i, x := range raw {
		v.Host[i] = float64(x)
	}
	v.HostReady = true
	v.HostEpoch = epoch
	delete(o.ReadbackEpochs, id)
	return nil
}

type tensorReadback struct {
	Tensor  TensorState
	Command driver.CommandID
}
type verification struct {
	Operation                                                  string
	Inputs, Outputs                                            []tensorReadback
	Order, Axis, KernelSize, Padding, Stride, Dilation, Labels []int
	Times                                                      int
	TransA, TransB                                             bool
	Alpha, Beta, Smooth1, Smooth2, LearningRate                float64
}

func (o *GPUOperator) capture(t tensor.Tensor) tensorReadback {
	state := TensorState{Shape: slices.Clone(t.Size()), Description: t.Descriptor()}
	state.Host = nil
	state.HostReady = false
	return tensorReadback{state, o.driver.EnqueueMemCopyD2H(o.CommandQueue, t.(*Tensor).Address, t.NumElement()*4)}
}
func (o *GPUOperator) beginVerification(v verification, inputs ...tensor.Tensor) int {
	if !o.Verification {
		return -1
	}
	v.Order = slices.Clone(v.Order)
	v.Axis = slices.Clone(v.Axis)
	v.KernelSize = slices.Clone(v.KernelSize)
	v.Padding = slices.Clone(v.Padding)
	v.Stride = slices.Clone(v.Stride)
	v.Dilation = slices.Clone(v.Dilation)
	v.Labels = slices.Clone(v.Labels)
	for _, t := range inputs {
		v.Inputs = append(v.Inputs, o.capture(t))
	}
	o.Checks = append(o.Checks, v)
	return len(o.Checks) - 1
}
func (o *GPUOperator) endVerification(index int, outputs ...tensor.Tensor) {
	for _, t := range outputs {
		t.(*Tensor).HostReady = false
	}
	if index < 0 {
		return
	}
	for _, t := range outputs {
		o.Checks[index].Outputs = append(o.Checks[index].Outputs, o.capture(t))
	}
}
func (o *GPUOperator) readVerification(r tensorReadback) (tensor.Tensor, error) {
	values := make([]float32, lenOf(r.Tensor.Shape))
	if err := o.driver.ReadResult(r.Command, values); err != nil {
		return nil, err
	}
	data := make([]float64, len(values))
	for i, v := range values {
		data[i] = float64(v)
	}
	return tensor.CPUOperator{}.CreateWithData(data, r.Tensor.Shape, r.Tensor.Description), nil
}
func lenOf(shape []int) int {
	n := 1
	for _, d := range shape {
		n *= d
	}
	return n
}
func (o *GPUOperator) check(v verification) error {
	inputs := make([]tensor.Tensor, len(v.Inputs))
	for i, r := range v.Inputs {
		t, err := o.readVerification(r)
		if err != nil {
			return err
		}
		inputs[i] = t
	}
	expected, err := reference(v, inputs)
	if err != nil {
		return err
	}
	for i, ref := range expected {
		actual, err := o.readVerification(v.Outputs[i])
		if err != nil {
			return err
		}
		if ref.Descriptor() != actual.Descriptor() || !reflect.DeepEqual(ref.Size(), actual.Size()) {
			return fmt.Errorf("%s: tensor shape/descriptor mismatch", v.Operation)
		}
		values := actual.Vector()
		for j, want := range ref.Vector() {
			got := values[j]
			if math.Abs(want) < 1e-5 && math.Abs(got) < 1e-5 {
				continue
			}
			if math.IsNaN(got) || math.Abs(want-got) > math.Abs(1e-2*want) {
				return fmt.Errorf("%s[%d]: expected %.15f, got %.15f", v.Operation, j, want, got)
			}
		}
	}
	return nil
}

type timerRecord struct {
	Name       string
	Start, End driver.Event
}

func (o *GPUOperator) timerStart() {
	if o.ReportTiming {
		o.Timer = o.driver.EnqueueRecordEvent(o.CommandQueue)
	}
}
func (o *GPUOperator) timerEnd(name string) {
	if o.ReportTiming {
		o.Timers = append(o.Timers, timerRecord{name, o.Timer, o.driver.EnqueueRecordEvent(o.CommandQueue)})
	}
}

// CompleteStep processes this operator's previous batch after the central
// runner has globally drained simulation. It never drives the engine.
func (o *GPUOperator) CompleteStep() error {
	counts := o.driver.Unfinished()
	if int(o.CommandQueue) > len(counts) || counts[o.CommandQueue-1] != 0 {
		return fmt.Errorf("gputensor: batch has not completed")
	}
	for len(o.Checks) > 0 {
		if err := o.check(o.Checks[0]); err != nil {
			return err
		}
		o.Checks = o.Checks[1:]
	}
	for len(o.Timers) > 0 {
		timer := o.Timers[0]
		start, err := o.driver.EventTime(timer.Start)
		if err != nil {
			return err
		}
		end, err := o.driver.EventTime(timer.End)
		if err != nil {
			return err
		}
		fmt.Printf(
			"Kernel %s, Start %.10f, Virtual Time: %v\n",
			timer.Name,
			float64(start)*1e-12,
			float64(end-start)*1e-12,
		)
		o.Timers = o.Timers[1:]
	}
	for len(o.PendingFrees) > 0 {
		if err := o.driver.FreeMemory(o.Context, o.PendingFrees[0]); err != nil {
			return err
		}
		o.PendingFrees = o.PendingFrees[1:]
	}
	return nil
}

// RandomFloat64 advances a local, checkpointed parameter initializer.
func (o *GPUOperator) RandomFloat64() float64 {
	o.RandomState = o.RandomState*6364136223846793005 + 1442695040888963407
	return float64(o.RandomState>>11) * (1.0 / (1 << 53))
}

func (o *GPUOperator) enqueueKernel(co *insts.KernelCodeObject, global [3]uint32, local [3]uint16, args any) {
	o.MutationEpoch++
	o.driver.EnqueueLaunchKernel(o.CommandQueue, co, global, local, args)
}

func reference(v verification, inputs []tensor.Tensor) ([]tensor.Tensor, error) {
	cpu := tensor.CPUOperator{}
	switch v.Operation {
	case "Repeat":
		return []tensor.Tensor{cpu.Repeat(inputs[0], v.Times)}, nil
	case "Transpose":
		return []tensor.Tensor{cpu.Transpose(inputs[0], v.Order)}, nil
	case "Rotate180":
		return []tensor.Tensor{cpu.Rotate180(inputs[0])}, nil
	case "Dilate":
		return []tensor.Tensor{cpu.Dilate(inputs[0], v.Dilation)}, nil
	case "Sum":
		return []tensor.Tensor{cpu.Sum(inputs[0], v.Axis)}, nil
	case "Gemm":
		return []tensor.Tensor{cpu.Gemm(v.TransA, v.TransB, v.Alpha, v.Beta, inputs[0], inputs[1], inputs[2])}, nil
	case "Im2Col":
		return []tensor.Tensor{cpu.Im2Col(inputs[0], v.KernelSize, v.Padding, v.Stride, v.Dilation)}, nil
	case "AvgPoolingForward":
		return []tensor.Tensor{cpu.AvgPoolingForward(inputs[0], v.KernelSize, v.Padding, v.Stride)}, nil
	case "AvgPoolingBackward":
		return []tensor.Tensor{cpu.AvgPoolingBackward(inputs[0], inputs[1], v.KernelSize, v.Padding, v.Stride)}, nil
	case "Softmax":
		return []tensor.Tensor{cpu.Softmax(inputs[0])}, nil
	case "CrossEntropyDerivative":
		return []tensor.Tensor{cpu.CrossEntropyDerivative(inputs[0], v.Labels)}, nil
	case "SoftmaxCrossEntropyDerivative":
		return []tensor.Tensor{cpu.SoftmaxCrossEntropyDerivative(inputs[0], v.Labels)}, nil
	case "ElementWiseMul":
		return []tensor.Tensor{cpu.ElementWiseMul(inputs[0], inputs[1])}, nil
	case "ScaleAdd":
		return []tensor.Tensor{cpu.ScaleAdd(v.Alpha, v.Beta, inputs[0], inputs[1])}, nil
	case "Adam":
		cpu.Adam(inputs[0], inputs[1], inputs[2], inputs[3], v.Smooth1, v.Smooth2, v.LearningRate)
		return []tensor.Tensor{inputs[0], inputs[2], inputs[3]}, nil
	case "ReluForward":
		return []tensor.Tensor{cpu.ReluForward(inputs[0])}, nil
	case "ReluBackward":
		return []tensor.Tensor{cpu.ReluBackward(inputs[0], inputs[1])}, nil
	default:
		return nil, fmt.Errorf("gputensor: unknown verification operation %q", v.Operation)
	}
}

// Replace changes a saved reference; Free owns GPU tensor retirement.
func (o *GPUOperator) Replace(_ tensor.ID, t tensor.Tensor) tensor.ID { return o.Reference(t) }
