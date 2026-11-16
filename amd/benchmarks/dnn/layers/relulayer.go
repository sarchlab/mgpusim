package layers

import (
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
)

// ReluLayer implements a Rectified Linear Unit.
type ReluLayer struct {
	*ReluLayerState
	to   tensor.Operator
	refs tensor.References
}

// NewReluLayer creates a new Relu layer.
func NewReluLayer(to tensor.Operator) *ReluLayer {
	refs := tensor.NewReferences(to)
	return &ReluLayer{ReluLayerState: &ReluLayerState{}, to: to, refs: refs}
}

// Randomize of the relu layer does nothing.
func (r *ReluLayer) Randomize() {
	// This function is intentionally left blank
}

// Forward calculates the forward propagation results.
func (r *ReluLayer) Forward(
	input tensor.Tensor,
) tensor.Tensor {
	r.ForwardInputID = r.refs.Replace(r.ForwardInputID, r.to.Clone(input))
	return r.to.ReluForward(input)
}

// Backward calculates the input gradients.
func (r *ReluLayer) Backward(input tensor.Tensor) tensor.Tensor {
	out := r.to.ReluBackward(r.refs.Resolve(r.ForwardInputID), input)
	r.to.Free(r.refs.Resolve(r.ForwardInputID))
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
