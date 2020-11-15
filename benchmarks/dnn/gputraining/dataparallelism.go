package gputraining

import (
	"log"
	"math"
	"sync"

	"gitlab.com/akita/dnn/tensor"
	"gitlab.com/akita/dnn/training"
	"gitlab.com/akita/dnn/training/optimization"
)

// DataParallelismMultiGPUTrainer can use multiple GPUs to train the DNN model
// in the data parallelism style.
type DataParallelismMultiGPUTrainer struct {
	Networks        []training.Network
	DataSource      training.DataSource
	LossFunc        training.LossFunction
	OptimizationAlg optimization.Alg
	Tester          *training.Tester
	Epoch           int
	BatchSize       int
	ShowBatchInfo   bool
}

// Train will run the training algorithm on the network.
func (t DataParallelismMultiGPUTrainer) Train() {
	for currentEpoch := 0; currentEpoch < t.Epoch; currentEpoch++ {
		log.Printf("Epoch %d\n", currentEpoch)

		t.DataSource.Rewind()

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
		}

		t.test()
	}
}

func (t DataParallelismMultiGPUTrainer) calculateBatchGradientAllGPUs() (
	epochCompleted bool,
) {
	var wg sync.WaitGroup

	for _, network := range t.Networks {
		data, label := t.DataSource.NextBatch(t.BatchSize / len(t.Networks))

		if len(label) == 0 {
			epochCompleted = true
			break
		}

		wg.Add(1)

		go t.calculateBatchGradientOneGPU(network, data, label, &wg)
	}

	wg.Wait()

	return epochCompleted
}

func (t DataParallelismMultiGPUTrainer) calculateBatchGradientOneGPU(
	network training.Network,
	data tensor.Tensor, label []int,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	output := t.forward(data, &network)
	derivative := t.calculateLoss(output, label)
	t.backward(derivative, &network)
}

func (t DataParallelismMultiGPUTrainer) forward(
	data tensor.Tensor,
	network *training.Network,
) tensor.Tensor {
	//log.Printf("Forward.\n")
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
) *tensor.SimpleTensor {
	loss, derivative := t.LossFunc.Loss(output, inputLabel)

	if t.ShowBatchInfo {
		accuracy := calculateAccuracy(output, inputLabel)
		log.Printf("loss: %f, accuracy %f\n", loss, accuracy)
	}

	return derivative
}

func (t DataParallelismMultiGPUTrainer) backward(
	derivative *tensor.SimpleTensor,
	network *training.Network,
) {
	//log.Printf("Backward.\n")
	var output tensor.Tensor
	output = derivative
	for i := len(network.Layers) - 1; i >= 0; i-- {
		input := output
		output = network.Layers[i].Backward(input)
	}
}

func (t DataParallelismMultiGPUTrainer) updateParameters() {
	//log.Pridntf("Update Parameters.\n")
	for _, n := range t.Networks {
		for _, l := range n.Layers {
			t.OptimizationAlg.UpdateParameters(l)
		}
	}
}

func (t DataParallelismMultiGPUTrainer) averageGradient() {
	// TODO: Replace this with AllReduce
	for l := range t.Networks[0].Layers {
		var gradients []tensor.Vector
		for _, n := range t.Networks {
			gradients = append(gradients, n.Layers[l].Gradients())
		}

		for i := range gradients {
			for j := range gradients {
				if i == j {
					continue
				}

				gradients[i].Add(gradients[j])
			}
		}

		for i := range gradients {
			gradients[i].Scale(1.0 / float64(len(gradients)))
		}
	}
}

func (t DataParallelismMultiGPUTrainer) test() {
	if t.Tester == nil {
		return
	}

	accuracy := t.Tester.Test()
	log.Printf("Accuracy %f\n", accuracy)
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
