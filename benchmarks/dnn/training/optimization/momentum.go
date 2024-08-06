package optimization

import (
	"github.com/sarchlab/mgpusim/v4/benchmarks/dnn/tensor"
)

// Momentum runs a exponentially weighted averaging over gradients.
type Momentum struct {
	to tensor.Operator

	LearningRate float64
	SmoothFactor float64

	history map[Layer]tensor.Tensor
}

// NewMomentum creates a new Momentum SGD algorithm object.
func NewMomentum(
	to tensor.Operator,
	learningRate float64, smoothFactor float64) *Momentum {
	return &Momentum{
		to:           to,
		LearningRate: learningRate,
		SmoothFactor: smoothFactor,
		history:      make(map[Layer]tensor.Tensor),
	}
}

// UpdateParameters modifies the layer parameters using the momentum algorithm.
func (m *Momentum) UpdateParameters(layer Layer) {
	params := layer.Parameters()
	gradients := layer.Gradients()

	if params == nil || gradients == nil {
		return
	}

	velocity, found := m.history[layer]
	if !found {
		velocity = m.to.Clone(gradients)
		m.history[layer] = velocity
	}

	newVelocity := m.to.ScaleAdd(
		m.SmoothFactor, 1-m.SmoothFactor, velocity, gradients)
	m.to.Copy(velocity, newVelocity)
	m.to.Free(newVelocity)

	newParams := m.to.ScaleAdd(1, -m.LearningRate, params, velocity)
	m.to.Copy(params, newParams)
	m.to.Free(newParams)
}
