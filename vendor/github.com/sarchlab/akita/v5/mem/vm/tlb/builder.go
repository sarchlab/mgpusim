package tlb

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
)

// DefaultSpec provides the default configuration for TLB components.
var DefaultSpec = Spec{
	Freq:           1 * sim.GHz,
	NumReqPerCycle: 4,
	NumSets:        1,
	NumWays:        32,
	PageSize:       4096,
	MSHRSize:       4,
	Latency:        4,
}

// A Builder can build TLBs
type Builder struct {
	engine            sim.EventScheduler
	spec              Spec
	log2PageSize      uint64
	addressMapperType string
	remotePorts       []sim.RemotePort
	topPort           sim.Port
	bottomPort        sim.Port
	controlPort       sim.Port

	// Legacy support for WithTranslationProviderMapper
	legacyMapper mem.AddressToPortMapper
}

// MakeBuilder returns a Builder
func MakeBuilder() Builder {
	return Builder{
		spec: DefaultSpec,
	}
}

// WithEngine sets the engine that the TLBs to use
func (b Builder) WithEngine(engine sim.EventScheduler) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the freq the TLBs use
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.spec.Freq = freq
	return b
}

// WithNumSets sets the number of sets in a TLB. Use 1 for fully associated
// TLBs.
func (b Builder) WithNumSets(n int) Builder {
	b.spec.NumSets = n
	return b
}

// WithNumWays sets the number of ways in a TLB. Set this field to the number
// of TLB entries for all the functions.
func (b Builder) WithNumWays(n int) Builder {
	b.spec.NumWays = n
	return b
}

// WithLog2PageSize sets the page size as a power of 2
func (b Builder) WithLog2PageSize(n uint64) Builder {
	b.log2PageSize = n
	b.spec.PageSize = 1 << n
	return b
}

// WithNumReqPerCycle sets the number of requests per cycle can be processed by
// a TLB
func (b Builder) WithNumReqPerCycle(n int) Builder {
	b.spec.NumReqPerCycle = n
	return b
}

// WithNumMSHREntry sets the number of mshr entry
func (b Builder) WithNumMSHREntry(num int) Builder {
	b.spec.MSHRSize = num
	return b
}

// WithLatency sets the latency of the TLB lookup. The latency is counted in
// both hit and miss cases.
func (b Builder) WithLatency(cycles int) Builder {
	b.spec.Latency = cycles
	return b
}

// WithTranslationProviderMapper sets the mapper that can find the remote port
// that can provide the translation service according to the virtual address.
func (b Builder) WithTranslationProviderMapper(
	mapper mem.AddressToPortMapper,
) Builder {
	b.legacyMapper = mapper
	return b
}

// WithTranslationProviderMapperType sets the type of the translation provider
// mapper. The mapper can find the remote port that can provide the translation
// service according to the virtual address. The type can be "single" or
// "interleaved".
func (b Builder) WithTranslationProviderMapperType(t string) Builder {
	b.addressMapperType = t
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
	b.remotePorts = ports
	return b
}

// WithTopPort sets the top port for the TLB
func (b Builder) WithTopPort(port sim.Port) Builder {
	b.topPort = port
	return b
}

// WithBottomPort sets the bottom port for the TLB
func (b Builder) WithBottomPort(port sim.Port) Builder {
	b.bottomPort = port
	return b
}

// WithControlPort sets the control port for the TLB
func (b Builder) WithControlPort(port sim.Port) Builder {
	b.controlPort = port
	return b
}

// Build creates a new TLB
func (b Builder) Build(name string) *modeling.Component[Spec, State] {
	addrMapperKind, addrMapperPorts, addrMapperInterleavingSize := b.resolveAddressMapper()

	spec := b.spec
	spec.PipelineWidth = b.spec.NumReqPerCycle
	spec.AddrMapperKind = addrMapperKind
	spec.AddrMapperPorts = addrMapperPorts
	spec.AddrMapperInterleavingSize = addrMapperInterleavingSize

	initialState := State{
		TLBState: tlbStateEnable,
		Sets:     initSets(b.spec.NumSets, b.spec.NumWays),
		Pipeline: queueing.Pipeline[pipelineTLBReqState]{
			Width:     spec.PipelineWidth,
			NumStages: b.spec.Latency,
		},
	}

	modelComp := modeling.NewBuilder[Spec, State]().
		WithEngine(b.engine).
		WithFreq(b.spec.Freq).
		WithSpec(spec).
		Build(name)
	modelComp.SetState(initialState)

	b.topPort.SetComponent(modelComp)
	modelComp.AddPort("Top", b.topPort)

	b.bottomPort.SetComponent(modelComp)
	modelComp.AddPort("Bottom", b.bottomPort)

	b.controlPort.SetComponent(modelComp)
	modelComp.AddPort("Control", b.controlPort)

	ctrlMW := &ctrlMiddleware{comp: modelComp}
	modelComp.AddMiddleware(ctrlMW)

	tlbMW := &tlbMiddleware{comp: modelComp}
	modelComp.AddMiddleware(tlbMW)

	return modelComp
}

func (b Builder) resolveAddressMapper() (string, []sim.RemotePort, uint64) {
	if b.legacyMapper != nil {
		// Convert legacy mapper to spec fields
		switch m := b.legacyMapper.(type) {
		case *mem.SinglePortMapper:
			return "single", []sim.RemotePort{m.Port}, 0
		case *mem.InterleavedAddressPortMapper:
			return "interleaved", m.LowModules, m.InterleavingSize
		default:
			// For any unknown mapper type, try to use it as single port
			// Fall through to the addressMapperType logic
		}
	}

	interleavingSize := b.spec.PageSize
	if interleavingSize == 0 {
		interleavingSize = 4096
	}

	if b.addressMapperType != "" {
		switch b.addressMapperType {
		case "single":
			if len(b.remotePorts) != 1 {
				panic("single address mapper requires exactly 1 port")
			}
			return "single", b.remotePorts, 0
		case "interleaved":
			if len(b.remotePorts) == 0 {
				panic("interleaved address mapper requires at least 1 port")
			}
			return "interleaved", b.remotePorts, interleavingSize
		default:
			panic("invalid address mapper type: " + b.addressMapperType)
		}
	}

	panic("no address mapper configured")
}
