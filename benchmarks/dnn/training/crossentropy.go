package training

import (
	"gitlab.com/akita/dnn/tensor"
)

// CrossEntropy calculates cross entropy loss function.
type CrossEntropy struct {
	to tensor.Operator
}

// NewCrossEntropy creates a new CrossEntropy object.
func NewCrossEntropy(to tensor.Operator) *CrossEntropy {
	ce := &CrossEntropy{
		to: to,
	}

	return ce
}

// Loss calculates the loss and the return derivative
func (c CrossEntropy) Loss(
	output tensor.Tensor,
	label []int,
) (
	loss float64,
	derivative tensor.Tensor,
) {
	loss = c.to.CrossEntropy(output, label)
	derivative = c.to.CrossEntropyDerivative(output, label)

	return loss, derivative
}
