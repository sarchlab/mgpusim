package layers

import "github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"

// AvgPoolingLayer can perform avgpooling forward and backward propagation
// operations.
type AvgPoolingLayer struct {
	*AvgPoolingLayerState
	to   tensor.Operator
	refs tensor.References
}

// NewAvgPoolingLayer returns a newly created avgpooling layer.
func NewAvgPoolingLayer(
	to tensor.Operator,
	kernelSize, padding, stride []int,
) *AvgPoolingLayer {
	refs := tensor.NewReferences(to)
	return &AvgPoolingLayer{
		AvgPoolingLayerState: &AvgPoolingLayerState{KernelSize: kernelSize, Padding: padding, Stride: stride},
		to:                   to, refs: refs,
	}
}

// Forward can perform forward propagation operation.
func (l *AvgPoolingLayer) Forward(in tensor.Tensor) tensor.Tensor {
	l.ForwardInID = l.refs.Replace(l.ForwardInID, l.to.Clone(in))

	out := l.to.AvgPoolingForward(in, l.KernelSize, l.Padding, l.Stride)

	l.ForwardOutputSize = out.Size()

	return out
}

// Backward can perform backward propgation operation.
func (l *AvgPoolingLayer) Backward(in tensor.Tensor) tensor.Tensor {
	in.SetSize(l.ForwardOutputSize)

	out := l.to.AvgPoolingBackward(l.refs.Resolve(l.ForwardInID), in,
		l.KernelSize, l.Padding, l.Stride)

	l.to.Free(l.refs.Resolve(l.ForwardInID))

	return out
}

// Randomize does nother as avgpooling layers do not have parameters.
func (l *AvgPoolingLayer) Randomize() {
	// Do nothing
}

// Parameters returns a nil tensor as avgpooling layers do not have parameters.
func (l *AvgPoolingLayer) Parameters() tensor.Tensor {
	return nil
}

// Gradients returns a nil tensor as avgpooling layers do not have parameters.
func (l *AvgPoolingLayer) Gradients() tensor.Tensor {
	return nil
}
