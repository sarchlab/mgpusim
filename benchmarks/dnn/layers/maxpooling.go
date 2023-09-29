package layers

import "github.com/sarchlab/mgpusim/v3/benchmarks/dnn/tensor"

// MaxPoolingLayer can perform maxpooling forward and backward propagation
// operations.
type MaxPoolingLayer struct {
	to tensor.Operator

	kernelSize []int
	padding    []int
	stride     []int

	forwardIn         tensor.Tensor
	forwardMask       tensor.Tensor
	forwardOutputSize []int
}

// NewMaxPoolingLayer returns a newly created maxpooling layer.
func NewMaxPoolingLayer(
	to tensor.Operator,
	kernelSize, padding, stride []int,
) *MaxPoolingLayer {
	return &MaxPoolingLayer{
		to:         to,
		kernelSize: kernelSize,
		padding:    padding,
		stride:     stride,
	}
}

// Forward can perform forward propagation operation.
func (l *MaxPoolingLayer) Forward(in tensor.Tensor) tensor.Tensor {
	l.forwardIn = l.to.Clone(in)

	out, mask := l.to.MaxPoolingForward(in, l.kernelSize, l.padding, l.stride)

	l.forwardMask = mask
	l.forwardOutputSize = out.Size()

	return out
}

// Backward can perform backward propgation operation.
func (l *MaxPoolingLayer) Backward(in tensor.Tensor) tensor.Tensor {
	in.SetSize(l.forwardOutputSize)

	out := l.to.MaxPoolingBackward(l.forwardIn, in, l.forwardMask,
		l.kernelSize, l.padding, l.stride)

	l.to.Free(l.forwardIn)
	l.to.Free(l.forwardMask)

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
