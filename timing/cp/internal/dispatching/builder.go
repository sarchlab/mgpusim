package dispatching

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mgpusim/kernels"
	"gitlab.com/akita/mgpusim/protocol"
	"gitlab.com/akita/mgpusim/timing/cp/internal/resource"
	"gitlab.com/akita/util/tracing"
)

// A Builder can build dispatchers
type Builder struct {
	cp              tracing.NamedHookable
	cuResourcePool  resource.CUResourcePool
	alg             string
	showProgressBar bool
	respondingPort  akita.Port
	dispatchingPort akita.Port
}

// MakeBuilder creates a builder with default dispatching configureations.
func MakeBuilder() Builder {
	b := Builder{
		alg: "round-robin",
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
func (b Builder) WithRespondingPort(p akita.Port) Builder {
	b.respondingPort = p
	return b
}

// WithDispatchingPort sets the port that connects to the Compute Units.
func (b Builder) WithDispatchingPort(p akita.Port) Builder {
	b.dispatchingPort = p
	return b
}

// WithProgressBar enables progress bar.
func (b Builder) WithProgressBar() Builder {
	b.showProgressBar = true
	return b
}

// WithAlg sets the dispatching algorithm.
func (b Builder) WithAlg(alg string) Builder {
	switch alg {
	case "round-robin":
		b.alg = alg
	default:
		panic("unknown dispatching algorithm " + alg)
	}

	return b
}

// Build creates a dispatcher.
func (b Builder) Build(name string) Dispatcher {
	d := &DispatcherImpl{
		name:            name,
		cp:              b.cp,
		showProgressBar: b.showProgressBar,
		respondingPort:  b.respondingPort,
		dispatchingPort: b.dispatchingPort,
		inflightWGs:     make(map[string]dispatchLocation),
		originalReqs:    make(map[string]*protocol.MapWGReq),
		latencyTable: []int{
			3,
			3, 3, 3, 3,
			4, 5, 6, 7,
			8, 9, 10, 11,
			12, 13, 14, 15,
			16, 17, 18, 19,
		},
		constantKernelOverhead: 1600,
	}

	switch b.alg {
	case "round-robin":
		d.alg = &roundRobinAlgorithm{
			gridBuilder: kernels.NewGridBuilder(),
			cuPool:      b.cuResourcePool,
		}
	default:
		panic("unknown dispatching algorithm " + b.alg)
	}

	return d
}
