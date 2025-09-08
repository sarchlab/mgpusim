// Package shaderarray provides a builder for a shader array.
package shaderarray

import (
	"fmt"

	"github.com/sarchlab/akita/v4/mem/cache/writearound"
	"github.com/sarchlab/akita/v4/mem/cache/writethrough"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm/addresstranslator"
	"github.com/sarchlab/akita/v4/mem/vm/tlb"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"github.com/sarchlab/akita/v4/simulation"
	"github.com/sarchlab/mgpusim/v4/amd/timing/cu"
	"github.com/sarchlab/mgpusim/v4/amd/timing/rob"
)

// Builder builds a shader array.
type Builder struct {
	simulation *simulation.Simulation

	gpuID              uint64
	name               string
	numCUs             int
	freq               sim.Freq
	log2CacheLineSize  uint64
	log2PageSize       uint64
	l1AddressMapper    mem.AddressToPortMapper
	l1TLBAddressMapper mem.AddressToPortMapper

	sa        *sim.Domain
	cus       []*cu.ComputeUnit
	l1vROBs   []*rob.ReorderBuffer
	l1sROB    *rob.ReorderBuffer
	l1iROB    *rob.ReorderBuffer
	l1vATs    []*addresstranslator.Comp
	l1sAT     *addresstranslator.Comp
	l1iAT     *addresstranslator.Comp
	l1vCaches []*writearound.Comp
	l1sCache  *writethrough.Comp
	l1iCache  *writethrough.Comp
    l1vTLBs   []*tlb.Comp
    l1sTLB    *tlb.Comp
    l1iTLB    *tlb.Comp

    // Mapper pointers to allow left-to-right component build order
    // Vector path: ROB -> AT -(mem)-> L1V Cache, AT -(xlate)-> L1V TLB
    l1vMemMappers   []*mem.SinglePortMapper
    l1vTransMappers []*mem.SinglePortMapper

    // Scalar path: ROB -> AT -(mem)-> L1S Cache, AT -(xlate)-> L1S TLB
    l1sMemMapper   *mem.SinglePortMapper
    l1sTransMapper *mem.SinglePortMapper

    // Instruction path: ROB -> L1I Cache -(mem)-> AT -(xlate)-> L1I TLB
    l1iCacheMapper *mem.SinglePortMapper
    l1iTransMapper *mem.SinglePortMapper

    connectionCount int
}

// MakeBuilder creates a new builder.
func MakeBuilder() Builder {
	return Builder{
		numCUs:            4,
		freq:              1 * sim.GHz,
		log2CacheLineSize: 6,
		log2PageSize:      12,
	}
}

// WithSimulation sets the simulation to use.
func (b Builder) WithSimulation(sim *simulation.Simulation) Builder {
	b.simulation = sim
	return b
}

// WithGPUID sets the GPU ID to use.
func (b Builder) WithGPUID(gpuID uint64) Builder {
	b.gpuID = gpuID
	return b
}

// WithNumCUs sets the number of CUs to use.
func (b Builder) WithNumCUs(numCUs int) Builder {
	b.numCUs = numCUs
	return b
}

// WithFreq sets the frequency to use.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithLog2CacheLineSize sets the log2 cache line size to use.
func (b Builder) WithLog2CacheLineSize(log2CacheLineSize uint64) Builder {
	b.log2CacheLineSize = log2CacheLineSize
	return b
}

// WithLog2PageSize sets the log2 page size to use.
func (b Builder) WithLog2PageSize(log2PageSize uint64) Builder {
	b.log2PageSize = log2PageSize
	return b
}

// WithL1AddressMapper sets the L1 address mapper to use.
func (b Builder) WithL1AddressMapper(
	l1AddressMapper mem.AddressToPortMapper,
) Builder {
	b.l1AddressMapper = l1AddressMapper
	return b
}

// WithL1TLBAddressMapper sets the L1 TLB address mapper to use.
func (b Builder) WithL1TLBAddressMapper(
	l1TLBAddressMapper mem.AddressToPortMapper,
) Builder {
	b.l1TLBAddressMapper = l1TLBAddressMapper
	return b
}

// Build builds the shader array.
func (b Builder) Build(name string) *sim.Domain {
	b.name = name
	b.sa = sim.NewDomain(name)

	b.buildComponents()
	b.connectComponents()

	return b.sa
}

