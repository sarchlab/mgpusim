package layers

import (
	"gitlab.com/akita/dnn/tensor"
)

// ReluLayer implements a Rectified Linear Unit.
type ReluLayer struct {
	to           tensor.Operator
	forwardInput tensor.Tensor
}

// NewReluLayer creates a new Relu layer.
func NewReluLayer(to tensor.Operator) *ReluLayer {
	return &ReluLayer{to: to}
}

// Randomize of the relu layer does nothing.
func (r *ReluLayer) Randomize() {
	// This function is intentionally left blank
}

// Forward calculates the forward propagation results.
func (r *ReluLayer) Forward(
	input tensor.Tensor,
) tensor.Tensor {
	r.forwardInput = r.to.Clone(input)
	return r.to.ReluForward(input)
}

// Backward calculates the input gradients.
func (r *ReluLayer) Backward(input tensor.Tensor) tensor.Tensor {
	out := r.to.ReluBackward(r.forwardInput, input)
	r.to.Free(r.forwardInput)
	return out
}

// Parameters returns the parameter of the layer.
func (r ReluLayer) Parameters() tensor.Tensor {
	return nil
}

// Gradients returns the gradients of the layer.
func (r ReluLayer) Gradients() tensor.Tensor {
	return nil
}
