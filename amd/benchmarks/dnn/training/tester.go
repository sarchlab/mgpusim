package training

import "github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/tensor"

// Tester runs a forward propagation and tests the overall accuracy.
type Tester struct {
	DataSource DataSource
	Network    Network
	BatchSize  int
}

// Test tests on all the data.
func (t Tester) Test() float64 {
	t.DataSource.Rewind()
	data, label := t.DataSource.NextBatch(t.BatchSize)

	var output, input tensor.Tensor
	output = data
	for _, l := range t.Network.Layers {
		input = output
		output = l.Forward(input)
	}

	correctCount := 0
	for i := 0; i < output.Size()[0]; i++ {
		start := i * output.Size()[1]
		end := start + output.Size()[1]
		probArray := output.Vector()[start:end]

		maxProb := 0.0
		maxProbIndex := -1
		for j := 0; j < output.Size()[1]; j++ {
			if probArray[j] > maxProb {
				maxProb = probArray[j]
				maxProbIndex = j
			}
		}

		if maxProbIndex == label[i] {
			correctCount++
		}
	}

	return float64(correctCount) / float64(output.Size()[0])
}
