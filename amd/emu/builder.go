package emu

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// defaultSpec is the default configuration of an emulation ComputeUnit.
var defaultSpec = Spec{
	Freq: 1 * timing.GHz,
}

// DefaultSpec returns a copy of the default configuration. Callers obtain
// it, tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// Builder builds emulation ComputeUnits. Configuration is supplied as a
// whole through WithSpec; shared references (decoder, ALU, storage accessor)
// through WithResources; wiring through WithSimulation. The component
// declares its "ToDispatcher" port; the port instance is supplied externally
// after Build with AssignPort.
type Builder struct {
	spec      Spec
	resources Resources
	registrar modeling.Simulation
}

// MakeBuilder creates a new Builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{spec: defaultSpec}
}

// WithSimulation wires the builder to a registrar (a *simulation.Simulation
// in assembly, or modeling.NewStandaloneRegistrar(engine) in isolated
// tests). The registrar provides the engine and registers the built
// component.
func (b Builder) WithSimulation(reg modeling.Simulation) Builder {
	b.registrar = reg
	return b
}

// WithSpec sets the entire configuration. Start from DefaultSpec() and
// tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

// WithResources sets the shared references that the ComputeUnit uses: the
// instruction decoder, the ALU, and the storage accessor.
func (b Builder) WithResources(resources Resources) Builder {
	b.resources = resources
	return b
}

// Build creates a new emulation ComputeUnit with the given name. It declares
// the component's "ToDispatcher" port; assign the port instance after Build
// with AssignPort.
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("emu: WithSimulation is required")
	}

	if b.resources.Decoder == nil ||
		b.resources.ALU == nil ||
		b.resources.StorageAccessor == nil {
		panic("emu: WithResources with Decoder, ALU, and " +
			"StorageAccessor is required")
	}

	if d, ok := b.resources.Decoder.(*insts.Disassembler); ok {
		b.spec.IsCDNA3 = d.IsCDNA3
	}
	return modeling.MakeBuilder[*Comp](b.spec).WithSimulation(b.registrar).WithResources(b.resources).Build(name)
}
