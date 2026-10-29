package cp

import (
	"fmt"

	"github.com/sarchlab/mgpusim/v5/amd/sampling"

	monitoring2 "github.com/sarchlab/akita/v5/monitoring"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/dispatching"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/resource"
)

// DefaultSpec returns a copy of the default configuration. Callers typically
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return Spec{
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
}

// Builder can build Command Processors. Configuration is supplied as a whole
// through WithSpec; wiring is supplied through WithSimulation. The component
// declares its ports; the port instances are supplied externally after Build
// with BindPort.
type Builder struct {
	sampler   *sampling.SampledEngine
	spec      Spec
	registrar modeling.Simulation
	monitor   *monitoring2.Monitor
}

// WithSampler shares the simulation's recoverable wavefront sampling state.
func (b Builder) WithSampler(s *sampling.SampledEngine) Builder { b.sampler = s; return b }

// MakeBuilder creates a new builder with default configuration values.
func MakeBuilder() Builder {
	return Builder{spec: DefaultSpec()}
}

// WithSimulation supplies the simulation that owns the component.
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

// Build builds a new Command Processor. It declares the component's ports
// ("ToDriver", "ToDMA", "ToCUs",
// "ToCaches", "ToRDMA", "ToTranslation", "ToDataMover", "PageMigration");
// assign the port instances after Build with
// BindPort.
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("cp: WithSimulation is required")
	}

	return modeling.MakeBuilder[*Comp](b.spec).WithSimulation(b.registrar).
		WithResources(Resources{Sampler: b.sampler, Monitor: b.monitor}).Build(name)
}

func buildDispatchers(comp *Comp) []dispatching.Dispatcher {
	var dispatchers []dispatching.Dispatcher
	cuResourcePool := resource.Bind(comp.Spec.CUs, func() *[]resource.CUState { return &comp.State.CUResources })
	alg := comp.Spec.Alg
	if alg == "" {
		alg = "round-robin"
	}
	builder := dispatching.MakeBuilder().WithSampler(comp.Resources.Sampler).
		WithCP(comp).
		WithAlg(alg).
		WithNumDies(comp.Spec.NumDies).
		WithWavefrontDispatchCycles(comp.Spec.WavefrontDispatchCycles).
		WithCUResourcePool(cuResourcePool).
		WithPortSource(dispatcherPorts{comp}).
		WithDispatchingPortName("ToCUs").
		WithRespondingPortName("ToDriver").
		WithMonitor(comp.Resources.Monitor).
		WithConstantKernelLaunchOverhead(comp.Spec.ConstantKernelLaunchOverhead).
		WithSubsequentKernelLaunchOverhead(
			comp.Spec.SubsequentKernelLaunchOverhead).
		WithWGScalingThreshold(comp.Spec.WGScalingThreshold)

	if comp.Spec.ConstantKernelOverhead > 0 {
		builder = builder.WithConstantKernelOverhead(
			comp.Spec.ConstantKernelOverhead)
	}

	for i := 0; i < comp.Spec.NumDispatchers; i++ {
		index := i
		disp := builder.
			WithState(func() *dispatching.State { return &comp.State.Dispatchers[index] }).
			Build(fmt.Sprintf("%s.Dispatcher%d", comp.Name(), i))

		dispatchers = append(dispatchers, disp)
	}
	return dispatchers
}
