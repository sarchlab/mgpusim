package sm

import (
	"fmt"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"github.com/sarchlab/mgpusim/v4/nvidia/subcore"
	"github.com/tebeka/atexit"
)

type SMBuilder struct {
	engine sim.Engine
	freq   sim.Freq

	subcoresCount int64
}

func (b *SMBuilder) WithEngine(engine sim.Engine) *SMBuilder {
	b.engine = engine
	return b
}

func (b *SMBuilder) WithFreq(freq sim.Freq) *SMBuilder {
	b.freq = freq
	return b
}

func (b *SMBuilder) WithSubcoresCount(count int64) *SMBuilder {
	b.subcoresCount = count
	return b
}

func (b *SMBuilder) Build(name string) *SM {
	s := &SM{
		ID:       sim.GetIDGenerator().Generate(),
		Subcores: make(map[string]*subcore.Subcore),
	}

	s.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, s)
	b.buildPortsForSM(s, name)
	subcores := b.buildSubcores(name)
	b.connectSMwithSubcores(s, subcores)

	atexit.Register(s.LogStatus)

	return s
}

func (b *SMBuilder) buildPortsForSM(sm *SM, name string) {
	sm.toGPU = sim.NewPort(sm, 4, 4, fmt.Sprintf("%s.ToGPU", name))
	sm.toSubcores = sim.NewPort(sm, 4, 4, fmt.Sprintf("%s.ToSubcores", name))
	sm.AddPort(fmt.Sprintf("%s.ToGPU", name), sm.toGPU)
	sm.AddPort(fmt.Sprintf("%s.ToSubcores", name), sm.toSubcores)
}

func (b *SMBuilder) buildSubcores(smName string) []*subcore.Subcore {
	subcoreBuilder := new(subcore.SubcoreBuilder).
		WithEngine(b.engine).
		WithFreq(b.freq)
	subcores := []*subcore.Subcore{}
	for i := int64(0); i < b.subcoresCount; i++ {
		subcore := subcoreBuilder.Build(fmt.Sprintf("%s.Subcore(%d)", smName, i))
		subcores = append(subcores, subcore)
	}

	return subcores
}

func (b *SMBuilder) connectSMwithSubcores(sm *SM, subcores []*subcore.Subcore) {
	conn := directconnection.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(1 * sim.GHz).
		Build("SMToSubcores")

	conn.PlugIn(sm.toSubcores)

	for i := range subcores {
		subcore := subcores[i]

		sm.freeSubcores = append(sm.freeSubcores, subcore)
		sm.Subcores[subcore.ID] = subcore

		subcore.SetSMRemotePort(sm.toSubcores)
		conn.PlugIn(subcore.GetPortByName(fmt.Sprintf("%s.ToSM", subcore.Name())))
	}
}
