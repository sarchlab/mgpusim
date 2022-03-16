package dispatching

import (
	"gitlab.com/akita/akita/v3/monitoring"
	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/akita/v3/tracing"
	"gitlab.com/akita/mgpusim/v2/kernels"
	"gitlab.com/akita/mgpusim/v2/protocol"
	"gitlab.com/akita/mgpusim/v2/timing/cp/internal/resource"
)

// A Builder can build dispatchers
type Builder struct {
	cp              tracing.NamedHookable
	cuResourcePool  resource.CUResourcePool
	alg             string
	respondingPort  sim.Port
	dispatchingPort sim.Port
	monitor         *monitoring.Monitor
}

// MakeBuilder creates a builder with default dispatching configureations.
func MakeBuilder() Builder {
	b := Builder{
		alg: "partition",
	}
	return b
}

// WithCP sets the Command Processor that the Dispatcher belongs to.
func (b Builder) WithCP(cp tracing.NamedHookable) Builder {
	b.cp = cp
	return b
}

// WithCUResourcePool sets the CU resource pool. It has to be given form
// outside, as all the dispatchers share the same CU resource pool.
func (b Builder) WithCUResourcePool(pool resource.CUResourcePool) Builder {
	b.cuResourcePool = pool
	return b
}

// WithRespondingPort sets the port that the dispatcher can send WFCompleteMsg
// to.
func (b Builder) WithRespondingPort(p sim.Port) Builder {
	b.respondingPort = p
	return b
}

// WithDispatchingPort sets the port that connects to the Compute Units.
func (b Builder) WithDispatchingPort(p sim.Port) Builder {
	b.dispatchingPort = p
	return b
}

// WithAlg sets the dispatching algorithm.
func (b Builder) WithAlg(alg string) Builder {
	switch alg {
	case "round-robin", "greedy", "partition":
		b.alg = alg
	default:
		panic("unknown dispatching algorithm " + alg)
	}

	return b
}

// WithMonitor sets the monitor that manages progress bars.
func (b Builder) WithMonitor(monitor *monitoring.Monitor) Builder {
	b.monitor = monitor
	return b
}

// Build creates a dispatcher.
func (b Builder) Build(name string) Dispatcher {
	d := &DispatcherImpl{
		name:            name,
		cp:              b.cp,
		respondingPort:  b.respondingPort,
		dispatchingPort: b.dispatchingPort,
		inflightWGs:     make(map[string]dispatchLocation),
		originalReqs:    make(map[string]*protocol.MapWGReq),
		latencyTable: []int{
			1,
			4, 4, 4, 4,
			5, 6, 7, 8,
			9, 10, 11, 12,
			13, 14, 15, 16,
		},
		constantKernelOverhead: 1600,
		monitor:                b.monitor,
	}

	switch b.alg {
	case "round-robin":
		d.alg = &roundRobinAlgorithm{
			gridBuilder: kernels.NewGridBuilder(),
			cuPool:      b.cuResourcePool,
		}
	case "greedy":
		d.alg = &greedyAlgorithm{
			gridBuilder: kernels.NewGridBuilder(),
			cuPool:      b.cuResourcePool,
		}
	case "partition":
		d.alg = &partitionAlgorithm{
			cuPool: b.cuResourcePool,
		}
	default:
		panic("unknown dispatching algorithm " + b.alg)
	}

	return d
}
