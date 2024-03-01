package subcore

import "github.com/sarchlab/akita/v3/sim"

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
	s := &Subcore{}
	s.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, s)
	s.toGPU = sim.NewLimitNumMsgPort(s, 4, "ToGPU")
	return s
}