func (b *Builder) buildComponents() {
    b.buildCUs()

    // Vector path (build left -> root): ROB -> AT -> L1 Cache -> L1 TLB
    b.buildL1VReorderBuffers()
    b.buildL1VAddressTranslators()
    b.buildL1VCaches()
    b.buildL1VTLBs()

    // Scalar path (build left -> root): ROB -> AT -> L1 Cache -> L1 TLB
    b.buildL1SReorderBuffer()
    b.buildL1SAddressTranslator()
    b.buildL1SCache()
    b.buildL1STLB()

    // Instruction path (build left -> root): ROB -> L1 Cache -> AT -> L1 TLB
    b.buildL1IReorderBuffer()
    b.buildL1ICache()
    b.buildL1IAddressTranslator()
    b.buildL1ITLB()

	b.populateExternalPorts()
}

func (b *Builder) populateExternalPorts() {
	for i := range b.numCUs {
		cu := b.cus[i]

		b.sa.AddPort(fmt.Sprintf("CU[%d]", i), cu.GetPortByName("Top"))
		b.sa.AddPort(fmt.Sprintf("CUCtrl[%d]", i), cu.GetPortByName("Ctrl"))
		b.sa.AddPort(fmt.Sprintf("L1VROBCtrl[%d]", i), b.l1vROBs[i].
			GetPortByName("Control"))
		b.sa.AddPort(fmt.Sprintf("L1VAddrTransCtrl[%d]", i),
			b.l1vATs[i].GetPortByName("Control"))
		b.sa.AddPort(fmt.Sprintf("L1VTLBCtrl[%d]", i),
			b.l1vTLBs[i].GetPortByName("Control"))
		b.sa.AddPort(fmt.Sprintf("L1VCacheCtrl[%d]", i),
			b.l1vCaches[i].GetPortByName("Control"))
		b.sa.AddPort(fmt.Sprintf("L1VCacheBottom[%d]", i),
			b.l1vCaches[i].GetPortByName("Bottom"))
		b.sa.AddPort(fmt.Sprintf("L1VTLBBottom[%d]", i),
			b.l1vTLBs[i].GetPortByName("Bottom"))
	}

	b.sa.AddPort("L1SROBCtrl", b.l1sROB.GetPortByName("Control"))
	b.sa.AddPort("L1SAddrTransCtrl", b.l1sAT.GetPortByName("Control"))
	b.sa.AddPort("L1STLBCtrl", b.l1sTLB.GetPortByName("Control"))
	b.sa.AddPort("L1SCacheCtrl", b.l1sCache.GetPortByName("Control"))
	b.sa.AddPort("L1SCacheBottom", b.l1sCache.GetPortByName("Bottom"))
	b.sa.AddPort("L1STLBBottom", b.l1sTLB.GetPortByName("Bottom"))

	b.sa.AddPort("L1IROBCtrl", b.l1iROB.GetPortByName("Control"))
	b.sa.AddPort("L1IAddrTransCtrl", b.l1iAT.GetPortByName("Control"))
	b.sa.AddPort("L1ITLBCtrl", b.l1iTLB.GetPortByName("Control"))
	b.sa.AddPort("L1ICacheCtrl", b.l1iCache.GetPortByName("Control"))
	b.sa.AddPort("L1ICacheBottom", b.l1iAT.GetPortByName("Bottom"))
	b.sa.AddPort("L1ITLBBottom", b.l1iTLB.GetPortByName("Bottom"))
}

func (b *Builder) connectComponents() {
	b.connectVectorMem()
	b.connectScalarMem()
	b.connectInstMem()
}

func (b *Builder) connectVectorMem() {
    for i := range b.numCUs {
        cu := b.cus[i]
        rob := b.l1vROBs[i]
        at := b.l1vATs[i]
        l1v := b.l1vCaches[i]
        tlb := b.l1vTLBs[i]

        // Set mapper targets now that cache/TLB are built
        if b.l1vMemMappers != nil && i < len(b.l1vMemMappers) && b.l1vMemMappers[i] != nil {
            b.l1vMemMappers[i].Port = l1v.GetPortByName("Top").AsRemote()
        }
        if b.l1vTransMappers != nil && i < len(b.l1vTransMappers) && b.l1vTransMappers[i] != nil {
            b.l1vTransMappers[i].Port = tlb.GetPortByName("Top").AsRemote()
        }

		cu.VectorMemModules = &mem.SinglePortMapper{
			Port: rob.GetPortByName("Top").AsRemote(),
		}
		b.connectWithDirectConnection(cu.ToVectorMem,
			rob.GetPortByName("Top"), 8)

		atTopPort := at.GetPortByName("Top")
		rob.BottomUnit = atTopPort
		b.connectWithDirectConnection(
			rob.GetPortByName("Bottom"), atTopPort, 8)

        tlbTopPort := tlb.GetPortByName("Top")
        b.connectWithDirectConnection(
            at.GetPortByName("Translation"), tlbTopPort, 8)

        b.connectWithDirectConnection(l1v.GetPortByName("Top"),
            at.GetPortByName("Bottom"), 8)
    }
}

