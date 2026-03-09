package dispatching

import (
	"github.com/sarchlab/akita/v4/monitoring"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
	"github.com/sarchlab/mgpusim/v4/amd/kernels"
	"github.com/sarchlab/mgpusim/v4/amd/protocol"
	"github.com/sarchlab/mgpusim/v4/amd/timing/cp/internal/resource"
)

// A Builder can build dispatchers
type Builder struct {
	cp                           tracing.NamedHookable
	cuResourcePool               resource.CUResourcePool
	alg                          string
	respondingPort               sim.Port
	dispatchingPort              sim.Port
	monitor                      *monitoring.Monitor
	constantKernelOverhead         int
	constantKernelLaunchOverhead   int
	subsequentKernelLaunchOverhead int
}

// MakeBuilder creates a builder with default dispatching configurations.
func MakeBuilder() Builder {
	b := Builder{
		alg:                           "partition",
		constantKernelOverhead:         3600,
		subsequentKernelLaunchOverhead: 1800,
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

// WithConstantKernelOverhead sets the overhead cycles after all WGs complete.
func (b Builder) WithConstantKernelOverhead(overhead int) Builder {
	b.constantKernelOverhead = overhead
	return b
}

// WithConstantKernelLaunchOverhead sets the overhead cycles before first WG
// dispatch. This models the kernel launch latency on real hardware.
func (b Builder) WithConstantKernelLaunchOverhead(overhead int) Builder {
	b.constantKernelLaunchOverhead = overhead
	return b
}

// WithSubsequentKernelLaunchOverhead sets the overhead cycles for kernel
// launches after the first one. Back-to-back kernel launches benefit from
// warm instruction caches, pre-set page tables, and preserved CU state,
// so they can use a reduced overhead compared to the initial launch.
func (b Builder) WithSubsequentKernelLaunchOverhead(overhead int) Builder {
	b.subsequentKernelLaunchOverhead = overhead
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
			0,           // 0 WFs
			0, 0, 0, 0, // 1-4 WFs
			0, 0, 0, 0, // 5-8 WFs
			0, 0, 0, 0, // 9-12 WFs
			0, 0, 0, 0, // 13-16 WFs
		},
		constantKernelOverhead:         b.constantKernelOverhead,
		constantKernelLaunchOverhead:   b.constantKernelLaunchOverhead,
		subsequentKernelLaunchOverhead: b.subsequentKernelLaunchOverhead,
		monitor:                        b.monitor,
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
