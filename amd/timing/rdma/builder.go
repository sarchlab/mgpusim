package rdma

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// defaultSpec provides the default configuration for the RDMA engine.
var defaultSpec = Spec{
	Freq:                1 * timing.GHz,
	BufferSize:          128,
	IncomingReqPerCycle: 1,
	IncomingRspPerCycle: 1,
	OutgoingReqPerCycle: 1,
	OutgoingRspPerCycle: 1,
}

// DefaultSpec returns a copy of the default configuration. Callers typically
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// Builder builds RDMA engines. Configuration is supplied as a whole through
// WithSpec; wiring is supplied through WithSimulation and WithResources. The
// component declares its "RDMARequestInside", "RDMARequestOutside",
// "RDMADataInside", "RDMADataOutside", and "Ctrl" ports; the port instances
// are supplied externally after Build with AssignPort.
type Builder struct {
	spec      Spec
	registrar modeling.Simulation
	resources Resources
}

// MakeBuilder returns a new Builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{spec: defaultSpec}
}

// WithSimulation wires the builder to a registrar (a *simulation.Simulation in
// assembly, or modeling.NewStandaloneRegistrar(engine) in isolated tests).
func (b Builder) WithSimulation(reg modeling.Simulation) Builder {
	b.registrar = reg
	return b
}

// WithSpec sets the entire configuration. Start from DefaultSpec() and tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

// WithResources injects the address-to-port mappers used to route requests to
// local modules and remote RDMA engines.
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// Build builds a new Comp. It declares the component's ports; assign the port
// instances after Build with AssignPort.
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("rdma: WithSimulation is required")
	}

	return modeling.MakeBuilder[*Comp](b.spec).WithSimulation(b.registrar).WithResources(b.resources).Build(name)
}
