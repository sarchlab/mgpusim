package cp

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/monitoring2"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/dispatching"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/resource"
)

// defaultSpec provides the default configuration for the Command Processor.
var defaultSpec = Spec{
	Freq:           1 * timing.GHz,
	NumDispatchers: 8,
}

// DefaultSpec returns a copy of the default configuration. Callers typically
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// Builder can build Command Processors. Configuration is supplied as a whole
// through WithSpec; wiring is supplied through WithRegistrar. The component
// declares its ports; the port instances are supplied externally after Build
// with AssignPort.
type Builder struct {
	spec      Spec
	registrar modeling.Registrar
	visTracer tracing.Tracer
	monitor   *monitoring2.Monitor
	driver    messaging.RemotePort
	cus       []CUInterfaceForCP
}

// MakeBuilder creates a new builder with default configuration values.
func MakeBuilder() Builder {
	return Builder{spec: defaultSpec}
}

// WithRegistrar wires the builder to a registrar (a *simulation.Simulation in
// assembly, or modeling.NewStandaloneRegistrar(engine) in isolated tests).
func (b Builder) WithRegistrar(reg modeling.Registrar) Builder {
	b.registrar = reg
	return b
}

// WithSpec sets the entire configuration. Start from DefaultSpec() and tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

// WithVisTracer enables tracing for visualization on the command processor's
// dispatchers.
func (b Builder) WithVisTracer(tracer tracing.Tracer) Builder {
	b.visTracer = tracer
	return b
}

// WithMonitor sets the monitor used to show kernel-dispatching progress bars.
func (b Builder) WithMonitor(monitor *monitoring2.Monitor) Builder {
	b.monitor = monitor
	return b
}

// WithDriver sets the driver port that the command processor responds to.
func (b Builder) WithDriver(driver messaging.RemotePort) Builder {
	b.driver = driver
	return b
}

// WithCU adds a compute unit to the command processor.
func (b Builder) WithCU(cu CUInterfaceForCP) Builder {
	b.cus = append(b.cus, cu)
	return b
}

// Build builds a new Command Processor. It declares the component's ports
// ("ToDriver", "ToDMA", "ToCUs", "ToTLBs", "ToAddressTranslators",
// "ToCaches", "ToRDMA"); assign the port instances after Build with
// AssignPort.
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("cp: WithRegistrar is required")
	}

	comp := modeling.NewBuilder[Spec, State, modeling.None]().
		WithEngine(b.registrar.GetEngine()).
		WithFreq(b.spec.Freq).
		WithSpec(b.spec).
		Build(name)
	comp.State = State{
		Driver:                b.driver,
		BottomMemCopyH2DToTop: make(map[uint64]protocol.MemCopyH2DReq),
		BottomMemCopyD2HToTop: make(map[uint64]protocol.MemCopyD2HReq),
	}

	cpMW := &cpMiddleware{comp: comp}
	ctrlMW := &ctrlMiddleware{comp: comp}
	comp.AddMiddleware(cpMW)
	comp.AddMiddleware(ctrlMW)

	b.buildDispatchers(comp, cpMW)

	for _, cu := range b.cus {
		RegisterCU(comp, cu)
	}

	comp.DeclarePort("ToDriver")
	comp.DeclarePort("ToDMA")
	comp.DeclarePort("ToCUs")
	comp.DeclarePort("ToTLBs", memcontrolprotocol.Requester)
	comp.DeclarePort("ToAddressTranslators", memcontrolprotocol.Requester)
	comp.DeclarePort("ToCaches", memcontrolprotocol.Requester)
	comp.DeclarePort("ToRDMA")

	b.registrar.RegisterComponent(comp)

	return comp
}

func (b Builder) buildDispatchers(comp *Comp, cpMW *cpMiddleware) {
	cuResourcePool := resource.NewCUResourcePool()
	builder := dispatching.MakeBuilder().
		WithCP(comp).
		WithAlg("round-robin").
		WithCUResourcePool(cuResourcePool).
		WithPortSource(comp).
		WithDispatchingPortName("ToCUs").
		WithRespondingPortName("ToDriver").
		WithMonitor(b.monitor).
		WithConstantKernelLaunchOverhead(b.spec.ConstantKernelLaunchOverhead).
		WithSubsequentKernelLaunchOverhead(
			b.spec.SubsequentKernelLaunchOverhead).
		WithWGScalingThreshold(b.spec.WGScalingThreshold)

	if b.spec.ConstantKernelOverhead > 0 {
		builder = builder.WithConstantKernelOverhead(
			b.spec.ConstantKernelOverhead)
	}

	for i := 0; i < b.spec.NumDispatchers; i++ {
		disp := builder.Build(fmt.Sprintf("%s.Dispatcher%d", comp.Name(), i))

		if b.visTracer != nil {
			tracing.CollectTrace(disp, b.visTracer)
		}

		cpMW.dispatchers = append(cpMW.dispatchers, disp)
	}
}
