package gputraining

import (
	"fmt"
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/layers"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training/optimization"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/mccl"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

// DataParallelismMultiGPUTrainer advances a batch one saved phase at a time.
// Its owner calls Step only after the central runner drains simulation.
type DataParallelismMultiGPUTrainer struct {
	*State
	TensorOperators []*gputensor.GPUOperator
	Networks        []training.Network
	DataSource      []training.DataSource
	LossFunc        []training.LossFunction
	OptimizationAlg []optimization.Alg
	Tester          []*training.Tester
	Driver          *driver.Runtime
}

func (t *DataParallelismMultiGPUTrainer) IsEnded() bool { return t.Ended }
func (t *DataParallelismMultiGPUTrainer) Step() error {
	for _, o := range t.TensorOperators {
		if err := o.CompleteStep(); err != nil {
			return err
		}
	}
	if t.Ended {
		return nil
	}
	switch t.Phase {
	case "":
		return t.stepInitialize()
	case "broadcast":
		return t.stepBroadcast()
	case "epoch":
		return t.stepEpoch()
	case "batch":
		return t.stepBatch()
	case "forward":
		return t.stepForward()
	case "loss":
		return t.stepLoss()
	case "lossRead":
		return t.stepLossRead()
	case "backward":
		return t.stepBackward()
	case "reduce":
		return t.stepReduce()
	case "update":
		return t.stepUpdate()
	case "testStart":
		return t.stepTestStart()
	case "testForward":
		return t.stepTestForward()
	case "testRead":
		return t.stepTestRead()
	default:
		return fmt.Errorf("unknown trainer phase %q", t.Phase)
	}
}
func calculateAccuracy(output tensor.Tensor, labels []int) float64 {
	size, data := output.Size(), output.Vector()
	correct := 0
	for i := 0; i < size[0]; i++ {
		maxValue, index := -math.MaxFloat64, -1
		for j := 0; j < size[1]; j++ {
			if data[i*size[1]+j] > maxValue {
				maxValue = data[i*size[1]+j]
				index = j
			}
		}
		if index == labels[i] {
			correct++
		}
	}
	return float64(correct) / float64(size[0])
}

func (t *DataParallelismMultiGPUTrainer) stepInitialize() error {
	if len(t.Networks) == 0 || len(t.Networks) != len(t.TensorOperators) || t.BatchSize <= 0 {
		return fmt.Errorf("invalid GPU trainer configuration")
	}
	for _, l := range t.Networks[0].Layers {
		l.Randomize()
	}
	t.Layer = 0
	t.Phase = "broadcast"
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepBroadcast() error {
	if t.Collective != nil {
		if err := t.Collective.Step(t.Driver); err != nil {
			return err
		}
		if !t.Collective.IsEnded() {
			return nil
		}
		t.Collective = nil
		t.Layer++
	}
	for t.Layer < len(t.Networks[0].Layers) {
		p := t.Networks[0].Layers[t.Layer].Parameters()
		if p == nil || len(t.Networks) == 1 {
			t.Layer++
			continue
		}
		data := make([]driver.Ptr, len(t.Networks))
		for i, n := range t.Networks {
			data[i] = n.Layers[t.Layer].Parameters().(*gputensor.Tensor).Ptr()
		}
		comms := mccl.CommInitAllMultipleContexts(len(data), t.Driver, t.Contexts, t.GPUs)
		t.Collective = mccl.BroadcastRing(t.Driver, comms, t.GPUs[0], data, p.NumElement())
		return t.Collective.Step(t.Driver)
	}
	t.Phase = "epoch"
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepEpoch() error {
	if t.CurrentEpoch >= t.Epoch {
		t.Ended = true
		return nil
	}
	log.Printf("Epoch %d", t.CurrentEpoch)
	for _, d := range t.DataSource {
		d.Rewind()
	}
	t.Batch = 0
	t.Phase = "batch"
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepBatch() error {
	n := len(t.Networks)
	t.Values = make([]tensor.ID, n)
	t.Labels = make([][]int, n)
	for i, d := range t.DataSource {
		v, label := d.NextBatch(t.BatchSize)
		t.Values[i], t.Labels[i] = t.TensorOperators[i].Reference(v), label
	}
	for _, label := range t.Labels {
		if len(label) == 0 {
			for i, v := range t.Values {
				if v != 0 {
					t.TensorOperators[i].Free(t.TensorOperators[i].Resolve(v))
				}
			}
			t.Values = nil
			t.Phase = "testStart"
			t.TestGPU = 0
			return nil
		}
	}
	if t.ShowBatchInfo {
		log.Printf("Batch %d", t.Batch)
	}
	t.Layer = 0
	t.Phase = "forward"
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepForward() error {
	for i, n := range t.Networks {
		input := t.TensorOperators[i].Resolve(t.Values[i])
		t.Values[i] = t.TensorOperators[i].Reference(n.Layers[t.Layer].Forward(input))
		t.TensorOperators[i].Free(input)
	}
	t.Layer++
	if t.Layer == len(t.Networks[0].Layers) {
		t.Phase = "loss"
	}
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepLoss() error {
	n := len(t.Networks)
	t.LossInputs = make([]tensor.ID, n)
	t.Derivatives = make([]tensor.ID, n)
	t.Readbacks = make([]driver.HostBuffer, n)
	t.LossReadbacks = make([]driver.HostBuffer, n)
	for i, o := range t.TensorOperators {
		switch t.LossFunc[i].(type) {
		case *training.SoftmaxCrossEntropy:
			t.LossInputs[i] = o.Reference(o.Softmax(o.Resolve(t.Values[i])))
			t.Derivatives[i] = o.Reference(o.SoftmaxCrossEntropyDerivative(o.Resolve(t.Values[i]), t.Labels[i]))
		case *training.CrossEntropy:
			t.LossInputs[i] = t.Values[i]
			t.Derivatives[i] = o.Reference(o.CrossEntropyDerivative(o.Resolve(t.Values[i]), t.Labels[i]))
		default:
			return fmt.Errorf("unsupported GPU loss %T", t.LossFunc[i])
		}
		t.LossReadbacks[i] = o.EnqueueReadback(o.Resolve(t.LossInputs[i]))
		if t.ShowBatchInfo {
			t.Readbacks[i] = o.EnqueueReadback(o.Resolve(t.Values[i]))
		}
	}
	t.Phase = "lossRead"
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepLossRead() error {
	t.LastLoss = make([]float64, len(t.Networks))
	t.LastAccuracy = make([]float64, len(t.Networks))
	for i, o := range t.TensorOperators {
		if err := o.ReadTensor(t.LossReadbacks[i], o.Resolve(t.LossInputs[i])); err != nil {
			return err
		}
		t.LastLoss[i] = o.CrossEntropy(o.Resolve(t.LossInputs[i]), t.Labels[i])
		if t.ShowBatchInfo {
			if err := o.ReadTensor(t.Readbacks[i], o.Resolve(t.Values[i])); err != nil {
				return err
			}
			t.LastAccuracy[i] = calculateAccuracy(o.Resolve(t.Values[i]), t.Labels[i])
			log.Printf("loss: %f, accuracy %f", t.LastLoss[i], t.LastAccuracy[i])
		}
		if t.LossInputs[i] != t.Values[i] {
			o.Free(o.Resolve(t.LossInputs[i]))
		}
		o.Free(o.Resolve(t.Values[i]))
	}
	t.Values = t.Derivatives
	t.Derivatives = nil
	t.LossInputs = nil
	t.Layer = len(t.Networks[0].Layers) - 1
	t.Phase = "backward"
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepBackward() error {
	for i, n := range t.Networks {
		input := t.TensorOperators[i].Resolve(t.Values[i])
		t.Values[i] = t.TensorOperators[i].Reference(n.Layers[t.Layer].Backward(input))
		t.TensorOperators[i].Free(input)
	}
	t.Layer--
	if t.Layer < 0 {
		for i, v := range t.Values {
			if v != 0 {
				t.TensorOperators[i].Free(t.TensorOperators[i].Resolve(v))
			}
		}
		t.Values = nil
		t.Layer = 0
		t.Phase = "reduce"
	}
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepReduce() error {
	if t.Collective != nil {
		if err := t.Collective.Step(t.Driver); err != nil {
			return err
		}
		if !t.Collective.IsEnded() {
			return nil
		}
		for i, p := range t.Buffers {
			if err := t.Driver.FreeMemory(t.Contexts[i], p); err != nil {
				return err
			}
		}
		t.Buffers = nil
		t.Collective = nil
		t.Layer++
	}
	for t.Layer < len(t.Networks[0].Layers) {
		g := t.Networks[0].Layers[t.Layer].Gradients()
		if g == nil || len(t.Networks) == 1 {
			t.Layer++
			continue
		}
		n := len(t.Networks)
		data := make([]driver.Ptr, n)
		t.Buffers = make([]driver.Ptr, n)
		size := min(65536, g.NumElement())
		for i, network := range t.Networks {
			data[i] = network.Layers[t.Layer].Gradients().(*gputensor.Tensor).Ptr()
			t.Driver.SelectGPU(t.Contexts[i], t.GPUs[i])
			t.Buffers[i] = t.Driver.AllocateMemory(t.Contexts[i], uint64(size*4))
		}
		t.Collective = mccl.AllReduceRing(
			t.Driver,
			mccl.CommInitAllMultipleContexts(n, t.Driver, t.Contexts, t.GPUs),
			data,
			g.NumElement(),
			t.Buffers,
			size,
		)
		return t.Collective.Step(t.Driver)
	}
	t.Phase = "update"
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepUpdate() error {
	for i, n := range t.Networks {
		for _, l := range n.Layers {
			t.OptimizationAlg[i].UpdateParameters(l)
		}
	}
	t.Batch++
	t.Phase = "batch"
	if t.Batch >= t.MaxBatchPerEpoch {
		t.TestGPU = 0
		t.Phase = "testStart"
	}
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepTestStart() error {
	for t.TestGPU < len(t.Networks) && (t.TestGPU >= len(t.Tester) || t.Tester[t.TestGPU] == nil) {
		t.TestGPU++
	}
	if t.TestGPU >= len(t.Networks) {
		t.CurrentEpoch++
		t.Phase = "epoch"
		return nil
	}
	tester := t.Tester[t.TestGPU]
	tester.DataSource.Rewind()
	v, label := tester.DataSource.NextBatch(tester.BatchSize)
	if len(label) == 0 {
		return fmt.Errorf("empty testing dataset")
	}
	t.Values = []tensor.ID{t.TensorOperators[t.TestGPU].Reference(v)}
	t.Labels = [][]int{label}
	t.Layer = 0
	t.Phase = "testForward"
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepTestForward() error {
	i := t.TestGPU
	input := t.TensorOperators[i].Resolve(t.Values[0])
	l := t.Networks[i].Layers[t.Layer]
	t.Values[0] = t.TensorOperators[i].Reference(l.Forward(input))
	t.TensorOperators[i].Free(input)
	layers.ReleaseForward(l, t.TensorOperators[i])
	t.Layer++
	if t.Layer == len(t.Networks[i].Layers) {
		t.Readbacks = []driver.HostBuffer{t.TensorOperators[i].EnqueueReadback(t.TensorOperators[i].Resolve(t.Values[0]))}
		t.Phase = "testRead"
	}
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepTestRead() error {
	o := t.TensorOperators[t.TestGPU]
	if err := o.ReadTensor(t.Readbacks[0], o.Resolve(t.Values[0])); err != nil {
		return err
	}
	log.Printf("Data Source %d Accuracy %f", t.TestGPU, calculateAccuracy(o.Resolve(t.Values[0]), t.Labels[0]))
	o.Free(o.Resolve(t.Values[0]))
	t.Values = nil
	t.TestGPU++
	t.Phase = "testStart"
	return nil
}
