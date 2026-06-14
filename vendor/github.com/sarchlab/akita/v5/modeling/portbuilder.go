package modeling

import (
	"github.com/sarchlab/akita/v5/messaging"
)

// PortSpec configures a port built by a PortBuilder.
type PortSpec struct {
	// BufSize is the capacity of both the incoming and outgoing buffers.
	BufSize int
}

// defaultPortSpec is the default port configuration.
var defaultPortSpec = PortSpec{BufSize: 1}

// DefaultPortSpec returns a copy of the default port configuration. Callers
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultPortSpec() PortSpec {
	return defaultPortSpec
}

// PortBuilder builds a messaging.Port and registers it with the registrar,
// mirroring how component and connection builders register themselves. The
// component owns its port topology (declared with DeclarePort); a PortBuilder
// supplies an instance for one of those ports. Build returns the port; attach
// it to the component with comp.AssignPort(name, port).
type PortBuilder struct {
	registrar Registrar
	comp      messaging.Component
	spec      PortSpec
}

// MakePortBuilder returns a PortBuilder seeded with the default spec.
func MakePortBuilder() PortBuilder {
	return PortBuilder{spec: defaultPortSpec}
}

// WithRegistrar wires the builder to a registrar (a *simulation.Simulation in
// assembly, or modeling.NewStandaloneRegistrar(engine) in isolated tests). The
// registrar registers the built port.
func (b PortBuilder) WithRegistrar(reg Registrar) PortBuilder {
	b.registrar = reg
	return b
}

// WithComponent sets the component that owns the port.
func (b PortBuilder) WithComponent(comp messaging.Component) PortBuilder {
	b.comp = comp
	return b
}

// WithSpec sets the port configuration. Start from DefaultPortSpec() and tweak.
func (b PortBuilder) WithSpec(spec PortSpec) PortBuilder {
	b.spec = spec
	return b
}

// Build builds a port whose full name is comp.Name()+"."+name, owned by the
// component, and registers it with the registrar. It returns the port; attach
// it to the component with comp.AssignPort(name, port).
func (b PortBuilder) Build(name string) messaging.Port {
	if b.registrar == nil {
		panic("modeling: PortBuilder requires a registrar")
	}

	if b.comp == nil {
		panic("modeling: PortBuilder requires a component")
	}

	port := messaging.NewPort(
		b.comp, b.spec.BufSize, b.spec.BufSize, b.comp.Name()+"."+name)
	b.registrar.RegisterPort(port)

	return port
}
