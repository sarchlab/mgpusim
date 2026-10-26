package optimization

import (
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
)

// RMSProp runs RMSProp optimization algorithm.
type RMSProp struct {
	*State
	to   tensor.Operator
	refs tensor.References
}

// NewRMSProp creates a new RMSProp optimization algorithm.
func NewRMSProp(to tensor.Operator, learningRate float64) *RMSProp {
	return &RMSProp{
		State: &State{
			Kind:         "rmsprop",
			LearningRate: learningRate,
			SmoothFactor: 0.9,
			History:      make(map[tensor.ID]tensor.ID),
		},
		to:   to,
		refs: tensor.NewReferences(to),
	}
}

// UpdateParameters modifies the layer parameters using the RMSProp algorithm.
func (r *RMSProp) UpdateParameters(layer Layer) {
	params := layer.Parameters()
	gradients := layer.Gradients()

	if params == nil || gradients == nil {
		return
	}

	key := r.refs.Reference(params)
	s := r.refs.Resolve(r.History[key])
	if s == nil {
		s = r.to.Clone(gradients)
		sSquare := r.to.ElementWiseMul(s, s)
		r.to.Free(s)

		s = sSquare
		r.History[key] = r.refs.Reference(s)
	}

	r.to.RMSProp(params, gradients, s, r.SmoothFactor, r.LearningRate)
}