func (b *Builder) connectScalarMem() {
    rob := b.l1sROB
    at := b.l1sAT
    tlb := b.l1sTLB
    l1s := b.l1sCache

    // Set mapper targets now that cache/TLB are built
    if b.l1sMemMapper != nil {
        b.l1sMemMapper.Port = l1s.GetPortByName("Top").AsRemote()
    }
    if b.l1sTransMapper != nil {
        b.l1sTransMapper.Port = tlb.GetPortByName("Top").AsRemote()
    }

	atTopPort := at.GetPortByName("Top")
	rob.BottomUnit = atTopPort
	b.connectWithDirectConnection(rob.GetPortByName("Bottom"), atTopPort, 8)

    tlbTopPort := tlb.GetPortByName("Top")
    b.connectWithDirectConnection(
        at.GetPortByName("Translation"), tlbTopPort, 8)
    b.connectWithDirectConnection(
        l1s.GetPortByName("Top"), at.GetPortByName("Bottom"), 8)

	conn := directconnection.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		Build(b.name + ".ScalarMemConn")
	b.simulation.RegisterComponent(conn)

	conn.PlugIn(rob.GetPortByName("Top"))
	for i := range b.numCUs {
		cu := b.cus[i]
		cu.ScalarMem = rob.GetPortByName("Top")
		conn.PlugIn(cu.ToScalarMem)
	}
}

func (b *Builder) connectInstMem() {
    rob := b.l1iROB
    at := b.l1iAT
    tlb := b.l1iTLB
    l1i := b.l1iCache

    // Set mapper targets now that AT/TLB are built
    if b.l1iCacheMapper != nil {
        b.l1iCacheMapper.Port = at.GetPortByName("Top").AsRemote()
    }
    if b.l1iTransMapper != nil {
        b.l1iTransMapper.Port = tlb.GetPortByName("Top").AsRemote()
    }

	l1iTopPort := l1i.GetPortByName("Top")
	rob.BottomUnit = l1iTopPort
	b.connectWithDirectConnection(rob.GetPortByName("Bottom"), l1iTopPort, 8)

    atTopPort := at.GetPortByName("Top")
    // L1I cache receives responses from AT
    b.connectWithDirectConnection(l1i.GetPortByName("Bottom"), atTopPort, 8)

    tlbTopPort := tlb.GetPortByName("Top")
    b.connectWithDirectConnection(
        at.GetPortByName("Translation"), tlbTopPort, 8)

	robTopPort := rob.GetPortByName("Top")
	conn := directconnection.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		Build(b.name + ".InstMemConn")
	b.simulation.RegisterComponent(conn)

	conn.PlugIn(robTopPort)
	for i := range b.numCUs {
		cu := b.cus[i]
		cu.InstMem = rob.GetPortByName("Top")
		conn.PlugIn(cu.ToInstMem)
	}
}

func (b *Builder) connectWithDirectConnection(
	port1, port2 sim.Port,
	bufferSize int,
) {
	name := fmt.Sprintf("%s.Conn[%d]", b.name, b.connectionCount)
	b.connectionCount++

	conn := directconnection.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		Build(name)

	b.simulation.RegisterComponent(conn)

	conn.PlugIn(port1)
	conn.PlugIn(port2)
}

