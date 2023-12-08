package optimization

import (
	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/tensor"
)

// Adam runs the Adam optimization algorithm.
type Adam struct {
	to tensor.Operator

	LearningRate  float64
	SmoothFactor1 float64
	SmoothFactor2 float64

	historyV map[Layer]tensor.Tensor
	historyS map[Layer]tensor.Tensor
}

// NewAdam creates a new Adam optimization algorithm.
func NewAdam(to tensor.Operator, learningRate float64) *Adam {
	return &Adam{
		to:            to,
		LearningRate:  learningRate,
		SmoothFactor1: 0.9,
		SmoothFactor2: 0.999,
		historyV:      make(map[Layer]tensor.Tensor),
		historyS:      make(map[Layer]tensor.Tensor),
	}
}

// UpdateParameters update the layer parameters using the Adam algorithm.
func (r *Adam) UpdateParameters(layer Layer) {
	params := layer.Parameters()
	gradients := layer.Gradients()

	if params == nil || gradients == nil {
		return
	}

	v := r.historyV[layer]
	s, found := r.historyS[layer]

	if !found {
		v = r.to.Clone(gradients)
		r.historyV[layer] = v

		s = r.to.Clone(gradients)
		sSquare := r.to.ElementWiseMul(s, s)
		r.to.Free(s)

		s = sSquare
		r.historyS[layer] = s
	}

	r.to.Adam(params, gradients, v, s,
		r.SmoothFactor1, r.SmoothFactor2, r.LearningRate)
}
