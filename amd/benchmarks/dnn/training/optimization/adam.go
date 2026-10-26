package optimization

import (
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
)

// Adam runs the Adam optimization algorithm.
type Adam struct {
	*State
	to   tensor.Operator
	refs tensor.References
}

// NewAdam creates a new Adam optimization algorithm.
func NewAdam(to tensor.Operator, learningRate float64) *Adam {
	return &Adam{
		State: &State{
			Kind:          "adam",
			LearningRate:  learningRate,
			SmoothFactor1: 0.9,
			SmoothFactor2: 0.999,
			History:       make(map[tensor.ID]tensor.ID),
			HistoryV:      make(map[tensor.ID]tensor.ID),
		},
		to:   to,
		refs: tensor.NewReferences(to),
	}
}

// UpdateParameters update the layer parameters using the Adam algorithm.
func (r *Adam) UpdateParameters(layer Layer) {
	params := layer.Parameters()
	gradients := layer.Gradients()

	if params == nil || gradients == nil {
		return
	}

	key := r.refs.Reference(params)
	v := r.refs.Resolve(r.HistoryV[key])
	s := r.refs.Resolve(r.History[key])

	if s == nil {
		v = r.to.Clone(gradients)
		r.HistoryV[key] = r.refs.Reference(v)

		s = r.to.Clone(gradients)
		sSquare := r.to.ElementWiseMul(s, s)
		r.to.Free(s)

		s = sSquare
		r.History[key] = r.refs.Reference(s)
	}

	r.to.Adam(params, gradients, v, s,
		r.SmoothFactor1, r.SmoothFactor2, r.LearningRate)
}
