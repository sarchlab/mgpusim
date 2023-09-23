package main

import (
	"flag"
	"math"
	"math/rand"

	"gitlab.com/akita/dnn/tensor"
	"gitlab.com/akita/dnn/training/optimization"

	"gitlab.com/akita/dnn/dataset/imagenet"

	"gitlab.com/akita/dnn/layers"
	"gitlab.com/akita/dnn/training"
)

func main() {
	flag.Parse()
	rand.Seed(1)

	to := &tensor.CPUOperator{}

	network := getNetwork(to)
	trainer := training.Trainer{
		DataSource:      imagenet.NewTrainingDataSource(to),
		Network:         network,
		LossFunc:        training.NewSoftmaxCrossEntropy(to),
		OptimizationAlg: optimization.NewAdam(to, 0.001),
		Tester: &training.Tester{
			DataSource: imagenet.NewTestDataSource(to),
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

func getNetwork(to tensor.Operator) training.Network {
	return training.Network{
		Layers: []layers.Layer{
			layers.NewConv2D(0, to, []int{3, 224, 224}, []int{64, 3, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(2, to, []int{64, 224, 224}, []int{64, 64, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewMaxPoolingLayer(to, []int{2, 2}, []int{0, 0}, []int{2, 2}),

			layers.NewConv2D(5, to, []int{64, 112, 112}, []int{128, 64, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(7, to, []int{128, 112, 112}, []int{128, 128, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(9, to, []int{128, 112, 112}, []int{128, 128, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewMaxPoolingLayer(to, []int{2, 2}, []int{0, 0}, []int{2, 2}),

			layers.NewConv2D(12, to, []int{128, 56, 56}, []int{256, 128, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(14, to, []int{256, 56, 56}, []int{256, 256, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(16, to, []int{256, 56, 56}, []int{256, 256, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewMaxPoolingLayer(to, []int{2, 2}, []int{0, 0}, []int{2, 2}),

			layers.NewConv2D(17, to, []int{256, 28, 28}, []int{512, 256, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(19, to, []int{512, 28, 28}, []int{512, 512, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(21, to, []int{512, 28, 28}, []int{512, 512, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewMaxPoolingLayer(to, []int{2, 2}, []int{0, 0}, []int{2, 2}),

			layers.NewConv2D(24, to, []int{512, 14, 14}, []int{512, 512, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(26, to, []int{512, 14, 14}, []int{512, 512, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewConv2D(27, to, []int{512, 14, 14}, []int{512, 512, 3, 3}, []int{1, 1}, []int{1, 1}),
			layers.NewReluLayer(to),
			layers.NewMaxPoolingLayer(to, []int{2, 2}, []int{0, 0}, []int{2, 2}),

			layers.NewFullyConnectedLayer(30, to, 7*7*512, 2*2*512),
			layers.NewReluLayer(to),
			layers.NewFullyConnectedLayer(32, to, 2*2*512, 200),
		},
	}
}
