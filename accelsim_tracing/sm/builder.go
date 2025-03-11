package sm

import (
	"fmt"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/subcore"
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
	b.buildPortsForSM(s)
	subcores := b.buildSubcores(name)
	b.connectSMwithSubcores(s, subcores)

	atexit.Register(s.LogStatus)

	return s
}

func (b *SMBuilder) buildPortsForSM(sm *SM) {
    // v3
    // sm.toGPU = sim.NewLimitNumMsgPort(sm, 4, "ToGPU")
    // sm.toSubcores = sim.NewLimitNumMsgPort(sm, 4, "ToSubcores")
	sm.toGPU = sim.NewPort(sm, 4, 4, "ToGPU")
	sm.toSubcores = sim.NewPort(sm, 4, 4, "ToSubcores")
	sm.AddPort("ToGPU", sm.toGPU)
	sm.AddPort("ToSubcores", sm.toSubcores)
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
	// v3
	// conn := sim.NewDirectConnection("SMToSubcores", b.engine, 1*sim.GHz)
	conn := directconnection.MakeBuilder().
        WithEngine(b.engine).
        WithFreq(1*sim.GHz).
        Build("SMToSubcores")

    // v3
    // conn.PlugIn(sm.toSubcores, 4)
	conn.PlugIn(sm.toSubcores)

	for i := range subcores {
		subcore := subcores[i]

		sm.freeSubcores = append(sm.freeSubcores, subcore)
		sm.Subcores[subcore.ID] = subcore

		subcore.SetSMRemotePort(sm.toSubcores)
		// v3
		// conn.PlugIn(subcore.GetPortByName("ToSM"), 4)
		conn.PlugIn(subcore.GetPortByName("ToSM"))
	}
}
