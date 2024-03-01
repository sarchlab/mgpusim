package gpu

import (
	"fmt"

	"github.com/sarchlab/accelsimtracing/subcore"
	"github.com/sarchlab/akita/v3/sim"
)

type GPUBuilder struct {
	engine       sim.Engine
	freq         sim.Freq
	subcoreCount int64
}

func (b *GPUBuilder) WithEngine(engine sim.Engine) *GPUBuilder {
	b.engine = engine
	return b
}

func (b *GPUBuilder) WithFreq(freq sim.Freq) *GPUBuilder {
	b.freq = freq
	return b
}

func (b *GPUBuilder) WithSubcoreCount(count int64) *GPUBuilder {
	b.subcoreCount = count
	return b
}

func (b *GPUBuilder) Build(name string) *GPU {
	g := &GPU{
		subcoreCount: b.subcoreCount,
		subcores:     make([]*SubCoreInfo, b.subcoreCount),
	}

	g.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, g)
	g.toDriver = sim.NewLimitNumMsgPort(g, 4, "ToDriver")
	g.toSubcores = sim.NewLimitNumMsgPort(g, 4, "ToSubcores")
	g.connectionWithSubcores = sim.NewDirectConnection("ConnWithDevices", b.engine, b.freq)

	for i := int64(0); i < g.subcoreCount; i++ {
		p := sim.NewLimitNumMsgPort(g, 4, "ToSubcore")
		subcore := new(subcore.SubcoreBuilder).
			WithEngine(b.engine).
			WithFreq(b.freq).
			Build(fmt.Sprintf("Subcore(%d)", i))

		subcoreInfo := &SubCoreInfo{
			device:          subcore,
			toSubcoreRemote: subcore.toGPU,
		}

		g.subcores[i] = subcoreInfo
		g.freeSubcores = append(g.freeSubcores, i)
	}

	return g
}
