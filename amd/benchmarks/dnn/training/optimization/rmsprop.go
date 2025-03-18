package optimization

import (
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/tensor"
)

// RMSProp runs RMSProp optimization algorithm.
type RMSProp struct {
	to tensor.Operator

	LearningRate float64
	SmoothFactor float64

	history map[Layer]tensor.Tensor
}

// NewRMSProp creates a new RMSProp optimization algorithm.
func NewRMSProp(to tensor.Operator, learningRate float64) *RMSProp {
	return &RMSProp{
		to:           to,
		LearningRate: learningRate,
		SmoothFactor: 0.9,
		history:      make(map[Layer]tensor.Tensor),
	}
}

// UpdateParameters modifies the layer parameters using the RMSProp algorithm.
func (r *RMSProp) UpdateParameters(layer Layer) {
	params := layer.Parameters()
	gradients := layer.Gradients()

	if params == nil || gradients == nil {
		return
	}

	s, found := r.history[layer]
	if !found {
		s = r.to.Clone(gradients)
		sSquare := r.to.ElementWiseMul(s, s)
		r.to.Free(s)

		s = sSquare
		r.history[layer] = s
	}

	r.to.RMSProp(params, gradients, s, r.SmoothFactor, r.LearningRate)
}
