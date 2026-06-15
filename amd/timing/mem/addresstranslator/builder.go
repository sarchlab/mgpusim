package addresstranslator

import (
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

// A Builder can create address translators
type Builder struct {
	engine   sim.Engine
	freq     sim.Freq
	ctrlPort sim.Port

	numReqPerCycle int
	log2PageSize   uint64
	deviceID       uint64

	memPortMapper             mem.AddressToPortMapper
	memPortMapperType         string
	memRemotePorts            []sim.RemotePort
	translationPortMapper     mem.AddressToPortMapper
	translationPortMapperType string
	translationRemotePorts    []sim.RemotePort
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

// WithAddressToPortMapper sets the low modules finder that can tell the address
// translators where to send the memory access request to.
//
// Deprecated: Use `WithMemoryProviderMapper` instead.
func (b Builder) WithAddressToPortMapper(f mem.AddressToPortMapper) Builder {
	b.memPortMapper = f
	return b
}

// WithMemoryProviderMapper sets the low modules finder that can tell the
// address translators where to send the memory access request to.
func (b Builder) WithMemoryProviderMapper(f mem.AddressToPortMapper) Builder {
	b.memPortMapper = f
	return b
}

// WithMemoryProviderType sets the type of the memory provider mapper. The
// mapper can find the remote port that can provide the memory service according
// to the virtual address. The type can be "single" or "interleaved".
func (b Builder) WithMemoryProviderType(t string) Builder {
	b.memPortMapperType = t
	return b
}

// WithMemoryProviders registers the remote ports that handle memory access
// requests.
//
// Use together with `WithMemoryProviderType` to control request distribution:
//   - "single": exactly one port must be provided.
//   - "interleaved": the number of ports must be a power of two; requests are
//     interleaved at page granularity (4 KiB by default).
func (b Builder) WithMemoryProviders(ports ...sim.RemotePort) Builder {
	b.memRemotePorts = ports
	return b
}

// WithTranslationProviderMapper sets the mapper that can find the remote port
// that can provide the translation service according to the virtual address.
func (b Builder) WithTranslationProviderMapper(
	table mem.AddressToPortMapper,
) Builder {
	b.translationPortMapper = table
	return b
}

// WithTranslationProvider sets the port that can provide the translation
// service. The port must be a port on a TLB or an MMU.
//
// Deprecated: Use `WithTranslationProviderMapper`, or use
// `WithTranslatorProviderMapperType` and `WithTranslationProviders` in
// combination instead.
func (b Builder) WithTranslationProvider(p sim.RemotePort) Builder {
	b.translationPortMapper = &mem.SinglePortMapper{
		Port: p,
	}

	return b
}

// WithTranslationProviderMapperType sets the type of the translation provider
// mapper. The mapper can find the remote port that can provide the translation
// service according to the virtual address. The type can be "single" or
// "interleaved".
func (b Builder) WithTranslationProviderMapperType(t string) Builder {
	b.translationPortMapperType = t
	return b
}

// WithTranslationProviders registers the remote ports that handle address
// translation requests.
//
// Use together with `WithTranslationProviderMapperType` to control request
// distribution:
//   - "single": exactly one port must be provided.
//   - "interleaved": the number of ports must be a power of two; requests are
//     interleaved at page granularity (4 KiB by default).
func (b Builder) WithTranslationProviders(ports ...sim.RemotePort) Builder {
	b.translationRemotePorts = ports
	return b
}

// Build returns a new AddressTranslator
func (b Builder) Build(name string) *Comp {
	t := &Comp{}
	t.TickingComponent = sim.NewTickingComponent(
		name, b.engine, b.freq, t)

	b.createPorts(name, t)
	b.setupMemoryPortMapper(t)
	b.setupTranslationPortMapper(t)

	t.numReqPerCycle = b.numReqPerCycle
	t.log2PageSize = b.log2PageSize
	t.deviceID = b.deviceID

	middleware := &middleware{Comp: t}
	t.AddMiddleware(middleware)

	return t
}

// setupMemoryPortMapper configures the memory port mapper for the address translator
func (b Builder) setupMemoryPortMapper(t *Comp) {
	if b.memPortMapper != nil {
		t.memoryPortMapper = b.memPortMapper
		return
	}

	switch b.memPortMapperType {
	case "single":
		if len(b.memRemotePorts) != 1 {
			panic("single address mapper requires exactly 1 port")
		}
		t.memoryPortMapper = &mem.SinglePortMapper{
			Port: b.memRemotePorts[0],
		}
	case "interleaved":
		if len(b.memRemotePorts) == 0 {
			panic("interleaved address mapper requires at least 1 port")
		}
		mapper := mem.NewInterleavedAddressPortMapper(1 << b.log2PageSize)
		mapper.LowModules = append(mapper.LowModules, b.memRemotePorts...)
		t.memoryPortMapper = mapper
	default:
		panic("invalid address mapper type: " + b.memPortMapperType)
	}
}

// setupTranslationPortMapper configures the translation port mapper for the address translator
func (b Builder) setupTranslationPortMapper(t *Comp) {
	if b.translationPortMapper != nil {
		t.translationPortMapper = b.translationPortMapper
		return
	}

	switch b.translationPortMapperType {
	case "single":
		if len(b.translationRemotePorts) != 1 {
			panic("single translation mapper requires exactly 1 port")
		}
		t.translationPortMapper = &mem.SinglePortMapper{
			Port: b.translationRemotePorts[0],
		}
	case "interleaved":
		if len(b.translationRemotePorts) == 0 {
			panic("interleaved translation mapper requires at least 1 port")
		}
		mapper := mem.NewInterleavedAddressPortMapper(1 << b.log2PageSize)
		mapper.LowModules = append(mapper.LowModules,
			b.translationRemotePorts...)
		t.translationPortMapper = mapper
	default:
		panic("invalid translation mapper type: " + b.translationPortMapperType)
	}
}

func (b Builder) createPorts(name string, t *Comp) {
	t.topPort = sim.NewPort(t, b.numReqPerCycle, b.numReqPerCycle,
		name+".TopPort")
	t.AddPort("Top", t.topPort)

	t.bottomPort = sim.NewPort(t, b.numReqPerCycle, b.numReqPerCycle,
		name+".BottomPort")
	t.AddPort("Bottom", t.bottomPort)

	t.translationPort = sim.NewPort(t, b.numReqPerCycle, b.numReqPerCycle,
		name+".TranslationPort")
	t.AddPort("Translation", t.translationPort)

	t.ctrlPort = sim.NewPort(t, 1, 1, name+".CtrlPort")
	t.AddPort("Control", t.ctrlPort)
}
