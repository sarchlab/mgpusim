package layers

import "github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"

// MaxPoolingLayer can perform maxpooling forward and backward propagation
// operations.
type MaxPoolingLayer struct {
	*MaxPoolingLayerState
	to   tensor.Operator
	refs tensor.References
}

// NewMaxPoolingLayer returns a newly created maxpooling layer.
func NewMaxPoolingLayer(
	to tensor.Operator,
	kernelSize, padding, stride []int,
) *MaxPoolingLayer {
	refs := tensor.NewReferences(to)
	return &MaxPoolingLayer{
		MaxPoolingLayerState: &MaxPoolingLayerState{KernelSize: kernelSize, Padding: padding, Stride: stride},
		to:                   to, refs: refs,
	}
}

// Forward can perform forward propagation operation.
func (l *MaxPoolingLayer) Forward(in tensor.Tensor) tensor.Tensor {
	l.ForwardInID = l.refs.Replace(l.ForwardInID, l.to.Clone(in))

	out, mask := l.to.MaxPoolingForward(in, l.KernelSize, l.Padding, l.Stride)

	l.ForwardMaskID = l.refs.Replace(l.ForwardMaskID, mask)
	l.ForwardOutputSize = out.Size()

	return out
}

// Backward can perform backward propgation operation.
func (l *MaxPoolingLayer) Backward(in tensor.Tensor) tensor.Tensor {
	in.SetSize(l.ForwardOutputSize)

	out := l.to.MaxPoolingBackward(l.refs.Resolve(l.ForwardInID), in, l.refs.Resolve(l.ForwardMaskID),
		l.KernelSize, l.Padding, l.Stride)

	l.to.Free(l.refs.Resolve(l.ForwardInID))
	l.to.Free(l.refs.Resolve(l.ForwardMaskID))

	return out
}

// Randomize does nother as maxpooling layer does not have parameters.
func (l *MaxPoolingLayer) Randomize() {
	// Do nothing
}

// Parameters returns a nil tensor as maxpooling layer does not have parameters.
func (l *MaxPoolingLayer) Parameters() tensor.Tensor {
	return nil
}

// Gradients returns a nil tensor as maxpooling layer does not have parameters.
func (l *MaxPoolingLayer) Gradients() tensor.Tensor {
	return nil
}
