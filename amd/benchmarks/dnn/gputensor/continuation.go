package gputensor

import (
	"fmt"
	"maps"
	"math"
	"reflect"
	"slices"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// TensorState is a tensor's host continuation, without a Runtime pointer.
// Device bytes belong to the simulated memory resource.
type TensorState struct {
	HostEpoch  uint64
	Present    bool
	Size       []int
	Address    driver.Ptr
	Descriptor string
	Host       []float64
	HostReady  bool
}

func Snapshot(t tensor.Tensor) TensorState {
	if t == nil {
		return TensorState{}
	}
	v := t.(*Tensor)
	return TensorState{
		v.hostEpoch,
		true,
		slices.Clone(v.size),
		v.ptr,
		v.descriptor,
		slices.Clone(v.host),
		v.hostReady,
	}
}
func (o *GPUOperator) RestoreTensor(s TensorState) tensor.Tensor {
	if !s.Present {
		return nil
	}
	return &Tensor{
		owner:      o,
		hostEpoch:  s.HostEpoch,
		driver:     o.driver,
		ctx:        o.ctx,
		size:       slices.Clone(s.Size),
		ptr:        s.Address,
		descriptor: s.Descriptor,
		host:       slices.Clone(s.Host),
		hostReady:  s.HostReady,
	}
}

// OperatorState is embedded in the owning benchmark's checkpoint.
type OperatorState struct {
	ReadbackEpochs           map[driver.CommandID]uint64
	MutationEpoch            uint64
	RandomState              uint64
	Context                  driver.Context
	Queue                    driver.CommandQueue
	Arch                     arch.Type
	Verification, ReportTime bool
	PendingFrees             []driver.Ptr
	Checks                   []verification
	Timers                   []timerRecord
	Timer                    driver.Event
}

func (o *GPUOperator) Snapshot() OperatorState {
	return OperatorState{
		maps.Clone(o.readbackEpochs), o.mutationEpoch,
		o.randomState,
		o.ctx,
		o.queue,
		o.Arch,
		o.verification,
		o.reportTime,
		slices.Clone(o.pendingFrees),
		o.checks,
		o.timers,
		o.timer,
	}
}

// RestoreOperator reconnects a saved operator without allocating or submitting.
func RestoreOperator(r *driver.Runtime, s OperatorState) *GPUOperator {
	return &GPUOperator{
		readbackEpochs: maps.Clone(s.ReadbackEpochs), mutationEpoch: s.MutationEpoch,
		randomState:  s.RandomState,
		driver:       r,
		ctx:          s.Context,
		queue:        s.Queue,
		Arch:         s.Arch,
		verification: s.Verification,
		reportTime:   s.ReportTime,
		pendingFrees: s.PendingFrees,
		checks:       s.Checks,
		timers:       s.Timers,
		timer:        s.Timer,
	}
}
func (o *GPUOperator) Queue() driver.CommandQueue   { return o.queue }
func (o *GPUOperator) SetVerification(enabled bool) { o.verification = enabled }
func (o *GPUOperator) deferFree(ptr driver.Ptr)     { o.pendingFrees = append(o.pendingFrees, ptr) }

// EnqueueReadback snapshots the tensor at its position in the shared stream.
func (o *GPUOperator) EnqueueReadback(t tensor.Tensor) driver.CommandID {
	id := o.driver.EnqueueMemCopyD2H(o.queue, t.(*Tensor).ptr, t.NumElement()*4)
	if o.readbackEpochs == nil {
		o.readbackEpochs = make(map[driver.CommandID]uint64)
	}
	o.readbackEpochs[id] = o.mutationEpoch
	return id
}

// ReadTensor copies a completed result into the tensor's explicit host cache.
func (o *GPUOperator) ReadTensor(id driver.CommandID, t tensor.Tensor) error {
	epoch, ok := o.readbackEpochs[id]
	if !ok {
		return fmt.Errorf("gputensor: readback %d was not submitted by this operator", id)
	}
	raw := make([]float32, t.NumElement())
	if err := o.driver.ReadResult(id, raw); err != nil {
		return err
	}
	v := t.(*Tensor)
	v.host = make([]float64, len(raw))
	for i, x := range raw {
		v.host[i] = float64(x)
	}
	v.hostReady = true
	v.hostEpoch = epoch
	delete(o.readbackEpochs, id)
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
	state := Snapshot(t)
	state.Host = nil
	state.HostReady = false
	return tensorReadback{state, o.driver.EnqueueMemCopyD2H(o.queue, t.(*Tensor).ptr, t.NumElement()*4)}
}
func (o *GPUOperator) beginVerification(v verification, inputs ...tensor.Tensor) int {
	if !o.verification {
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
	o.checks = append(o.checks, v)
	return len(o.checks) - 1
}
func (o *GPUOperator) endVerification(index int, outputs ...tensor.Tensor) {
	for _, t := range outputs {
		t.(*Tensor).hostReady = false
	}
	if index < 0 {
		return
	}
	for _, t := range outputs {
		o.checks[index].Outputs = append(o.checks[index].Outputs, o.capture(t))
	}
}
func (o *GPUOperator) readVerification(r tensorReadback) (tensor.Tensor, error) {
	values := make([]float32, lenOf(r.Tensor.Size))
	if err := o.driver.ReadResult(r.Command, values); err != nil {
		return nil, err
	}
	data := make([]float64, len(values))
	for i, v := range values {
		data[i] = float64(v)
	}
	return tensor.CPUOperator{}.CreateWithData(data, r.Tensor.Size, r.Tensor.Descriptor), nil
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
	if o.reportTime {
		o.timer = o.driver.EnqueueRecordEvent(o.queue)
	}
}
func (o *GPUOperator) timerEnd(name string) {
	if o.reportTime {
		o.timers = append(o.timers, timerRecord{name, o.timer, o.driver.EnqueueRecordEvent(o.queue)})
	}
}

// CompleteStep processes this operator's previous batch after the central
// runner has globally drained simulation. It never drives the engine.
func (o *GPUOperator) CompleteStep() error {
	counts := o.driver.Unfinished()
	if int(o.queue) > len(counts) || counts[o.queue-1] != 0 {
		return fmt.Errorf("gputensor: batch has not completed")
	}
	for len(o.checks) > 0 {
		if err := o.check(o.checks[0]); err != nil {
			return err
		}
		o.checks = o.checks[1:]
	}
	for len(o.timers) > 0 {
		timer := o.timers[0]
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
		o.timers = o.timers[1:]
	}
	for len(o.pendingFrees) > 0 {
		if err := o.driver.FreeMemory(o.ctx, o.pendingFrees[0]); err != nil {
			return err
		}
		o.pendingFrees = o.pendingFrees[1:]
	}
	return nil
}

// RandomFloat64 advances a local, checkpointed parameter initializer.
func (o *GPUOperator) RandomFloat64() float64 {
	o.randomState = o.randomState*6364136223846793005 + 1442695040888963407
	return float64(o.randomState>>11) * (1.0 / (1 << 53))
}

func (o *GPUOperator) enqueueKernel(co *insts.KernelCodeObject, global [3]uint32, local [3]uint16, args any) {
	o.mutationEpoch++
	o.driver.EnqueueLaunchKernel(o.queue, co, global, local, args)
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
