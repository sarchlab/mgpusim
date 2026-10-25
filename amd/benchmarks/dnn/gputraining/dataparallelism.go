package gputraining

import (
	"fmt"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/layers"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/training/optimization"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/mccl"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"log"
	"math"
)

// DataParallelismMultiGPUTrainer advances a batch one saved phase at a time.
// Its owner calls Step only after the central runner drains simulation.
type DataParallelismMultiGPUTrainer struct {
	TensorOperators                    []*gputensor.GPUOperator
	Networks                           []training.Network
	DataSource                         []training.DataSource
	LossFunc                           []training.LossFunction
	OptimizationAlg                    []optimization.Alg
	Tester                             []*training.Tester
	Epoch, MaxBatchPerEpoch, BatchSize int
	ShowBatchInfo                      bool
	GPUs                               []int
	Contexts                           []driver.Context
	Driver                             *driver.Runtime
	phase                              string
	epoch, batch, layer, testGPU       int
	ended                              bool
	values, lossInputs, derivatives    []tensor.Tensor
	labels                             [][]int
	readbacks, lossReadbacks           []driver.CommandID
	collective                         *mccl.Collective
	buffers                            []driver.Ptr
	LastLoss, LastAccuracy             []float64
}

func (t *DataParallelismMultiGPUTrainer) IsEnded() bool { return t.ended }
func (t *DataParallelismMultiGPUTrainer) Step() error {
	for _, o := range t.TensorOperators {
		if err := o.CompleteStep(); err != nil {
			return err
		}
	}
	if t.ended {
		return nil
	}
	switch t.phase {
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
		return fmt.Errorf("unknown trainer phase %q", t.phase)
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
	t.layer = 0
	t.phase = "broadcast"
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepBroadcast() error {
	if t.collective != nil {
		if err := t.collective.Step(t.Driver); err != nil {
			return err
		}
		if !t.collective.IsEnded() {
			return nil
		}
		t.collective = nil
		t.layer++
	}
	for t.layer < len(t.Networks[0].Layers) {
		p := t.Networks[0].Layers[t.layer].Parameters()
		if p == nil || len(t.Networks) == 1 {
			t.layer++
			continue
		}
		data := make([]driver.Ptr, len(t.Networks))
		for i, n := range t.Networks {
			data[i] = n.Layers[t.layer].Parameters().(*gputensor.Tensor).Ptr()
		}
		comms := mccl.CommInitAllMultipleContexts(len(data), t.Driver, t.Contexts, t.GPUs)
		t.collective = mccl.BroadcastRing(t.Driver, comms, t.GPUs[0], data, p.NumElement())
		return t.collective.Step(t.Driver)
	}
	t.phase = "epoch"
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepEpoch() error {
	if t.epoch >= t.Epoch {
		t.ended = true
		return nil
	}
	log.Printf("Epoch %d", t.epoch)
	for _, d := range t.DataSource {
		d.Rewind()
	}
	t.batch = 0
	t.phase = "batch"
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepBatch() error {
	n := len(t.Networks)
	t.values = make([]tensor.Tensor, n)
	t.labels = make([][]int, n)
	for i, d := range t.DataSource {
		t.values[i], t.labels[i] = d.NextBatch(t.BatchSize)
	}
	for _, label := range t.labels {
		if len(label) == 0 {
			for i, v := range t.values {
				if v != nil {
					t.TensorOperators[i].Free(v)
				}
			}
			t.values = nil
			t.phase = "testStart"
			t.testGPU = 0
			return nil
		}
	}
	if t.ShowBatchInfo {
		log.Printf("Batch %d", t.batch)
	}
	t.layer = 0
	t.phase = "forward"
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepForward() error {
	for i, n := range t.Networks {
		input := t.values[i]
		t.values[i] = n.Layers[t.layer].Forward(input)
		t.TensorOperators[i].Free(input)
	}
	t.layer++
	if t.layer == len(t.Networks[0].Layers) {
		t.phase = "loss"
	}
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepLoss() error {
	n := len(t.Networks)
	t.lossInputs = make([]tensor.Tensor, n)
	t.derivatives = make([]tensor.Tensor, n)
	t.readbacks = make([]driver.CommandID, n)
	t.lossReadbacks = make([]driver.CommandID, n)
	for i, o := range t.TensorOperators {
		switch t.LossFunc[i].(type) {
		case *training.SoftmaxCrossEntropy:
			t.lossInputs[i] = o.Softmax(t.values[i])
			t.derivatives[i] = o.SoftmaxCrossEntropyDerivative(t.values[i], t.labels[i])
		case *training.CrossEntropy:
			t.lossInputs[i] = t.values[i]
			t.derivatives[i] = o.CrossEntropyDerivative(t.values[i], t.labels[i])
		default:
			return fmt.Errorf("unsupported GPU loss %T", t.LossFunc[i])
		}
		t.lossReadbacks[i] = o.EnqueueReadback(t.lossInputs[i])
		if t.ShowBatchInfo {
			t.readbacks[i] = o.EnqueueReadback(t.values[i])
		}
	}
	t.phase = "lossRead"
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepLossRead() error {
	t.LastLoss = make([]float64, len(t.Networks))
	t.LastAccuracy = make([]float64, len(t.Networks))
	for i, o := range t.TensorOperators {
		if err := o.ReadTensor(t.lossReadbacks[i], t.lossInputs[i]); err != nil {
			return err
		}
		t.LastLoss[i] = o.CrossEntropy(t.lossInputs[i], t.labels[i])
		if t.ShowBatchInfo {
			if err := o.ReadTensor(t.readbacks[i], t.values[i]); err != nil {
				return err
			}
			t.LastAccuracy[i] = calculateAccuracy(t.values[i], t.labels[i])
			log.Printf("loss: %f, accuracy %f", t.LastLoss[i], t.LastAccuracy[i])
		}
		if t.lossInputs[i] != t.values[i] {
			o.Free(t.lossInputs[i])
		}
		o.Free(t.values[i])
	}
	t.values = t.derivatives
	t.derivatives = nil
	t.lossInputs = nil
	t.layer = len(t.Networks[0].Layers) - 1
	t.phase = "backward"
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepBackward() error {
	for i, n := range t.Networks {
		input := t.values[i]
		t.values[i] = n.Layers[t.layer].Backward(input)
		t.TensorOperators[i].Free(input)
	}
	t.layer--
	if t.layer < 0 {
		for i, v := range t.values {
			if v != nil {
				t.TensorOperators[i].Free(v)
			}
		}
		t.values = nil
		t.layer = 0
		t.phase = "reduce"
	}
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepReduce() error {
	if t.collective != nil {
		if err := t.collective.Step(t.Driver); err != nil {
			return err
		}
		if !t.collective.IsEnded() {
			return nil
		}
		for i, p := range t.buffers {
			if err := t.Driver.FreeMemory(t.Contexts[i], p); err != nil {
				return err
			}
		}
		t.buffers = nil
		t.collective = nil
		t.layer++
	}
	for t.layer < len(t.Networks[0].Layers) {
		g := t.Networks[0].Layers[t.layer].Gradients()
		if g == nil || len(t.Networks) == 1 {
			t.layer++
			continue
		}
		n := len(t.Networks)
		data := make([]driver.Ptr, n)
		t.buffers = make([]driver.Ptr, n)
		size := min(65536, g.NumElement())
		for i, network := range t.Networks {
			data[i] = network.Layers[t.layer].Gradients().(*gputensor.Tensor).Ptr()
			t.Driver.SelectGPU(t.Contexts[i], t.GPUs[i])
			t.buffers[i] = t.Driver.AllocateMemory(t.Contexts[i], uint64(size*4))
		}
		t.collective = mccl.AllReduceRing(
			t.Driver,
			mccl.CommInitAllMultipleContexts(n, t.Driver, t.Contexts, t.GPUs),
			data,
			g.NumElement(),
			t.buffers,
			size,
		)
		return t.collective.Step(t.Driver)
	}
	t.phase = "update"
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepUpdate() error {
	for i, n := range t.Networks {
		for _, l := range n.Layers {
			t.OptimizationAlg[i].UpdateParameters(l)
		}
	}
	t.batch++
	t.phase = "batch"
	if t.batch >= t.MaxBatchPerEpoch {
		t.testGPU = 0
		t.phase = "testStart"
	}
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepTestStart() error {
	for t.testGPU < len(t.Networks) && (t.testGPU >= len(t.Tester) || t.Tester[t.testGPU] == nil) {
		t.testGPU++
	}
	if t.testGPU >= len(t.Networks) {
		t.epoch++
		t.phase = "epoch"
		return nil
	}
	tester := t.Tester[t.testGPU]
	tester.DataSource.Rewind()
	v, label := tester.DataSource.NextBatch(tester.BatchSize)
	if len(label) == 0 {
		return fmt.Errorf("empty testing dataset")
	}
	t.values = []tensor.Tensor{v}
	t.labels = [][]int{label}
	t.layer = 0
	t.phase = "testForward"
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepTestForward() error {
	i := t.testGPU
	input := t.values[0]
	l := t.Networks[i].Layers[t.layer]
	t.values[0] = l.Forward(input)
	t.TensorOperators[i].Free(input)
	layers.ReleaseForward(l, t.TensorOperators[i])
	t.layer++
	if t.layer == len(t.Networks[i].Layers) {
		t.readbacks = []driver.CommandID{t.TensorOperators[i].EnqueueReadback(t.values[0])}
		t.phase = "testRead"
	}
	return nil
}
func (t *DataParallelismMultiGPUTrainer) stepTestRead() error {
	o := t.TensorOperators[t.testGPU]
	if err := o.ReadTensor(t.readbacks[0], t.values[0]); err != nil {
		return err
	}
	log.Printf("Data Source %d Accuracy %f", t.testGPU, calculateAccuracy(t.values[0], t.labels[0]))
	o.Free(t.values[0])
	t.values = nil
	t.testGPU++
	t.phase = "testStart"
	return nil
}
