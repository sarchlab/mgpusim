package gputraining

import (
	"log"
	"math"
	"sync"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/mccl"

	"github.com/sarchlab/mgpusim/v4/amd/driver"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/tensor"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/training"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/training/optimization"
)

// DataParallelismMultiGPUTrainer can use multiple GPUs to train the DNN model
// in the data parallelism style.
type DataParallelismMultiGPUTrainer struct {
	TensorOperators  []*gputensor.GPUOperator
	Networks         []training.Network
	DataSource       []training.DataSource
	LossFunc         []training.LossFunction
	OptimizationAlg  []optimization.Alg
	Tester           []*training.Tester
	Epoch            int
	MaxBatchPerEpoch int
	BatchSize        int
	ShowBatchInfo    bool
	GPUs             []int
	Contexts         []*driver.Context
	Driver           *driver.Driver
}

// Train will run the training algorithm on the network.
func (t DataParallelismMultiGPUTrainer) Train() {
	for currentEpoch := 0; currentEpoch < t.Epoch; currentEpoch++ {
		log.Printf("Epoch %d\n", currentEpoch)

		for i := 0; i < len(t.DataSource); i++ {
			dataSource := t.DataSource[i]
			dataSource.Rewind()
		}

		batchNum := 0
		for {
			if t.ShowBatchInfo {
				log.Printf("Batch %d\n", batchNum)
			}

			epochCompleted := t.calculateBatchGradientAllGPUs()

			if epochCompleted {
				break
			}

			t.updateParameters()
			batchNum++

			if batchNum >= t.MaxBatchPerEpoch {
				break
			}
		}

		t.test()
	}
}

func (t DataParallelismMultiGPUTrainer) calculateBatchGradientAllGPUs() (
	epochCompleted bool,
) {
	var wg sync.WaitGroup

	for i, network := range t.Networks {
		dataSource := t.DataSource[i]
		data, label := dataSource.NextBatch(t.BatchSize)

		if len(label) == 0 {
			epochCompleted = true
			break
		}

		wg.Add(1)

		go t.calculateBatchGradientOneGPU(
			network, data, label, &wg, t.LossFunc[i])
	}

	wg.Wait()

	return epochCompleted
}

func (t DataParallelismMultiGPUTrainer) calculateBatchGradientOneGPU(
	network training.Network,
	data tensor.Tensor, label []int,
	wg *sync.WaitGroup,
	lossFunc training.LossFunction,
) {
	defer wg.Done()

	output := t.forward(data, &network)
	derivative := t.calculateLoss(output, label, lossFunc)
	t.backward(derivative, &network)
}

func (t DataParallelismMultiGPUTrainer) forward(
	data tensor.Tensor,
	network *training.Network,
) tensor.Tensor {
	var input, output tensor.Tensor
	output = data
	for _, l := range network.Layers {
		input = output
		output = l.Forward(input)
	}
	return output
}

func (t DataParallelismMultiGPUTrainer) calculateLoss(
	output tensor.Tensor,
	inputLabel []int,
	lossFunc training.LossFunction,
) tensor.Tensor {
	loss, derivative := lossFunc.Loss(output, inputLabel)

	if t.ShowBatchInfo {
		accuracy := calculateAccuracy(output, inputLabel)
		log.Printf("loss: %f, accuracy %f\n", loss, accuracy)
	}

	return derivative
}

func (t DataParallelismMultiGPUTrainer) backward(
	derivative tensor.Tensor,
	network *training.Network,
) {
	var output tensor.Tensor
	output = derivative
	for i := len(network.Layers) - 1; i >= 0; i-- {
		input := output
		output = network.Layers[i].Backward(input)
	}
}

func (t DataParallelismMultiGPUTrainer) updateParameters() {
	if len(t.Networks) > 1 {
		t.averageGradient()
	}

	for _, n := range t.Networks {
		for _, l := range n.Layers {
			for i := 0; i < len(t.OptimizationAlg); i++ {
				alg := t.OptimizationAlg[i]
				alg.UpdateParameters(l)
			}
		}
	}
}

func (t DataParallelismMultiGPUTrainer) averageGradient() {
	for l := range t.Networks[0].Layers {
		if t.Networks[0].Layers[l].Gradients() == nil {
			continue
		}

		var gradients []*gputensor.Tensor
		for _, n := range t.Networks {
			gradients = append(gradients,
				n.Layers[l].Gradients().(*gputensor.Tensor))
		}

		gpuNum := len(t.GPUs)

		datas := make([]driver.Ptr, gpuNum)
		dataSizeArr := gradients[0].Size()
		dataSize := 1
		for i := 0; i < len(dataSizeArr); i++ {
			dataSize *= dataSizeArr[i]
		}

		bufs := make([]driver.Ptr, gpuNum)
		bufSize := 65536

		for i := 0; i < gpuNum; i++ {
			datas[i] = gradients[i].Ptr()
			bufs[i] = t.Driver.AllocateMemory(t.Contexts[i], uint64(bufSize*4))
		}

		comms := mccl.CommInitAllMultipleContexts(
			gpuNum, t.Driver, t.Contexts, t.GPUs)
		mccl.AllReduceRing(t.Driver, comms, datas, dataSize, bufs, bufSize)
	}
}

func (t DataParallelismMultiGPUTrainer) test() {
	if t.Tester == nil {
		return
	}

	for i := 0; i < len(t.Tester); i++ {
		tester := t.Tester[i]
		if tester == nil {
			continue
		}

		accuracy := tester.Test()
		log.Printf("Data Source %d Accuracy %f\n", i, accuracy)
	}
}

func calculateAccuracy(output tensor.Tensor, inputLabel []int) float64 {
	size := output.Size()
	data := output.Vector()
	correct := 0

	for i := 0; i < size[0]; i++ {
		max := -math.MaxFloat64
		maxIndex := -1

		for j := 0; j < size[1]; j++ {
			index := i*size[1] + j
			prob := data[index]
			if prob > max {
				max = prob
				maxIndex = j
			}
		}

		if maxIndex == inputLabel[i] {
			correct++
		}
	}

	return float64(correct) / float64(size[0])
}
