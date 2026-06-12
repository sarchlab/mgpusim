package emu

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
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
// through WithResources; wiring through WithRegistrar. The component
// declares its "ToDispatcher" port; the port instance is supplied externally
// after Build with AssignPort.
type Builder struct {
	spec      Spec
	resources Resources
	registrar modeling.Registrar
}

// MakeBuilder creates a new Builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{spec: defaultSpec}
}

// WithRegistrar wires the builder to a registrar (a *simulation.Simulation
// in assembly, or modeling.NewStandaloneRegistrar(engine) in isolated
// tests). The registrar provides the engine and registers the built
// component.
func (b Builder) WithRegistrar(reg modeling.Registrar) Builder {
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
		panic("emu: WithRegistrar is required")
	}

	if b.resources.Decoder == nil ||
		b.resources.ALU == nil ||
		b.resources.StorageAccessor == nil {
		panic("emu: WithResources with Decoder, ALU, and " +
			"StorageAccessor is required")
	}

	processor := &cuProcessor{
		// TODO(akita5): state purity — the wavefront map, the queued
		// MapWGReqs, and the decoded-instruction cache hold pointers and
		// therefore live on the processor instead of the component State.
		queuedWGs: make([]protocol.MapWGReq, 0),
		wfs:       make(map[*kernels.WorkGroup][]*Wavefront),
		instCache: make(map[uint64]*insts.Inst),
	}

	comp := modeling.NewEventDrivenBuilder[Spec, State, Resources]().
		WithEngine(b.registrar.GetEngine()).
		WithSpec(b.spec).
		WithResources(b.resources).
		WithProcessor(processor).
		Build(name)

	comp.DeclarePort(DispatchPortName)

	b.registrar.RegisterComponent(comp)

	return comp
}
