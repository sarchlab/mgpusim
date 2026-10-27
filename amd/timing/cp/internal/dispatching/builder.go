package dispatching

import (
	monitoring2 "github.com/sarchlab/akita/v5/monitoring"
	"github.com/sarchlab/mgpusim/v5/amd/sampling"

	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/resource"
)

// A Builder can build dispatchers
type Builder struct {
	state                          func() *State
	sampler                        *sampling.SampledEngine
	cp                             Parent
	cuResourcePool                 resource.CUResourcePool
	alg                            string
	portSource                     PortSource
	respondingPortName             string
	dispatchingPortName            string
	monitor                        *monitoring2.Monitor
	constantKernelOverhead         int
	constantKernelLaunchOverhead   int
	subsequentKernelLaunchOverhead int
	wgScalingThreshold             int
	numDies                        int
	wavefrontDispatchCycles        int
}

// WithSampler shares the simulation's recoverable wavefront sampling state.
func (b Builder) WithSampler(s *sampling.SampledEngine) Builder { b.sampler = s; return b }

// MakeBuilder creates a builder with default dispatching configurations.
func MakeBuilder() Builder {
	b := Builder{
		alg: "partition",
		// Default to no fixed kernel overhead. A platform sets a real value via
		// the CP spec (cp/builder.go only applies ConstantKernelOverhead when >0,
		// so this default is what spec==0 falls through to).
		constantKernelOverhead:         0,
		subsequentKernelLaunchOverhead: 1800,
		wgScalingThreshold:             128,
		numDies:                        1,
		wavefrontDispatchCycles:        2,
	}
	return b
}

// WithCP sets the Command Processor that the Dispatcher belongs to.
func (b Builder) WithCP(cp Parent) Builder {
	b.cp = cp
	return b
}

// WithCUResourcePool sets the CU resource pool. It has to be given form
// outside, as all the dispatchers share the same CU resource pool.
func (b Builder) WithCUResourcePool(pool resource.CUResourcePool) Builder {
	b.cuResourcePool = pool
	return b
}

// WithPortSource sets the object (typically the Command Processor component)
// that the dispatcher uses to resolve its ports by name. Ports are resolved
// lazily, since port instances are assigned to the component after Build.
func (b Builder) WithPortSource(ps PortSource) Builder {
	b.portSource = ps
	return b
}

// WithRespondingPortName sets the name of the port that the dispatcher sends
// kernel-completion responses through.
func (b Builder) WithRespondingPortName(name string) Builder {
	b.respondingPortName = name
	return b
}

// WithDispatchingPortName sets the name of the port that connects to the
// Compute Units.
func (b Builder) WithDispatchingPortName(name string) Builder {
	b.dispatchingPortName = name
	return b
}

// WithAlg sets the dispatching algorithm.
func (b Builder) WithAlg(alg string) Builder {
	switch alg {
	case "round-robin", "greedy", "partition", "per-die":
		b.alg = alg
	default:
		panic("unknown dispatching algorithm " + alg)
	}

	return b
}

// WithNumDies sets the number of dies (XCDs) the per-die algorithm dispatches
// across in parallel. Only used by the "per-die" algorithm.
func (b Builder) WithNumDies(n int) Builder {
	b.numDies = n
	return b
}

// WithWavefrontDispatchCycles sets the per-die dispatch cost charged per
// wavefront, in cycles. A W-wavefront work-group occupies its die's dispatch
// pipe for W*cycles before that die can dispatch the next work-group. Only used
// by the "per-die" algorithm.
func (b Builder) WithWavefrontDispatchCycles(cycles int) Builder {
	b.wavefrontDispatchCycles = cycles
	return b
}

// WithMonitor sets the monitor that manages progress bars.
func (b Builder) WithMonitor(monitor *monitoring2.Monitor) Builder {
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

// WithWGScalingThreshold sets the threshold for WG-count-based scaling of
// subsequent kernel launch overhead. When the previous kernel had more WGs
// than this threshold, the overhead is scaled down proportionally.
func (b Builder) WithWGScalingThreshold(n int) Builder {
	b.wgScalingThreshold = n
	return b
}

// WithState supplies the parent-owned canonical state accessor.
func (b Builder) WithState(state func() *State) Builder { b.state = state; return b }

// Build creates a dispatcher.
func (b Builder) Build(name string) Dispatcher {
	if b.state == nil {
		s := NewState(b.alg, b.numDies)
		b.state = func() *State { return &s }
	}
	d := &DispatcherImpl{
		state:               b.state,
		sampler:             b.sampler,
		name:                name,
		cp:                  b.cp,
		portSource:          b.portSource,
		respondingPortName:  b.respondingPortName,
		dispatchingPortName: b.dispatchingPortName,
		latencyTable: []int{
			0,          // 0 WFs
			0, 0, 0, 0, // 1-4 WFs
			0, 0, 0, 0, // 5-8 WFs
			0, 0, 0, 0, // 9-12 WFs
			0, 0, 0, 0, // 13-16 WFs
		},
		constantKernelOverhead:         b.constantKernelOverhead,
		constantKernelLaunchOverhead:   b.constantKernelLaunchOverhead,
		subsequentKernelLaunchOverhead: b.subsequentKernelLaunchOverhead,
		wgScalingThreshold:             b.wgScalingThreshold,
		monitor:                        b.monitor,
	}

	switch b.alg {
	case "round-robin":
		d.alg = &roundRobinAlgorithm{
			gridBuilder: &stateGrid{ids: b.cp, state: func() *kernels.GridBuilderState { return &b.state().Algorithm.Grid }},
			state:       func() *AlgorithmState { return &b.state().Algorithm },
			cuPool:      b.cuResourcePool,
		}
	case "greedy":
		d.alg = &greedyAlgorithm{
			gridBuilder: &stateGrid{ids: b.cp, state: func() *kernels.GridBuilderState { return &b.state().Algorithm.Grid }},
			state:       func() *AlgorithmState { return &b.state().Algorithm },
			cuPool:      b.cuResourcePool,
		}
	case "partition":
		d.alg = &partitionAlgorithm{
			ids:    b.cp,
			state:  func() *AlgorithmState { return &b.state().Algorithm },
			cuPool: b.cuResourcePool,
		}
	case "per-die":
		d.alg = &perDieAlgorithm{
			ids:     b.cp,
			state:   func() *AlgorithmState { return &b.state().Algorithm },
			cuPool:  b.cuResourcePool,
			numDies: b.numDies,
		}
	default:
		panic("unknown dispatching algorithm " + b.alg)
	}

	if da, ok := d.alg.(dieAwareAlgorithm); ok {
		d.dieAware = da
		d.wavefrontDispatchCycles = b.wavefrontDispatchCycles
	}

	return d
}

// NewState initializes a dispatcher before any runtime work is admitted.
func NewState(alg string, dies int) State {
	s := State{Inflight: make(map[uint64]Location), WgTaskIDs: make(map[uint64]uint64)}
	if alg == "per-die" {
		s.DieCyclesLeft = make([]int, dies)
	}
	return s
}
