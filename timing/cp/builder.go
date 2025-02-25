package cp

import (
	"fmt"

	"github.com/sarchlab/akita/v4/analysis"
	"github.com/sarchlab/akita/v4/monitoring"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
	"github.com/sarchlab/mgpusim/v4/protocol"
	"github.com/sarchlab/mgpusim/v4/timing/cp/internal/dispatching"
	"github.com/sarchlab/mgpusim/v4/timing/cp/internal/resource"
)

// Builder can build Command Processors
type Builder struct {
	freq           sim.Freq
	engine         sim.Engine
	visTracer      tracing.Tracer
	monitor        *monitoring.Monitor
	perfAnalyzer   *analysis.PerfAnalyzer
	numDispatchers int
}

// MakeBuilder creates a new builder with default configuration values.
func MakeBuilder() Builder {
	b := Builder{
		freq:           1 * sim.GHz,
		numDispatchers: 8,
	}
	return b
}

// WithVisTracer enables tracing for visualization on the command processor and
// the dispatchers.
func (b Builder) WithVisTracer(tracer tracing.Tracer) Builder {
	b.visTracer = tracer
	return b
}

// WithEngine sets the even-driven simulation engine to use.
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency that the Command Processor works at.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithMonitor sets the monitor used to show progress bars.
func (b Builder) WithMonitor(monitor *monitoring.Monitor) Builder {
	b.monitor = monitor
	return b
}

// WithPerfAnalyzer sets the buffer analyzer used to analyze the
// command processor's buffers.
func (b Builder) WithPerfAnalyzer(
	analyzer *analysis.PerfAnalyzer,
) Builder {
	b.perfAnalyzer = analyzer
	return b
}

// Build builds a new Command Processor
func (b Builder) Build(name string) *CommandProcessor {
	cp := new(CommandProcessor)
	cp.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, cp)

	b.createPorts(cp, name)

	cp.bottomKernelLaunchReqIDToTopReqMap =
		make(map[string]*protocol.LaunchKernelReq)
	cp.bottomMemCopyH2DReqIDToTopReqMap =
		make(map[string]*protocol.MemCopyH2DReq)
	cp.bottomMemCopyD2HReqIDToTopReqMap =
		make(map[string]*protocol.MemCopyD2HReq)

	b.buildDispatchers(cp)

	if b.visTracer != nil {
		tracing.CollectTrace(cp, b.visTracer)
	}

	if b.perfAnalyzer != nil {
		b.perfAnalyzer.RegisterComponent(cp)
	}

	return cp
}

func (Builder) createPorts(cp *CommandProcessor, name string) {
	cp.ToDriver = sim.NewPort(cp, 1, 1, name+".ToDriver")
	cp.ToDMA = sim.NewPort(cp, 1, 1, name+".ToDispatcher")
	cp.ToCUs = sim.NewPort(cp, 1, 1, name+".ToCUs")
	cp.ToTLBs = sim.NewPort(cp, 1, 1, name+".ToTLBs")
	cp.ToRDMA = sim.NewPort(cp, 1, 1, name+".ToRDMA")
	cp.ToPMC = sim.NewPort(cp, 1, 1, name+".ToPMC")
	cp.ToAddressTranslators = sim.NewPort(cp, 1, 1,
		name+".ToAddressTranslators")
	cp.ToCaches = sim.NewPort(cp, 1, 1, name+".ToCaches")
}

func (b *Builder) buildDispatchers(cp *CommandProcessor) {
	cuResourcePool := resource.NewCUResourcePool()
	builder := dispatching.MakeBuilder().
		WithCP(cp).
		WithAlg("round-robin").
		WithCUResourcePool(cuResourcePool).
		WithDispatchingPort(cp.ToCUs).
		WithRespondingPort(cp.ToDriver).
		WithMonitor(b.monitor)

	for i := 0; i < b.numDispatchers; i++ {
		disp := builder.Build(fmt.Sprintf("%s.Dispatcher%d", cp.Name(), i))

		if b.visTracer != nil {
			tracing.CollectTrace(disp, b.visTracer)
		}

		cp.Dispatchers = append(cp.Dispatchers, disp)
	}
}
