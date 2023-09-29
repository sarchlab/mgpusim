package training

import (
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/training/optimization"

	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/layers"
	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/tensor"
)

// A Network represents a group of layers and how they are connected.
type Network struct {
	Layers []layers.Layer
}

// Trainer implements a basic training algorithm.
type Trainer struct {
	TO              tensor.Operator
	Network         Network
	DataSource      DataSource
	LossFunc        LossFunction
	OptimizationAlg optimization.Alg
	Tester          *Tester
	Epoch           int
	BatchSize       int
	ShowBatchInfo   bool
}

// Train will run the training algorithm on the network.
func (t Trainer) Train() {
	for currentEpoch := 0; currentEpoch < t.Epoch; currentEpoch++ {
		log.Printf("Epoch %d\n", currentEpoch)

		t.DataSource.Rewind()

		batchNum := 0
		for {
			if t.ShowBatchInfo {
				log.Printf("Batch %d\n", batchNum)
			}

			data, label := t.DataSource.NextBatch(t.BatchSize)
			if len(label) == 0 {
				break
			}

			output := t.forward(data)
			derivative := t.calculateLoss(output, label)
			t.backward(derivative)
			t.updateParameters()
			batchNum++
		}

		t.test()
	}
}

func (t Trainer) forward(data tensor.Tensor) tensor.Tensor {
	//log.Printf("Forward.\n")
	var input, output tensor.Tensor
	output = data
	for _, l := range t.Network.Layers {
		input = output
		//log.Println(t.TO == nil)
		//t.TO.Dump(input)
		//log.Println("Input ", t.TO.Dump(input))
		//if l.Parameters() != nil {
		//	log.Println("Param ", t.TO.Dump(l.Parameters()))
		//}
		output = l.Forward(input)
		//log.Println("Output ", t.TO.Dump(output))
	}
	return output
}

func (t Trainer) calculateLoss(
	output tensor.Tensor,
	inputLabel []int,
) tensor.Tensor {
	loss, derivative := t.LossFunc.Loss(output, inputLabel)

	if t.ShowBatchInfo {
		accuracy := calculateAccuracy(output, inputLabel)
		log.Printf("loss: %f, accuracy %f\n", loss, accuracy)
	}

	return derivative
}

func (t Trainer) backward(derivative tensor.Tensor) {
	//log.Printf("Backward.\n")
	var output tensor.Tensor
	output = derivative
	for i := len(t.Network.Layers) - 1; i >= 0; i-- {
		input := output
		output = t.Network.Layers[i].Backward(input)
	}
}

func (t Trainer) updateParameters() {
	//log.Printf("Update Parameters.\n")
	for _, l := range t.Network.Layers {
		t.OptimizationAlg.UpdateParameters(l)
		// if l.Gradients() != nil {
		// 	fmt.Println("\n\nLayer ", i, "\nGradient", t.TO.Dump(l.Gradients()),
		// 		"\nParams", t.TO.Dump(l.Parameters()))
		// }
	}
}

func (t Trainer) test() {
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