func (b *Builder) buildCUs() {
	cuBuilder := cu.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithLog2CachelineSize(b.log2CacheLineSize)

	for i := 0; i < b.numCUs; i++ {
		cuName := fmt.Sprintf("%s.CU[%d]", b.name, i)
		computeUnit := cuBuilder.Build(cuName)
		b.cus = append(b.cus, computeUnit)
		b.simulation.RegisterComponent(computeUnit)

		// if b.isaDebugging {
		// 	isaDebug, err := os.Create(
		// 		fmt.Sprintf("isa_%s.debug", cuName))
		// 	if err != nil {
		// 		log.Fatal(err.Error())
		// 	}
		// 	isaDebugger := cu.NewISADebugger(
		// 		log.New(isaDebug, "", 0), computeUnit)

		// 	tracing.CollectTrace(computeUnit, isaDebugger)
		// }
	}
}

func (b *Builder) buildL1VReorderBuffers() {
	builder := rob.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithBufferSize(128).
		WithNumReqPerCycle(4)

	for i := 0; i < b.numCUs; i++ {
		name := fmt.Sprintf("%s.L1VROB[%d]", b.name, i)
		rob := builder.Build(name)
		b.l1vROBs = append(b.l1vROBs, rob)
		b.simulation.RegisterComponent(rob)

		// if b.visTracer != nil {
		// 	tracing.CollectTrace(rob, b.visTracer)
		// }
	}
}

func (b *Builder) buildL1VAddressTranslators() {
    base := addresstranslator.MakeBuilder().
        WithEngine(b.simulation.GetEngine()).
        WithFreq(b.freq).
        WithDeviceID(b.gpuID).
        WithLog2PageSize(b.log2PageSize)

    b.l1vMemMappers = make([]*mem.SinglePortMapper, 0, b.numCUs)
    b.l1vTransMappers = make([]*mem.SinglePortMapper, 0, b.numCUs)

    for i := 0; i < b.numCUs; i++ {
        name := fmt.Sprintf("%s.L1VAddrTrans[%d]", b.name, i)
        memMapper := &mem.SinglePortMapper{}
        xlateMapper := &mem.SinglePortMapper{}
        curr := base.
            WithMemoryProviderMapper(memMapper).
            WithTranslationProviderMapper(xlateMapper)
        at := curr.Build(name)
        b.l1vATs = append(b.l1vATs, at)
        b.l1vMemMappers = append(b.l1vMemMappers, memMapper)
        b.l1vTransMappers = append(b.l1vTransMappers, xlateMapper)
        b.simulation.RegisterComponent(at)
    }
}

func (b *Builder) buildL1VTLBs() {
    builder := tlb.MakeBuilder().
        WithEngine(b.simulation.GetEngine()).
        WithFreq(b.freq).
        WithNumMSHREntry(4).
        WithNumSets(1).
        WithNumWays(64).
        WithNumReqPerCycle(4).
        WithTranslationProviderMapper(b.l1TLBAddressMapper)

	for i := 0; i < b.numCUs; i++ {
		name := fmt.Sprintf("%s.L1VTLB[%d]", b.name, i)
		tlb := builder.Build(name)
		b.l1vTLBs = append(b.l1vTLBs, tlb)
		b.simulation.RegisterComponent(tlb)
	}
}

func (b *Builder) buildL1VCaches() {
	builder := writearound.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithBankLatency(60).
		WithNumBanks(1).
		WithLog2BlockSize(b.log2CacheLineSize).
		WithWayAssociativity(4).
		WithNumMSHREntry(16).
		WithTotalByteSize(16 * mem.KB).
		WithAddressToPortMapper(b.l1AddressMapper)

	for i := 0; i < b.numCUs; i++ {
		name := fmt.Sprintf("%s.L1VCache[%d]", b.name, i)
		cache := builder.Build(name)
		b.l1vCaches = append(b.l1vCaches, cache)
		b.simulation.RegisterComponent(cache)

		// if b.memTracer != nil {
		// 	tracing.CollectTrace(cache, b.memTracer)
		// }
	}
}

func (b *Builder) buildL1SReorderBuffer() {
	builder := rob.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithBufferSize(128).
		WithNumReqPerCycle(4)

	name := fmt.Sprintf("%s.L1SROB", b.name)
	rob := builder.Build(name)
	b.l1sROB = rob
	b.simulation.RegisterComponent(rob)
}

