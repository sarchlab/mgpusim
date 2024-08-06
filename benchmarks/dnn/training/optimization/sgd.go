package optimization

import (
	"github.com/sarchlab/mgpusim/v4/benchmarks/dnn/tensor"
)

// SGD is an optimizer that runs SGD algorithm
type SGD struct {
	to           tensor.Operator
	LearningRate float64
}

// NewSGD creates a new SGD object.
func NewSGD(to tensor.Operator, learningRate float64) *SGD {
	sgd := &SGD{
		to:           to,
		LearningRate: learningRate,
	}
	return sgd
}

// UpdateParameters modifies the layer parameter using the sgd algorithm.
func (s *SGD) UpdateParameters(layer Layer) {
	params := layer.Parameters()
	gradients := layer.Gradients()

	if params == nil || gradients == nil {
		return
	}

	newParams := s.to.ScaleAdd(1, -s.LearningRate, params, gradients)
	s.to.Copy(params, newParams)
	s.to.Free(newParams)
}
