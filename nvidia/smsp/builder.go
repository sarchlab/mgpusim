package smsp

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

const portBufSize = 4096

// SMSPBuilder builds SMSPs.
type SMSPBuilder struct {
	registrar              modeling.Registrar
	freq                   timing.Freq
	log2CacheLineSize      uint64
	MemResponseHandleWidth uint64
}

// WithRegistrar sets the simulation that the SMSP and its ports register to.
func (b *SMSPBuilder) WithRegistrar(r modeling.Registrar) *SMSPBuilder {
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
	s := &SMSPController{
		ID:                     name,
		scheduler:              NewSMSPScheduler(),
		ResourcePool:           NewH100SMSPResourcePool(),
		inflightMemPipelines:   make(map[uint64]*PipelineInstance),
		log2CacheLineSize:      b.log2CacheLineSize,
		MemResponseHandleWidth: b.MemResponseHandleWidth,
	}
	s.TickingComponent = modeling.NewTickingComponent(
		name, b.registrar.GetEngine(), b.freq, s)
	b.registrar.RegisterComponent(s)

	s.toSM = b.buildPort(s, "ToSM")
	s.toVectorMem = b.buildPort(s, "ToVectorMem")

	return s
}

func (b *SMSPBuilder) buildPort(
	s *SMSPController,
	name string,
) messaging.Port {
	s.DeclarePort(name)
	port := modeling.MakePortBuilder().
		WithRegistrar(b.registrar).
		WithComponent(s).
		WithSpec(modeling.PortSpec{BufSize: portBufSize}).
		Build(name)
	s.AssignPort(name, port)

	return port
}