func (b *Builder) buildL1SAddressTranslator() {
    // Prepare mappers and set ports later when cache/TLB are ready
    if b.l1sMemMapper == nil {
        b.l1sMemMapper = &mem.SinglePortMapper{}
    }
    if b.l1sTransMapper == nil {
        b.l1sTransMapper = &mem.SinglePortMapper{}
    }
    builder := addresstranslator.MakeBuilder().
        WithEngine(b.simulation.GetEngine()).
        WithFreq(b.freq).
        WithDeviceID(b.gpuID).
        WithLog2PageSize(b.log2PageSize).
        WithMemoryProviderMapper(b.l1sMemMapper).
        WithTranslationProviderMapper(b.l1sTransMapper)

    name := fmt.Sprintf("%s.L1SAddrTrans", b.name)
    at := builder.Build(name)
    b.l1sAT = at
    b.simulation.RegisterComponent(at)
}

func (b *Builder) buildL1STLB() {
    builder := tlb.MakeBuilder().
        WithEngine(b.simulation.GetEngine()).
        WithFreq(b.freq).
        WithNumMSHREntry(4).
        WithNumSets(1).
        WithNumWays(64).
        WithNumReqPerCycle(4).
        WithTranslationProviderMapper(b.l1TLBAddressMapper)

	name := fmt.Sprintf("%s.L1STLB", b.name)
	tlb := builder.Build(name)
	b.l1sTLB = tlb
	b.simulation.RegisterComponent(tlb)
}

func (b *Builder) buildL1SCache() {
	builder := writethrough.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithBankLatency(1).
		WithNumBanks(1).
		WithLog2BlockSize(b.log2CacheLineSize).
		WithWayAssociativity(4).
		WithNumMSHREntry(16).
		WithTotalByteSize(16 * mem.KB).
		WithAddressToPortMapper(b.l1AddressMapper)

	name := fmt.Sprintf("%s.L1SCache", b.name)
	cache := builder.Build(name)
	b.l1sCache = cache
	b.simulation.RegisterComponent(cache)

	// if b.memTracer != nil {
	// 	tracing.CollectTrace(cache, b.memTracer)
	// }
}

func (b *Builder) buildL1IReorderBuffer() {
	builder := rob.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithBufferSize(128).
		WithNumReqPerCycle(4)

	name := fmt.Sprintf("%s.L1IROB", b.name)
	rob := builder.Build(name)
	b.l1iROB = rob
	b.simulation.RegisterComponent(rob)
}

func (b *Builder) buildL1IAddressTranslator() {
    if b.l1iTransMapper == nil {
        b.l1iTransMapper = &mem.SinglePortMapper{}
    }
    builder := addresstranslator.MakeBuilder().
        WithEngine(b.simulation.GetEngine()).
        WithFreq(b.freq).
        WithDeviceID(b.gpuID).
        WithLog2PageSize(b.log2PageSize).
        WithMemoryProviderMapper(b.l1AddressMapper).
        WithTranslationProviderMapper(b.l1iTransMapper)

	name := fmt.Sprintf("%s.L1IAddrTrans", b.name)
	at := builder.Build(name)
	b.l1iAT = at
	b.simulation.RegisterComponent(at)
}

func (b *Builder) buildL1ITLB() {
    builder := tlb.MakeBuilder().
        WithEngine(b.simulation.GetEngine()).
        WithFreq(b.freq).
        WithNumMSHREntry(4).
        WithNumSets(1).
        WithNumWays(64).
        WithNumReqPerCycle(4).
        WithTranslationProviderMapper(b.l1TLBAddressMapper)

	name := fmt.Sprintf("%s.L1ITLB", b.name)
	tlb := builder.Build(name)
	b.l1iTLB = tlb
	b.simulation.RegisterComponent(tlb)
}

func (b *Builder) buildL1ICache() {
    if b.l1iCacheMapper == nil {
        b.l1iCacheMapper = &mem.SinglePortMapper{}
    }
    builder := writethrough.MakeBuilder().
        WithEngine(b.simulation.GetEngine()).
        WithFreq(b.freq).
        WithBankLatency(1).
        WithNumBanks(1).
        WithLog2BlockSize(b.log2CacheLineSize).
        WithWayAssociativity(4).
        WithNumMSHREntry(16).
        WithTotalByteSize(32 * mem.KB).
        WithNumReqsPerCycle(4).
        WithAddressToPortMapper(b.l1iCacheMapper)

	name := fmt.Sprintf("%s.L1ICache", b.name)
	cache := builder.Build(name)
	b.l1iCache = cache
	b.simulation.RegisterComponent(cache)
	// if b.memTracer != nil {
	// 	tracing.CollectTrace(cache, b.memTracer)
	// }
}
