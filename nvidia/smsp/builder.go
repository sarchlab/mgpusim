package smsp

import (
	"fmt"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/tebeka/atexit"
)

type SMSPBuilder struct {
	engine sim.Engine
	freq   sim.Freq
}

func (b *SMSPBuilder) WithEngine(engine sim.Engine) *SMSPBuilder {
	b.engine = engine
	return b
}

func (b *SMSPBuilder) WithFreq(freq sim.Freq) *SMSPBuilder {
	b.freq = freq
	return b
}

func (b *SMSPBuilder) Build(name string) *SMSPController {
	s := &SMSPController{
		ID: sim.GetIDGenerator().Generate(),
	}

	s.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, s)
	s.toSM = sim.NewPort(s, 4, 4, fmt.Sprintf("%s.ToSM", name))
	s.AddPort(fmt.Sprintf("%s.ToSM", name), s.toSM)

	// cache updates
	s.ToGPUControllerMem = sim.NewPort(s, 4, 4, fmt.Sprintf("%s.ToGPUControllerMem", name))
	s.AddPort(fmt.Sprintf("%s.ToGPUControllerMem", name), s.ToGPUControllerMem)

	atexit.Register(s.LogStatus)

	return s
}
