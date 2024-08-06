package layers

import "github.com/sarchlab/mgpusim/v4/benchmarks/dnn/tensor"

// A Layer can do both forward and backward propagation.
type Layer interface {
	// Randomize creates random initial parameters for the layer.
	Randomize()

	// Forward performs forward propagation. It stores the input.
	Forward(input tensor.Tensor) tensor.Tensor

	// Backward performs backward propagation. It stores the gradient.
	Backward(input tensor.Tensor) tensor.Tensor

	// Parameters retrieves all the parameters of the layer
	Parameters() tensor.Tensor

	// Gradients retrieves all the gradients of the layer parameters.
	Gradients() tensor.Tensor
}
