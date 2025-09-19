package smsp

import (
	"fmt"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/simulation"
	"github.com/tebeka/atexit"
)

type SMSPBuilder struct {
	simulation       *simulation.Simulation
	engine           sim.Engine
	freq             sim.Freq
	warpIssueLatency uint64
}

func (b *SMSPBuilder) WithEngine(engine sim.Engine) *SMSPBuilder {
	b.engine = engine
	return b
}

func (b *SMSPBuilder) WithFreq(freq sim.Freq) *SMSPBuilder {
	b.freq = freq
	return b
}

func (b *SMSPBuilder) WithSimulation(sim *simulation.Simulation) *SMSPBuilder {
	b.simulation = sim
	return b
}

func (b *SMSPBuilder) WithWarpIssueLatency(latency uint64) *SMSPBuilder {
	b.warpIssueLatency = latency
	return b
}

func (b *SMSPBuilder) Build(name string) *SMSPController {
	s := &SMSPController{
		ID:                        sim.GetIDGenerator().Generate(),
		warpIssueLatency:          b.warpIssueLatency,
		warpIssueLatencyRemaining: b.warpIssueLatency,
	}

	s.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, s)
	s.toSM = sim.NewPort(s, 4096, 4096, fmt.Sprintf("%s.ToSM", name))
	s.AddPort(fmt.Sprintf("%s.ToSM", name), s.toSM)

	// cache updates
	// s.ToMem = sim.NewPort(s, 4096, 4096, fmt.Sprintf("%s.ToMem", name))
	// s.AddPort(fmt.Sprintf("%s.ToMem", name), s.ToMem)

	s.ToVectorMem = sim.NewPort(s, 4096, 4096, fmt.Sprintf("%s.ToVectorMem", name))
	s.AddPort(fmt.Sprintf("%s.ToVectorMem", name), s.ToVectorMem)

	// s.waitingCycle = 0

	s.PendingSMSPtoMemReadReq = make(map[string]*mem.ReadReq)
	s.PendingSMSPtoMemWriteReq = make(map[string]*mem.WriteReq)
	s.PendingSMSPMemMsgID2Warp = make(map[string]*SMSPWarpUnit)

	atexit.Register(s.LogStatus)

	return s
}
