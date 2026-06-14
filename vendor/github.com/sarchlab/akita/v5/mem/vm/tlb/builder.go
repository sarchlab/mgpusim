package tlb

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
)

// defaultSpec provides the default configuration for TLB components.
var defaultSpec = Spec{
	Freq:           1 * timing.GHz,
	NumReqPerCycle: 4,
	NumSets:        1,
	NumWays:        32,
	Log2PageSize:   12,
	PageSize:       4096,
	MSHRSize:       4,
	Latency:        4,
}

// DefaultSpec returns a copy of the default configuration. Callers typically
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// A Builder can build TLBs. Configuration is supplied as a whole through
// WithSpec; wiring is supplied through WithRegistrar and WithResources. The
// component declares its "Top", "Bottom", and "Control" ports; the port
// instances are supplied externally after Build with AssignPort (the caller
// chooses the buffer sizes).
type Builder struct {
	spec      Spec
	registrar modeling.Registrar
	resources Resources
}

// MakeBuilder returns a Builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{
		spec: defaultSpec,
	}
}

// WithRegistrar wires the builder to a registrar (a *simulation.Simulation in
// assembly, or modeling.NewStandaloneRegistrar(engine) in isolated tests). The
// registrar provides the engine and registers the built component.
func (b Builder) WithRegistrar(reg modeling.Registrar) Builder {
	b.registrar = reg
	return b
}

// WithSpec sets the entire configuration. Start from DefaultSpec() and tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

// WithResources injects the component's external wiring, in particular the
// translation provider mapper used to locate the remote port that serves the
// translation for a given virtual address.
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// Build creates a new TLB. It declares the component's Top, Bottom, and Control
// ports and registers the component; the port instances are assigned externally
// after Build with AssignPort (the caller chooses the buffer sizes).
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("tlb: WithRegistrar is required")
	}

	spec := b.spec
	if spec.Log2PageSize != 0 {
		spec.PageSize = 1 << spec.Log2PageSize
	}
	spec.PipelineWidth = spec.NumReqPerCycle

	initialState := State{
		TLBState: tlbStateEnable,
		Sets:     initSets(spec.NumSets, spec.NumWays),
		Pipeline: queueing.NewPipeline[pipelineTLBReqState](
			spec.PipelineWidth,
			spec.Latency,
		),
		BufferItems: queueing.NewBuffer[pipelineTLBReqState](
			name+".BufferItems",
			spec.PipelineWidth,
		),
	}

	modelComp := modeling.NewBuilder[Spec, State, Resources]().
		WithEngine(b.registrar.GetEngine()).
		WithFreq(spec.Freq).
		WithSpec(spec).
		WithResources(b.resources).
		Build(name)
	modelComp.State = initialState

	ctrlMW := &ctrlMiddleware{comp: modelComp}
	modelComp.AddMiddleware(ctrlMW)

	tlbMW := &tlbMiddleware{comp: modelComp}
	modelComp.AddMiddleware(tlbMW)

	modelComp.DeclarePort("Top", vmprotocol.Responder)
	modelComp.DeclarePort("Bottom", vmprotocol.Requester)
	modelComp.DeclarePort("Control", memcontrolprotocol.Responder)

	b.registrar.RegisterComponent(modelComp)

	return modelComp
}
