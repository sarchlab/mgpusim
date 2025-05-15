// Package timingconfig contains the configuration for the timing simulation.
package timingconfig

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/simulation"
)

// Builder builds a hardware platform for timing simulation.
type Builder struct {
	simulation *simulation.Simulation
}

// WithSimulation sets the simulation to use.
func (b Builder) WithSimulation(sim *simulation.Simulation) Builder {
	b.simulation = sim
	return b
}

// Build builds the hardware platform.
func (b Builder) Build() *sim.Domain {
	return &sim.Domain{}
}
