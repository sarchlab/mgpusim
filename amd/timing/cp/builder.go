package cp

import (
	"fmt"

	"github.com/sarchlab/mgpusim/v5/amd/sampling"

	monitoring2 "github.com/sarchlab/akita/v5/monitoring"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/dispatching"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/resource"
)

// defaultSpec provides the default configuration for the Command Processor.
var defaultSpec = Spec{
	Freq:           1 * timing.GHz,
	NumDispatchers: 8,
	Alg:            "round-robin",
	NumDies:        1,
	// Per-kernel constant overhead (cycles after all WGs complete). Kept
	// non-zero on purpose: a value of 0 lets otherwise-separated per-kernel
	// events coincide, which exposes a latent tie-break ordering in the
	// instruction-cache latency accounting and makes the deterministic test
	// see run-to-run differences. Platforms that want a different value set
	// spec.ConstantKernelOverhead; setting it explicitly to 0 opts out (the
	// value falls through to the dispatcher default, since cp/builder only
	// applies ConstantKernelOverhead when it is > 0).
	ConstantKernelOverhead: 3600,
}

// DefaultSpec returns a copy of the default configuration. Callers typically
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// Builder can build Command Processors. Configuration is supplied as a whole
// through WithSpec; wiring is supplied through WithSimulation. The component
// declares its ports; the port instances are supplied externally after Build
// with AssignPort.
type Builder struct {
	sampler   *sampling.SampledEngine
	spec      Spec
	registrar modeling.Simulation
	monitor   *monitoring2.Monitor
	driver    messaging.RemotePort
	cus       []CUInterfaceForCP
}

// WithSampler shares the simulation's recoverable wavefront sampling state.
func (b Builder) WithSampler(s *sampling.SampledEngine) Builder { b.sampler = s; return b }

// MakeBuilder creates a new builder with default configuration values.
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
		panic("cp: WithSimulation is required")
	}

	comp := modeling.MakeBuilder[*Comp](b.spec).WithSimulation(b.registrar).Build(name)
	comp.State = State{
		Driver:                b.driver,
		BottomMemCopyH2DToTop: make(map[uint64]messaging.Msg),
		BottomMemCopyD2HToTop: make(map[uint64]messaging.Msg),
		CopyRecvTasks:         make(map[uint64]uint64),
		CopySendTasks:         make(map[uint64]uint64),
	}
	comp.Resources.Sampler = b.sampler
	b.buildDispatchers(comp)
	for _, cu := range b.cus {
		RegisterCU(comp, cu)
	}

	return comp
}

func (b Builder) buildDispatchers(comp *Comp) {
	cuResourcePool := resource.NewCUResourcePool()
	cuResourcePool.SetName(comp.Name() + ".CUResources")
	b.registrar.RegisterResource(cuResourcePool)
	alg := b.spec.Alg
	if alg == "" {
		alg = "round-robin"
	}
	builder := dispatching.MakeBuilder().WithSampler(b.sampler).
		WithCP(comp).
		WithAlg(alg).
		WithNumDies(b.spec.NumDies).
		WithWavefrontDispatchCycles(b.spec.WavefrontDispatchCycles).
		WithCUResourcePool(cuResourcePool).
		WithPortSource(dispatcherPorts{comp}).
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

		b.registrar.RegisterResource(disp)
		comp.Resources.Dispatchers = append(comp.Resources.Dispatchers, disp)
	}
}
