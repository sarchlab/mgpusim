package optimization

import "github.com/sarchlab/mgpusim/v4/benchmarks/dnn/tensor"

// Layer define the Layer interface used by the optimization algorithm.
type Layer interface {
	Parameters() tensor.Tensor
	Gradients() tensor.Tensor
}
