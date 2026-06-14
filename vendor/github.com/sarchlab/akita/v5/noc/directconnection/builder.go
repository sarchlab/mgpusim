package directconnection

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"

	// Builder can help building directconnection.
	"github.com/sarchlab/akita/v5/messaging"
)

// defaultSpec provides the default configuration for a direct connection.
var defaultSpec = Spec{Freq: 1 * timing.GHz}

// DefaultSpec returns a copy of the default configuration. Callers obtain it,
// tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// Builder builds direct connections. A connection owns no ports (ports plug in)
// and has no resources, so it is configured by Spec alone and wired to the
// simulation through a registrar.
type Builder struct {
	spec      Spec
	registrar modeling.Registrar
}

func MakeBuilder() Builder {
	return Builder{spec: defaultSpec}
}

// WithRegistrar wires the builder to a registrar (a *simulation.Simulation in
// assembly, or modeling.NewStandaloneRegistrar(engine) in isolated tests). The
// registrar provides the engine and registers the built connection.
func (b Builder) WithRegistrar(reg modeling.Registrar) Builder {
	b.registrar = reg
	return b
}

// WithSpec sets the entire configuration. Start from DefaultSpec() and tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("directconnection: WithRegistrar is required")
	}

	engine := b.registrar.GetEngine()
	spec := b.spec

	modelComp := modeling.NewBuilder[Spec, State, modeling.None]().
		WithEngine(engine).
		WithFreq(spec.Freq).
		WithSpec(spec).
		Build(name)

	// DirectConnection uses secondary tick events so it runs after primary
	// components. Replace the primary TickingComponent created by the builder
	// with a secondary one. Since SerialEngine.RegisterHandler overwrites by
	// name, the final registration is for the secondary component. ✓
	modelComp.TickingComponent = modeling.NewSecondaryTickingComponent(
		name, engine, spec.Freq, modelComp)

	mw := &middleware{
		comp: modelComp,
		ports: ports{
			ports:   make([]messaging.Port, 0),
			portMap: make(map[messaging.RemotePort]int),
		},
	}
	modelComp.AddMiddleware(mw)

	conn := &Comp{Component: modelComp}

	b.registrar.RegisterConnection(conn)

	return conn
}
