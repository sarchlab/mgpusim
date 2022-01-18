package cp

import (
	"fmt"
	"math"

	"gitlab.com/akita/akita/v2/monitoring"
	"gitlab.com/akita/akita/v2/sim"
	"gitlab.com/akita/mgpusim/v2/protocol"
	"gitlab.com/akita/mgpusim/v2/timing/cp/internal/dispatching"
	"gitlab.com/akita/mgpusim/v2/timing/cp/internal/resource"
	"gitlab.com/akita/util/v2/akitaext"
	"gitlab.com/akita/util/v2/buffering"
	"gitlab.com/akita/util/v2/tracing"
)

// Builder can build Command Processors
type Builder struct {
	freq           sim.Freq
	engine         sim.Engine
	visTracer      tracing.Tracer
	monitor        *monitoring.Monitor
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

// WithVisTracer enables tracing for visualzation on the command processor and
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

// Build builds a new Command Processor
func (b Builder) Build(name string) *CommandProcessor {
	cp := new(CommandProcessor)
	cp.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, cp)

	unlimited := math.MaxInt32
	cp.ToDriver = sim.NewLimitNumMsgPort(cp, 1, name+".ToDriver")
	cp.toDriverSender = akitaext.NewBufferedSender(
		cp.ToDriver, buffering.NewBuffer(unlimited))
	cp.ToDMA = sim.NewLimitNumMsgPort(cp, 1, name+".ToDispatcher")
	cp.toDMASender = akitaext.NewBufferedSender(
		cp.ToDMA, buffering.NewBuffer(unlimited))
	cp.ToCUs = sim.NewLimitNumMsgPort(cp, 1, name+".ToCUs")
	cp.toCUsSender = akitaext.NewBufferedSender(
		cp.ToCUs, buffering.NewBuffer(unlimited))
	cp.ToTLBs = sim.NewLimitNumMsgPort(cp, 1, name+".ToTLBs")
	cp.toTLBsSender = akitaext.NewBufferedSender(
		cp.ToTLBs, buffering.NewBuffer(unlimited))
	cp.ToRDMA = sim.NewLimitNumMsgPort(cp, 1, name+".ToRDMA")
	cp.toRDMASender = akitaext.NewBufferedSender(
		cp.ToRDMA, buffering.NewBuffer(unlimited))
	cp.ToPMC = sim.NewLimitNumMsgPort(cp, 1, name+".ToPMC")
	cp.toPMCSender = akitaext.NewBufferedSender(
		cp.ToPMC, buffering.NewBuffer(unlimited))
	cp.ToAddressTranslators = sim.NewLimitNumMsgPort(cp, 1,
		name+".ToAddressTranslators")
	cp.toAddressTranslatorsSender = akitaext.NewBufferedSender(
		cp.ToAddressTranslators, buffering.NewBuffer(unlimited))
	cp.ToCaches = sim.NewLimitNumMsgPort(cp, 1, name+".ToCaches")
	cp.toCachesSender = akitaext.NewBufferedSender(
		cp.ToCaches, buffering.NewBuffer(unlimited))

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

	return cp
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
