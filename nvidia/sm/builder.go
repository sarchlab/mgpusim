package sm

import (
	"fmt"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"

	"github.com/sarchlab/mgpusim/v4/nvidia/smsp"
	"github.com/tebeka/atexit"
)

type SMBuilder struct {
	name string

	engine sim.Engine
	freq   sim.Freq

	smspsCount        uint64
	log2CacheLineSize uint64
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

func (b *SMBuilder) Build(name string) *SM {
	s := &SM{
		ID:    sim.GetIDGenerator().Generate(),
		SMSPs: make(map[string]*smsp.SMSP),
	}
	b.name = name

	s.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, s)
	b.buildPortsForSM(s, name)
	smsps := b.buildSMSPs(name)
	b.connectSMwithSMSPs(s, smsps)

	// b.buildL1Caches(s)

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

// func (b *SMBuilder) buildL1Caches(sm *SM) {
// 	builder := writearound.NewBuilder().
// 		WithEngine(b.engine).
// 		WithFreq(b.freq).
// 		WithBankLatency(60).
// 		WithNumBanks(1).
// 		WithLog2BlockSize(b.log2CacheLineSize).
// 		WithWayAssociativity(4).
// 		WithNumMSHREntry(16).
// 		WithTotalByteSize(16 * mem.KB)

// 	// if b.visTracer != nil {
// 	// 	builder = builder.WithVisTracer(b.visTracer)
// 	// }

// 	// for i := 0; i < b.numCU; i++ {
// 	// 	name := fmt.Sprintf("%s.L1VCache[%d]", b.name, i)
// 	// 	cache := builder.Build(name)
// 	// 	sa.l1vCaches = append(sa.l1vCaches, cache)

// 	// 	if b.memTracer != nil {
// 	// 		tracing.CollectTrace(cache, b.memTracer)
// 	// 	}
// 	// }
// 	for i := 0; i < int(b.smspsCount); i++ {
// 		name := fmt.Sprintf("%s.L1VCache[%d]", b.name, i)
// 		cache := builder.Build(name)
// 		sm.l1Caches = append(sm.l1Caches, cache)

// 		// if b.memTracer != nil {
// 		// 	tracing.CollectTrace(cache, b.memTracer)
// 		// }
// 	}
// }
