package subcore

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/tebeka/atexit"
)

type SubcoreBuilder struct {
	engine sim.Engine
	freq   sim.Freq
}

func (b *SubcoreBuilder) WithEngine(engine sim.Engine) *SubcoreBuilder {
	b.engine = engine
	return b
}

func (b *SubcoreBuilder) WithFreq(freq sim.Freq) *SubcoreBuilder {
	b.freq = freq
	return b
}

func (b *SubcoreBuilder) Build(name string) *Subcore {
	s := &Subcore{
		ID: sim.GetIDGenerator().Generate(),
	}

	s.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, s)
	s.toSM = sim.NewLimitNumMsgPort(s, 4, "ToSM")
	s.AddPort("ToSM", s.toSM)

	atexit.Register(s.LogStatus)

	return s
}
