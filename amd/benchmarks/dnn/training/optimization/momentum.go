package optimization

import (
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
)

// Momentum runs a exponentially weighted averaging over gradients.
type Momentum struct {
	*State
	to   tensor.Operator
	refs tensor.References
}

// NewMomentum creates a new Momentum SGD algorithm object.
func NewMomentum(to tensor.Operator, learningRate, smoothFactor float64) *Momentum {
	return &Momentum{
		State: &State{
			Kind:         "momentum",
			LearningRate: learningRate,
			SmoothFactor: smoothFactor,
			History:      make(map[tensor.ID]tensor.ID),
		},
		to:   to,
		refs: tensor.NewReferences(to),
	}
}

// UpdateParameters modifies the layer parameters using the momentum algorithm.
func (m *Momentum) UpdateParameters(layer Layer) {
	params := layer.Parameters()
	gradients := layer.Gradients()

	if params == nil || gradients == nil {
		return
	}

	key := m.refs.Reference(params)
	velocity := m.refs.Resolve(m.History[key])
	if velocity == nil {
		velocity = m.to.Clone(gradients)
		m.History[key] = m.refs.Reference(velocity)
	}

	newVelocity := m.to.ScaleAdd(
		m.SmoothFactor, 1-m.SmoothFactor, velocity, gradients)
	m.to.Copy(velocity, newVelocity)
	m.to.Free(newVelocity)

	newParams := m.to.ScaleAdd(1, -m.LearningRate, params, velocity)
	m.to.Copy(params, newParams)
	m.to.Free(newParams)
}
