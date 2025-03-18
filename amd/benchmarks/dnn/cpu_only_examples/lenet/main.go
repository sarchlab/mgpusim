package main

import (
	"flag"
	"math"
	"math/rand"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/tensor"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/training/optimization"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/dataset/mnist"

	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/layers"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/training"
)

func main() {
	flag.Parse()
	rand.Seed(1)

	to := &tensor.CPUOperator{}

	network := defineNetwork(to)
	trainer := training.Trainer{
		TO:         to,
		DataSource: mnist.NewTrainingDataSource(to),
		Network:    network,
		LossFunc:   training.NewSoftmaxCrossEntropy(to),
		// OptimizationAlg: optimization.NewSGD(to, 0.001),
		//OptimizationAlg: optimization.NewMomentum(0.1, 0.9),
		//OptimizationAlg: optimization.NewRMSProp(0.003),
		OptimizationAlg: optimization.NewAdam(to, 0.01),
		Tester: &training.Tester{
			DataSource: mnist.NewTestDataSource(to),
			Network:    network,
			BatchSize:  math.MaxInt32,
		},
		Epoch:         1000,
		BatchSize:     128,
		ShowBatchInfo: true,
	}

	for _, l := range network.Layers {
		l.Randomize()
	}

	trainer.Train()
}

func defineNetwork(to *tensor.CPUOperator) training.Network {
	network := training.Network{
		Layers: []layers.Layer{
			layers.NewConv2D(
				0,
				to,
				[]int{1, 28, 28},
				[]int{6, 1, 5, 5},
				[]int{1, 1},
				[]int{2, 2}),
			layers.NewReluLayer(to),
			layers.NewAvgPoolingLayer(
				to,
				[]int{2, 2},
				[]int{0, 0},
				[]int{2, 2}),
			layers.NewConv2D(
				3,
				to,
				[]int{6, 14, 14},
				[]int{16, 6, 5, 5},
				[]int{1, 1},
				[]int{0, 0}),
			layers.NewReluLayer(to),
			layers.NewAvgPoolingLayer(to,
				[]int{2, 2},
				[]int{0, 0},
				[]int{2, 2}),
			layers.NewFullyConnectedLayer(6, to, 400, 120),
			layers.NewReluLayer(to),
			layers.NewFullyConnectedLayer(8, to, 120, 84),
			layers.NewReluLayer(to),
			layers.NewFullyConnectedLayer(10, to, 84, 10),
		},
	}
	return network
}
