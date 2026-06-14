package writethroughcache

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"

	"github.com/sarchlab/akita/v5/messaging"
)

// defaultSpec provides default configuration for the writethroughcache.
// The default write policy type is "write-around".
var defaultSpec = Spec{
	Freq:                  1 * timing.GHz,
	NumReqPerCycle:        4,
	Log2BlockSize:         6,
	BankLatency:           20,
	WayAssociativity:      4,
	MaxNumConcurrentTrans: 16,
	NumBanks:              1,
	NumMSHREntry:          4,
	TotalByteSize:         4 * mem.KB,
	DirLatency:            2,
	InterleavingSize:      4096,
	WritePolicyType:       "write-around",
}

// DefaultSpec returns a copy of the default configuration. Callers typically
// obtain it, tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// A Builder can build a writethroughcache cache. Configuration is supplied as a
// whole through WithSpec; wiring is supplied through WithRegistrar and
// WithResources. The component declares its "Top", "Bottom", and "Control"
// ports; the port instances are supplied externally after Build with AssignPort
// (the caller chooses the buffer sizes).
type Builder struct {
	spec      Spec
	registrar modeling.Registrar
	resources Resources
}

// MakeBuilder creates a builder with default parameter setting.
func MakeBuilder() Builder {
	return Builder{spec: defaultSpec}
}

// WithRegistrar wires the builder to a registrar (a *simulation.Simulation in
// assembly, or modeling.NewStandaloneRegistrar(engine) in isolated tests). The
// registrar provides the engine and registers the built component.
func (b Builder) WithRegistrar(reg modeling.Registrar) Builder {
	b.registrar = reg
	return b
}

// WithSpec sets the entire configuration. Start from DefaultSpec() and tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

// WithResources injects the component's shared resources and external wiring
// (storage, the address-to-port mapper, and the remote ports the cache sends
// requests to). If not set, the component builds its own storage.
func (b Builder) WithResources(r Resources) Builder {
	b.resources = r
	return b
}

// Build returns a new cache component. It declares the component's "Top",
// "Bottom", and "Control" ports; assign the port instances after Build with
// AssignPort.
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("writethroughcache: WithRegistrar is required")
	}

	spec := b.spec
	if spec.WritePolicyType == "" {
		spec.WritePolicyType = "write-around"
	}

	b.resolveAddressMapper(&spec)

	blockSize := 1 << spec.Log2BlockSize
	spec.NumSets = int(spec.TotalByteSize /
		uint64(spec.WayAssociativity*blockSize))

	initialState := b.buildInitialState(name, spec, spec.NumSets, blockSize)

	storage := b.resolveStorage(name, spec)

	comp := modeling.NewBuilder[Spec, State, Resources]().
		WithEngine(b.registrar.GetEngine()).
		WithFreq(spec.Freq).
		WithSpec(spec).
		WithResources(Resources{Storage: storage}).
		Build(name)

	comp.State = initialState

	pmw := b.buildPipelineMW(comp)
	b.buildStages(pmw, spec)

	ucmw := &ctrlMiddleware{pipeline: pmw}

	// Control runs before the data pipeline so a Pause/Drain/Reset takes
	// effect this tick before any Top/Bottom traffic advances.
	comp.AddMiddleware(ucmw) // index 0: control verbs
	comp.AddMiddleware(pmw)  // index 1: data pipeline

	comp.DeclarePort("Top", memprotocol.Responder)
	comp.DeclarePort("Bottom", memprotocol.Requester)
	comp.DeclarePort("Control", memcontrolprotocol.Responder)

	b.registrar.RegisterComponent(comp)

	return comp
}

// resolveStorage returns the injected storage, or builds a default one sized by
// Spec.TotalByteSize.
func (b Builder) resolveStorage(name string, spec Spec) *mem.Storage {
	if b.resources.Storage != nil {
		return b.resources.Storage
	}

	return mem.MakeStorageBuilder().
		WithCapacity(spec.TotalByteSize).
		WithSimulation(b.registrar).
		Build(name + ".Storage")
}

