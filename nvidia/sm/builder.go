package sm

import (
	"fmt"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"github.com/sarchlab/mgpusim/v4/nvidia/l1cache"
	"github.com/sarchlab/mgpusim/v4/nvidia/smsp"
	"github.com/tebeka/atexit"
)

type SMBuilder struct {
	engine sim.Engine
	freq   sim.Freq

	smspsCount uint64
	l1Cache    l1cache.L1Cache
}

func (b *SMBuilder) WithEngine(engine sim.Engine) *SMBuilder {
	b.engine = engine
	return b
}

func (b *SMBuilder) WithFreq(freq sim.Freq) *SMBuilder {
	b.freq = freq
	return b
}

func (b *SMBuilder) WithSMSPsCount(count uint64) *SMBuilder {
	b.smspsCount = count
	return b
}

func (b *SMBuilder) WithL1Cache(l1Cache l1cache.L1Cache) *SMBuilder {
	b.l1Cache = l1Cache
	return b
}

func (b *SMBuilder) Build(name string) *SM {
	s := &SM{
		ID:    sim.GetIDGenerator().Generate(),
		SMSPs: make(map[string]*smsp.SMSP),
	}

	s.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, s)
	b.buildPortsForSM(s, name)
	smsps := b.buildSMSPs(name)
	b.connectSMwithSMSPs(s, smsps)

	atexit.Register(s.LogStatus)

	return s
}

func (b *SMBuilder) buildPortsForSM(sm *SM, name string) {
	sm.toGPU = sim.NewPort(sm, 4, 4, fmt.Sprintf("%s.ToGPU", name))
	sm.toSMSPs = sim.NewPort(sm, 4, 4, fmt.Sprintf("%s.ToSMSPs", name))
	sm.AddPort(fmt.Sprintf("%s.ToGPU", name), sm.toGPU)
	sm.AddPort(fmt.Sprintf("%s.ToSMSPs", name), sm.toSMSPs)
}

func (b *SMBuilder) buildSMSPs(smName string) []*smsp.SMSP {
	smspBuilder := new(smsp.SMSPBuilder).
		WithEngine(b.engine).
		WithFreq(b.freq)
	smsps := []*smsp.SMSP{}
	for i := uint64(0); i < b.smspsCount; i++ {
		smsp := smspBuilder.Build(fmt.Sprintf("%s.SMSP(%d)", smName, i))
		smsps = append(smsps, smsp)
	}

	return smsps
}

func (b *SMBuilder) connectSMwithSMSPs(sm *SM, smsps []*smsp.SMSP) {
	conn := directconnection.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(1 * sim.GHz).
		Build("SMToSMSPs")

	conn.PlugIn(sm.toSMSPs)

	for i := range smsps {
		smsp := smsps[i]

		sm.freeSMSPs = append(sm.freeSMSPs, smsp)
		sm.SMSPs[smsp.ID] = smsp

		smsp.SetSMRemotePort(sm.toSMSPs)
		conn.PlugIn(smsp.GetPortByName(fmt.Sprintf("%s.ToSM", smsp.Name())))
	}
}
