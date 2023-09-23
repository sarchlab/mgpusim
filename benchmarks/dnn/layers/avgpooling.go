package layers

import "gitlab.com/akita/dnn/tensor"

// AvgPoolingLayer can perform avgpooling forward and backward propagation
// operations.
type AvgPoolingLayer struct {
	to tensor.Operator

	kernelSize []int
	padding    []int
	stride     []int

	forwardIn         tensor.Tensor
	forwardOutputSize []int
}

// NewAvgPoolingLayer returns a newly created avgpooling layer.
func NewAvgPoolingLayer(
	to tensor.Operator,
	kernelSize, padding, stride []int,
) *AvgPoolingLayer {
	return &AvgPoolingLayer{
		to:         to,
		kernelSize: kernelSize,
		padding:    padding,
		stride:     stride,
	}
}

// Forward can perform forward propagation operation.
func (l *AvgPoolingLayer) Forward(in tensor.Tensor) tensor.Tensor {
	l.forwardIn = l.to.Clone(in)

	out := l.to.AvgPoolingForward(in, l.kernelSize, l.padding, l.stride)

	l.forwardOutputSize = out.Size()

	return out
}

// Backward can perform backward propgation operation.
func (l *AvgPoolingLayer) Backward(in tensor.Tensor) tensor.Tensor {
	in.SetSize(l.forwardOutputSize)

	out := l.to.AvgPoolingBackward(l.forwardIn, in,
		l.kernelSize, l.padding, l.stride)

	l.to.Free(l.forwardIn)

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