// resolveAddressMapper derives the address-mapper configuration stored in Spec
// (AddressMapperType, RemotePortNames, InterleavingSize) from the wiring placed
// in Resources. When an explicit mapper is injected via Resources.AddressMapper
// it is decoded into the type string and remote ports; otherwise the remote
// ports listed in Resources are combined with the Spec type string.
func (b Builder) resolveAddressMapper(spec *Spec) {
	if b.resources.AddressMapper != nil {
		switch m := b.resources.AddressMapper.(type) {
		case *mem.SinglePortMapper:
			spec.AddressMapperType = "single"
			spec.RemotePortNames = []string{string(m.Port)}
		case *mem.InterleavedAddressPortMapper:
			spec.AddressMapperType = "interleaved"
			spec.RemotePortNames = remotePortNames(m.LowModules)
			spec.InterleavingSize = m.InterleavingSize
		default:
			panic(fmt.Sprintf(
				"unsupported address mapper type: %T", b.resources.AddressMapper))
		}

		return
	}

	if spec.AddressMapperType != "" {
		spec.RemotePortNames = remotePortNames(b.resources.RemotePorts)
	}
}

func remotePortNames(ports []messaging.RemotePort) []string {
	names := make([]string, len(ports))
	for i, rp := range ports {
		names[i] = string(rp)
	}

	return names
}

func (b *Builder) buildInitialState(
	name string,
	spec Spec,
	numSets, blockSize int,
) State {
	bankBufs := make([]queueing.Buffer[int], spec.NumBanks)
	for i := 0; i < spec.NumBanks; i++ {
		bankBufs[i] = queueing.NewBuffer[int](
			fmt.Sprintf("%s.Bank%d.Buffer", name, i),
			spec.NumReqPerCycle,
		)
	}

	bankPipelines := make([]queueing.Pipeline[int], spec.NumBanks)
	for i := 0; i < spec.NumBanks; i++ {
		bankPipelines[i] = queueing.NewPipeline[int](
			spec.NumReqPerCycle,
			spec.BankLatency,
		)
	}

	bankPostBufs := make([]queueing.Buffer[int], spec.NumBanks)
	for i := 0; i < spec.NumBanks; i++ {
		bankPostBufs[i] = queueing.NewBuffer[int](
			fmt.Sprintf("%s.Bank[%d].PostPipelineBuffer", name, i),
			spec.NumReqPerCycle,
		)
	}

	initialState := State{
		DirBuf: queueing.NewBuffer[int](
			name+".DirectoryBuffer",
			spec.NumReqPerCycle,
		),
		BankBufs: bankBufs,
		DirPipeline: queueing.NewPipeline[int](
			spec.NumReqPerCycle,
			spec.DirLatency,
		),
		DirPostBuf: queueing.NewBuffer[int](
			name+".DirectoryStage.PostPipelineBuffer",
			spec.NumReqPerCycle,
		),
		BankPipelines: bankPipelines,
		BankPostBufs:  bankPostBufs,
	}

	cache.DirectoryReset(
		&initialState.DirectoryState, numSets, spec.WayAssociativity, blockSize)

	return initialState
}

func (b *Builder) buildPipelineMW(
	comp *modeling.Component[Spec, State, Resources],
) *pipelineMW {
	m := &pipelineMW{
		comp: comp,
	}

	m.storage = comp.Resources().Storage

	return m
}

func (b *Builder) buildStages(m *pipelineMW, spec Spec) {
	m.intakeStage = &intake{cache: m}
	m.directoryStage = &directory{
		cache: m,
	}
	b.buildBankStages(m, spec)
	m.parseBottomStage = &bottomParser{cache: m}
	m.respondStage = &respondStage{cache: m}
}

func (b *Builder) buildBankStages(m *pipelineMW, spec Spec) {
	for i := 0; i < spec.NumBanks; i++ {
		bs := &bankStage{
			cache:          m,
			bankID:         i,
			numReqPerCycle: spec.NumReqPerCycle,
		}
		m.bankStages = append(m.bankStages, bs)
	}
}
