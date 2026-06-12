package idealmemcontroller

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// DefaultSpec provides default configuration for the ideal memory controller.
var DefaultSpec = Spec{
	Freq:          1 * sim.GHz,
	Latency:       100,
	Width:         1,
	CacheLineSize: 64,
}

// Builder builds ideal memory controller components.
type Builder struct {
	spec       Spec
	capacity   uint64
	engine     sim.EventScheduler
	topBufSize int
	storage    *mem.Storage
	topPort    sim.Port
	ctrlPort   sim.Port
}

// MakeBuilder returns a new Builder
func MakeBuilder() Builder {
	return Builder{
		spec:       DefaultSpec,
		capacity:   4 * mem.GB,
		topBufSize: 16,
	}
}

// WithSpec sets the spec of the memory controller. If the provided spec has
// a zero Freq, the builder's current Freq is preserved.
func (b Builder) WithSpec(spec Spec) Builder {
	prevFreq := b.spec.Freq
	b.spec = spec
	if b.spec.Freq == 0 {
		b.spec.Freq = prevFreq
	}
	return b
}

// WithFreq sets the frequency of the memory controller
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.spec.Freq = freq
	return b
}

// WithNewStorage sets the capacity of the memory controller
func (b Builder) WithNewStorage(capacity uint64) Builder {
	b.capacity = capacity
	return b
}

// WithEngine sets the engine of the memory controller
func (b Builder) WithEngine(engine sim.EventScheduler) Builder {
	b.engine = engine
	return b
}

// WithTopBufSize sets the size of the top buffer
func (b Builder) WithTopBufSize(topBufSize int) Builder {
	b.topBufSize = topBufSize
	return b
}

// WithStorage sets the storage of the memory controller
func (b Builder) WithStorage(storage *mem.Storage) Builder {
	b.storage = storage
	return b
}

// WithAddressConverter sets the address converter of the memory controller
func (b Builder) WithAddressConverter(
	addressConverter mem.AddressConverter,
) Builder {
	if ic, ok := addressConverter.(mem.InterleavingConverter); ok {
		b.spec.AddrConvKind = "interleaving"
		b.spec.AddrInterleavingSize = ic.InterleavingSize
		b.spec.AddrTotalNumOfElements = ic.TotalNumOfElements
		b.spec.AddrCurrentElementIndex = ic.CurrentElementIndex
		b.spec.AddrOffset = ic.Offset
	}

	return b
}

// WithTopPort sets the top port of the memory controller
func (b Builder) WithTopPort(port sim.Port) Builder {
	b.topPort = port
	return b
}

// WithCtrlPort sets the control port of the memory controller
func (b Builder) WithCtrlPort(port sim.Port) Builder {
	b.ctrlPort = port
	return b
}

// Build builds a new Comp
func (b Builder) Build(
	name string,
) *Comp {
	spec := b.spec
	spec.StorageRef = name

	initialState := State{
		CurrentState: "enable",
	}

	modelComp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(spec.Freq).
		WithSpec(spec).
		Build(name)
	modelComp.SetState(initialState)

	var storage *mem.Storage
	if b.storage == nil {
		storage = mem.NewStorage(b.capacity)
	} else {
		storage = b.storage
	}

	c := &Comp{
		Component: modelComp,
		storage:   storage,
	}

	ctrlMW := &ctrlMiddleware{comp: modelComp}
	modelComp.AddMiddleware(ctrlMW)
	memMW := &memMiddleware{comp: modelComp, storage: c.storage}
	modelComp.AddMiddleware(memMW)

	b.topPort.SetComponent(c)
	modelComp.AddPort("Top", b.topPort)
	b.ctrlPort.SetComponent(c)
	modelComp.AddPort("Control", b.ctrlPort)

	return c
}
