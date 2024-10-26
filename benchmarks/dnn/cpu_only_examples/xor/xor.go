package main

import (
	"math/rand"

	"github.com/sarchlab/mgpusim/v4/benchmarks/dnn/tensor"
	"github.com/sarchlab/mgpusim/v4/benchmarks/dnn/training/optimization"

	"github.com/sarchlab/mgpusim/v4/benchmarks/dnn/layers"
	"github.com/sarchlab/mgpusim/v4/benchmarks/dnn/training"
)

func main() {
	rand.Seed(1)
	to := tensor.CPUOperator{}

	network := training.Network{
		Layers: []layers.Layer{
			layers.NewFullyConnectedLayer(0, to, 2, 4),
			layers.NewReluLayer(to),
			layers.NewFullyConnectedLayer(2, to, 4, 2),
		},
	}
	trainer := training.Trainer{
		DataSource:      NewDataSource(to),
		Network:         network,
		LossFunc:        training.NewSoftmaxCrossEntropy(to),
		OptimizationAlg: optimization.NewSGD(to, 0.03),
		Epoch:           50,
		BatchSize:       4,
		ShowBatchInfo:   true,
	}

	for _, l := range network.Layers {
		l.Randomize()
	}

	trainer.Train()
}
