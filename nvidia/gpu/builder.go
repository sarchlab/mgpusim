package gpu

import (
	"fmt"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"github.com/sarchlab/mgpusim/v4/nvidia/sm"
	"github.com/tebeka/atexit"
)

type GPUBuilder struct {
	engine sim.Engine
	freq   sim.Freq

	smsCount        uint64
	smspsCountPerSM uint64
}

func (b *GPUBuilder) WithEngine(engine sim.Engine) *GPUBuilder {
	b.engine = engine
	return b
}

func (b *GPUBuilder) WithFreq(freq sim.Freq) *GPUBuilder {
	b.freq = freq
	return b
}

func (b *GPUBuilder) WithSMsCount(count uint64) *GPUBuilder {
	b.smsCount = count
	return b
}

func (b *GPUBuilder) WithSMSPsCountPerSM(count uint64) *GPUBuilder {
	b.smspsCountPerSM = count
	return b
}

func (b *GPUBuilder) Build(name string) *GPU {
	g := &GPU{
		ID:  sim.GetIDGenerator().Generate(),
		SMs: make(map[string]*sm.SM),
	}

	g.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, g)
	b.buildPortsForGPU(g, name)
	sms := b.buildSMs(name)
	b.connectGPUWithSMs(g, sms)

	atexit.Register(g.LogStatus)

	return g
}

func (b *GPUBuilder) buildPortsForGPU(g *GPU, name string) {
	g.toDriver = sim.NewPort(g, 4, 4, fmt.Sprintf("%s.ToDriver", name))
	g.toSMs = sim.NewPort(g, 4, 4, fmt.Sprintf("%s.ToSMs", name))
	g.AddPort(fmt.Sprintf("%s.ToDriver", name), g.toDriver)
	g.AddPort(fmt.Sprintf("%s.ToSMs", name), g.toSMs)
}

func (b *GPUBuilder) buildSMs(gpuName string) []*sm.SM {
	smBuilder := new(sm.SMBuilder).
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithSMSPsCount(b.smspsCountPerSM)

	sms := []*sm.SM{}
	for i := uint64(0); i < b.smsCount; i++ {
		sm := smBuilder.Build(fmt.Sprintf("%s.SM(%d)", gpuName, i))
		sms = append(sms, sm)
	}

	return sms
}

func (b *GPUBuilder) connectGPUWithSMs(gpu *GPU, sms []*sm.SM) {
	// 	conn := sim.NewDirectConnection("GPUToSMs", b.engine, 1*sim.GHz)
	// conn.PlugIn(gpu.toSMs, 4)
	conn := directconnection.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(1 * sim.GHz).
		Build("GPUToSMs")
	conn.PlugIn(gpu.toSMs)

	for i := range sms {
		sm := sms[i]

		gpu.freeSMs = append(gpu.freeSMs, sm)
		gpu.SMs[sm.ID] = sm

		sm.SetGPURemotePort(gpu.toSMs)

		conn.PlugIn(sm.GetPortByName(fmt.Sprintf("%s.ToGPU", sms[i].Name())))
	}
}
