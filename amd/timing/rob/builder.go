package rob

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

var defaultSpec = Spec{
	Freq:           1 * timing.GHz,
	BufferSize:     128,
	NumReqPerCycle: 4,
}

// DefaultSpec returns a copy of the default reorder-buffer configuration.
// Callers typically take it, tweak the fields they care about, and pass the
// result to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// A Builder can build ReorderBuffers. Configuration is supplied as a whole
// through WithSpec; wiring is supplied through WithRegistrar. The component
// declares its "Top", "Bottom", and "Control" ports; the port instances are
// supplied externally after Build with AssignPort.
type Builder struct {
	spec      Spec
	registrar modeling.Registrar
}

// MakeBuilder creates a builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{spec: defaultSpec}
}

// WithRegistrar wires the builder to a registrar (a *simulation.Simulation in
// platform assembly, or modeling.NewStandaloneRegistrar(engine) in isolated
// tests). The registrar provides the engine and registers the built
// component.
func (b Builder) WithRegistrar(reg modeling.Registrar) Builder {
	b.registrar = reg
	return b
}

// WithSpec sets the entire configuration. Start from DefaultSpec() and tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

// Build creates a ReorderBuffer with the given name. It declares the
// component's Top, Bottom, and Control ports and registers the component;
// the port instances are assigned externally after Build with AssignPort.
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("rob: WithRegistrar is required")
	}

	comp := modeling.NewBuilder[Spec, State, modeling.None]().
		WithEngine(b.registrar.GetEngine()).
		WithFreq(b.spec.Freq).
		WithSpec(b.spec).
		Build(name)

	comp.State = State{}
	comp.AddMiddleware(&middleware{comp: comp})

	comp.DeclarePort("Top", memprotocol.Responder)
	comp.DeclarePort("Bottom", memprotocol.Requester)
	comp.DeclarePort("Control", memcontrolprotocol.Responder)

	b.registrar.RegisterComponent(comp)

	return comp
}
