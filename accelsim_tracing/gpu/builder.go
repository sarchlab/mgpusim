package gpu

import (
	"fmt"

	"github.com/sarchlab/accelsimtracing/sm"
	"github.com/sarchlab/akita/v3/sim"
)

type GPUBuilder struct {
	engine sim.Engine
	freq   sim.Freq

	smsCount           int64
	subcoresCountPerSM int64
}

func (b *GPUBuilder) WithEngine(engine sim.Engine) *GPUBuilder {
	b.engine = engine
	return b
}

func (b *GPUBuilder) WithFreq(freq sim.Freq) *GPUBuilder {
	b.freq = freq
	return b
}

func (b *GPUBuilder) WithSMsCount(count int64) *GPUBuilder {
	b.smsCount = count
	return b
}

func (b *GPUBuilder) WithSubcoresCountPerSM(count int64) *GPUBuilder {
	b.subcoresCountPerSM = count
	return b
}

func (b *GPUBuilder) Build(name string) *GPU {
	g := &GPU{
		ID:  sim.GetIDGenerator().Generate(),
		sms: make(map[string]*sm.SM),
	}

	g.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, g)
	b.buildPortsForGPU(g)
	sms := b.buildSMs(name)
	b.connectGPUWithSMs(g, sms)

	return g
}

func (b *GPUBuilder) buildPortsForGPU(g *GPU) {
	g.toDriver = sim.NewLimitNumMsgPort(g, 4, "ToDriver")
	g.toSMs = sim.NewLimitNumMsgPort(g, 4, "ToSMs")
	g.AddPort("ToDriver", g.toDriver)
	g.AddPort("ToSMs", g.toSMs)
}

func (b *GPUBuilder) buildSMs(gpuName string) []*sm.SM {
	smBuilder := new(sm.SMBuilder).
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithSubcoresCount(b.subcoresCountPerSM)

	sms := []*sm.SM{}
	for i := int64(0); i < b.smsCount; i++ {
		sm := smBuilder.Build(fmt.Sprintf("%s.SM(%d)", gpuName, i))
		sms = append(sms, sm)
	}

	return sms
}

func (b *GPUBuilder) connectGPUWithSMs(gpu *GPU, sms []*sm.SM) {
	conn := sim.NewDirectConnection("GPUToSMs", b.engine, 1*sim.GHz)
	conn.PlugIn(gpu.toSMs, 4)

	for i := range sms {
		sm := sms[i]

		gpu.freeSMs = append(gpu.freeSMs, sm)
		gpu.sms[sm.ID] = sm

		sm.SetGPURemotePort(gpu.toSMs)
		conn.PlugIn(sm.GetPortByName("ToGPU"), 4)
	}
}
