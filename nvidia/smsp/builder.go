package smsp

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

const portBufSize = 4096

// DefaultSpec returns the default component configuration.
func DefaultSpec() Spec {
	return Spec{Freq: 1 * timing.GHz}
}

// SMSPBuilder builds SMSPs.
type SMSPBuilder struct {
	registrar              modeling.Simulation
	freq                   timing.Freq
	log2CacheLineSize      uint64
	MemResponseHandleWidth uint64
}

// WithSimulation sets the simulation that the SMSP and its ports register to.
func (b *SMSPBuilder) WithSimulation(r modeling.Simulation) *SMSPBuilder {
	b.registrar = r
	return b
}

// WithFreq sets the frequency of the SMSP.
func (b *SMSPBuilder) WithFreq(freq timing.Freq) *SMSPBuilder {
	b.freq = freq
	return b
}

// WithLog2CacheLineSize sets the cache line size used to split memory
// accesses.
func (b *SMSPBuilder) WithLog2CacheLineSize(n uint64) *SMSPBuilder {
	b.log2CacheLineSize = n
	return b
}

// WithMemResponseHandleWidth sets how many memory responses the SMSP can
// process per cycle.
func (b *SMSPBuilder) WithMemResponseHandleWidth(w uint64) *SMSPBuilder {
	b.MemResponseHandleWidth = w
	return b
}

// Build creates an SMSP with the given name. The SMSP has two ports, "ToSM"
// and "ToVectorMem".
func (b *SMSPBuilder) Build(name string) *SMSPController {
	t := &topology{ID: name}
	spec := Spec{Freq: b.freq, Log2CacheLineSize: b.log2CacheLineSize, MemResponseHandleWidth: b.MemResponseHandleWidth}
	c := modeling.MakeBuilder[*Comp](spec).WithSimulation(b.registrar).
		WithResources(Resources{Topology: t}).Build(name)
	s := &SMSPController{Comp: c, topology: t}
	b.buildPort(s, "ToSM")
	b.buildPort(s, "ToVectorMem")

	return s
}

func (b *SMSPBuilder) buildPort(comp *SMSPController, name string) messaging.Port {
	port := twowaybuffered.MakeBuilder().
		WithSimulation(b.registrar).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: portBufSize, OutgoingCapacity: portBufSize}).
		Build()
	comp.BindPort(name, port)
	return port
}
