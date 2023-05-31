package addresstranslator

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/mem"
)

// A Builder can create address translators
type Builder struct {
	engine              sim.Engine
	freq                sim.Freq
	translationProvider sim.Port
	ctrlPort            sim.Port
	lowModuleFinder     mem.LowModuleFinder
	numReqPerCycle      int
	log2PageSize        uint64
	deviceID            uint64
}

// MakeBuilder creates a new builder
func MakeBuilder() Builder {
	return Builder{
		freq:           1 * sim.GHz,
		numReqPerCycle: 4,
		log2PageSize:   12,
		deviceID:       1,
	}
}

// WithEngine sets the engine to be used by the address translators
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency of the address translators
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithTranslationProvider sets the port that can provide the translation
// service. The port must be a port on a TLB or an MMU.
func (b Builder) WithTranslationProvider(p sim.Port) Builder {
	b.translationProvider = p
	return b
}

// WithLowModuleFinder sets the low modules finder that can tell the address
// translators where to send the memory access request to.
func (b Builder) WithLowModuleFinder(f mem.LowModuleFinder) Builder {
	b.lowModuleFinder = f
	return b
}

// WithNumReqPerCycle sets the number of request the address translators can
// process in each cycle.
func (b Builder) WithNumReqPerCycle(n int) Builder {
	b.numReqPerCycle = n
	return b
}

// WithLog2PageSize sets the page size as a power of 2
func (b Builder) WithLog2PageSize(n uint64) Builder {
	b.log2PageSize = n
	return b
}

// WithDeviceID sets the GPU ID that the address translator belongs to
func (b Builder) WithDeviceID(n uint64) Builder {
	b.deviceID = n
	return b
}

// WithCtrlPort sets the port of the component that can send ctrl reqs to AT
func (b Builder) WithCtrlPort(p sim.Port) Builder {
	b.ctrlPort = p
	return b
}

// Build returns a new AddressTranslator
func (b Builder) Build(name string) *AddressTranslator {
	t := &AddressTranslator{}
	t.TickingComponent = sim.NewTickingComponent(
		name, b.engine, b.freq, t)

	b.createPorts(name, t)

	t.translationProvider = b.translationProvider
	t.lowModuleFinder = b.lowModuleFinder
	t.numReqPerCycle = b.numReqPerCycle
	t.log2PageSize = b.log2PageSize
	t.deviceID = b.deviceID

	return t
}

func (b Builder) createPorts(name string, t *AddressTranslator) {
	t.topPort = sim.NewLimitNumMsgPort(t, b.numReqPerCycle,
		name+".TopPort")
	t.AddPort("Top", t.topPort)

	t.bottomPort = sim.NewLimitNumMsgPort(t, b.numReqPerCycle,
		name+".BottomPort")
	t.AddPort("Bottom", t.bottomPort)

	t.translationPort = sim.NewLimitNumMsgPort(t, b.numReqPerCycle,
		name+".TranslationPort")
	t.AddPort("Translation", t.translationPort)

	t.ctrlPort = sim.NewLimitNumMsgPort(t, 1, name+".CtrlPort")
	t.AddPort("Control", t.ctrlPort)
}
