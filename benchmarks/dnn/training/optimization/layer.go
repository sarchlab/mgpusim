package optimization

import "gitlab.com/akita/dnn/tensor"

// Layer define the Layer interface used by the optimization algorithm.
type Layer interface {
	Parameters() tensor.Tensor
	Gradients() tensor.Tensor
}
