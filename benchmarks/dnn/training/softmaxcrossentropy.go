package training

import (
	"github.com/sarchlab/mgpusim/v4/benchmarks/dnn/tensor"
)

// SoftmaxCrossEntropy can calculate the softmax and cross entropy together.
type SoftmaxCrossEntropy struct {
	to tensor.Operator
}

// NewSoftmaxCrossEntropy creates a SoftmaxCrossEntropy object.
func NewSoftmaxCrossEntropy(to tensor.Operator) *SoftmaxCrossEntropy {
	return &SoftmaxCrossEntropy{to: to}
}

// Loss calculates the loss and derivative.
func (s SoftmaxCrossEntropy) Loss(
	output tensor.Tensor,
	label []int,
) (
	loss float64,
	derivative tensor.Tensor,
) {
	softmax := s.to.Softmax(output)
	loss = s.to.CrossEntropy(softmax, label)
	derivative = s.to.SoftmaxCrossEntropyDerivative(output, label)

	s.to.Free(softmax)

	return loss, derivative
}
