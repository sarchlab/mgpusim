package main

import (
	"flag"
	"math"
	"math/rand"

	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/tensor"
	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/training/optimization"

	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/dataset/mnist"

	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/layers"
	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/training"
)

func main() {
	flag.Parse()
	rand.Seed(1)

	to := &tensor.CPUOperator{}

	network := training.Network{
		Layers: []layers.Layer{
			layers.NewFullyConnectedLayer(0, to, 784, 256),
			layers.NewReluLayer(to),
			layers.NewFullyConnectedLayer(2, to, 256, 100),
			layers.NewReluLayer(to),
			layers.NewFullyConnectedLayer(4, to, 100, 100),
			layers.NewReluLayer(to),
			layers.NewFullyConnectedLayer(6, to, 100, 10),
		},
	}
	trainer := training.Trainer{
		DataSource: mnist.NewTrainingDataSource(to),
		Network:    network,
		LossFunc:   training.NewSoftmaxCrossEntropy(to),
		//OptimizationAlg: optimization.NewSGD(0.03),
		//OptimizationAlg: optimization.NewMomentum(0.1, 0.9),
		//OptimizationAlg: optimization.NewRMSProp(0.003),
		OptimizationAlg: optimization.NewAdam(to, 0.001),
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
