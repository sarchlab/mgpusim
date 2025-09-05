package sm

import (
	"fmt"

	"github.com/sarchlab/akita/v4/mem/cache/writearound"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"github.com/sarchlab/akita/v4/simulation"

	"github.com/sarchlab/mgpusim/v4/nvidia/smsp"
	"github.com/tebeka/atexit"
)

type SMBuilder struct {
	simulation *simulation.Simulation
	name       string

	engine sim.Engine
	freq   sim.Freq

	smspsCount        uint64
	log2CacheLineSize uint64

	// cache updates
	l1vCaches       []*writearound.Comp
	l1AddressMapper mem.AddressToPortMapper

	// sm    *SMControllers
	smsps []*smsp.SMSPController

	connectionCount int
}

func MakeBuilder() SMBuilder {
	return SMBuilder{
		freq:              1 * sim.GHz,
		log2CacheLineSize: 7,
	}
}

func (b SMBuilder) WithEngine(engine sim.Engine) SMBuilder {
	b.engine = engine
	return b
}

func (b SMBuilder) WithFreq(freq sim.Freq) SMBuilder {
	b.freq = freq
	return b
}

func (b SMBuilder) WithSimulation(sim *simulation.Simulation) SMBuilder {
	b.simulation = sim
	return b
}

func (b SMBuilder) WithSMSPsCount(count uint64) SMBuilder {
	b.smspsCount = count
	return b
}

func (b SMBuilder) WithL1AddressMapper(
	l1AddressMapper mem.AddressToPortMapper,
) SMBuilder {
	b.l1AddressMapper = l1AddressMapper
	return b
}

func (b SMBuilder) WithLog2CacheLineSize(size uint64) SMBuilder {
	b.log2CacheLineSize = size
	return b
}

func (b SMBuilder) Build(name string) *SMController {
	s := &SMController{
		ID:       sim.GetIDGenerator().Generate(),
		SMSPs:    make(map[string]*smsp.SMSPController),
		SMSPsIDs: []string{},
	}
	b.name = name
	b.connectionCount = 0

	s.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, s)
	b.buildL1VCaches()
	b.buildPortsForSM(s, name)
	b.buildSMSPs(name)
	b.connectSMwithSMSPs(s, b.smsps)

	// b.sm = s

	// s.PendingWriteReq = make(map[string]*message.SMSPToSMMemWriteMsg)
	// s.PendingReadReq = make(map[string]*message.SMSPToSMMemReadMsg)

	b.connectVectorMem()

	// b.populateExternalPorts(s)

	atexit.Register(s.LogStatus)

	return s
}

// func (b *SMBuilder) populateExternalPorts(sm *SMController) {
// 	// sm.AddPort("L1CacheBottom", b.l1Cache.GetPortByName("Bottom"))
// 	for i := range b.smspsCount {
// 		smsp := b.smsps[i]
// 		b.sm.AddPort(fmt.Sprintf("L1VCacheBottom[%d]", i),
// 			b.l1vCaches[i].GetPortByName("Bottom"))
// 	}
// }

func (b *SMBuilder) buildPortsForSM(sm *SMController, name string) {
	sm.toGPU = sim.NewPort(sm, 4096, 4096, fmt.Sprintf("%s.ToGPU", name))
	sm.toSMSPs = sim.NewPort(sm, 4096, 4096, fmt.Sprintf("%s.ToSMSPs", name))
	sm.AddPort(fmt.Sprintf("%s.ToGPU", name), sm.toGPU)
	sm.AddPort(fmt.Sprintf("%s.ToSMSPs", name), sm.toSMSPs)

	for i := range b.smspsCount {
		// smsp := b.smsps[i]
		sm.AddPort(fmt.Sprintf("L1VCacheBottom[%d]", i),
			b.l1vCaches[i].GetPortByName("Bottom"))
	}

	// cache updates
	// sm.toGPUMem = sim.NewPort(sm,4096, 4096, fmt.Sprintf("%s.ToGPUMem", name))
	// sm.toSMSPMem = sim.NewPort(sm,4096, 4096, fmt.Sprintf("%s.ToSMSPMem", name))
	// sm.AddPort(fmt.Sprintf("%s.ToGPUMem", name), sm.toGPUMem)
	// sm.AddPort(fmt.Sprintf("%s.ToSMSPMem", name), sm.toSMSPMem)
}

func (b *SMBuilder) buildSMSPs(smName string) []*smsp.SMSPController {

	b.smsps = []*smsp.SMSPController{}
	for i := uint64(0); i < b.smspsCount; i++ {
		smspBuilder := new(smsp.SMSPBuilder).
			WithEngine(b.engine).
			WithFreq(b.freq).
			WithSimulation(b.simulation)

		smsp := smspBuilder.Build(fmt.Sprintf("%s.SMSP(%d)", smName, i))
		b.simulation.RegisterComponent(smsp)
		b.smsps = append(b.smsps, smsp)
		smsp.SetVectorMemRemote(b.l1vCaches[i].GetPortByName("Top"))
	}

	return b.smsps
}

func (b *SMBuilder) connectSMwithSMSPs(sm *SMController, smsps []*smsp.SMSPController) {
	conn := directconnection.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(1 * sim.GHz).
		Build(fmt.Sprintf("%s.SMToSMSPs", b.name))
		// Build("SMToSMSPs")
		// Build(fmt.Sprintf("%s.SMSPController(%d)", smName, i))

	conn.PlugIn(sm.toSMSPs)
	b.simulation.RegisterComponent(conn)
	// conn.PlugIn(sm.toSMSPMem)

	for i := range smsps {
		smsp := smsps[i]

		sm.freeSMSPs = append(sm.freeSMSPs, smsp)
		sm.SMSPs[smsp.ID] = smsp
		sm.SMSPsIDs = append(sm.SMSPsIDs, smsp.ID)

		smsp.SetSMRemotePort(sm.toSMSPs)
		// smsp.SetGPUControllerMemRemote(sm.toGPUControllerCaches)
		conn.PlugIn(smsp.GetPortByName(fmt.Sprintf("%s.ToSM", smsp.Name())))

		// smsp.SetSMMemRemotePort(sm.toSMSPMem)
		// conn.PlugIn(smsp.GetPortByName(fmt.Sprintf("%s.ToSMMem", smsp.Name())))
	}
}

func (b *SMBuilder) buildL1VCaches() {
	for i := 0; i < int(b.smspsCount); i++ {
		builder := writearound.MakeBuilder().
			WithEngine(b.engine).
			WithFreq(b.freq).
			WithBankLatency(60).
			WithNumBanks(1).
			WithLog2BlockSize(b.log2CacheLineSize).
			WithWayAssociativity(4).
			WithNumMSHREntry(16).
			WithTotalByteSize(16 * mem.KB).
			WithAddressToPortMapper(b.l1AddressMapper)

		name := fmt.Sprintf("%s.L1VCache[%d]", b.name, i)
		// fmt.Printf("b.name: %s, cache name: %s\n", b.name, name)
		cache := builder.Build(name)
		b.l1vCaches = append(b.l1vCaches, cache)
		b.simulation.RegisterComponent(cache)

		// if b.memTracer != nil {
		// 	tracing.CollectTrace(cache, b.memTracer)
		// }
	}
	// name := fmt.Sprintf("%s.L1Cache", b.name)
	// cache := builder.Build(name)
	// b.simulation.RegisterComponent(cache)
	// b.l1Cache = cache
}

func (b *SMBuilder) connectVectorMem() {
	for i := range b.smspsCount {
		smsp := b.smsps[i]
		// rob := b.l1vROBs[i]
		// at := b.l1vATs[i]
		l1v := b.l1vCaches[i]
		// tlb := b.l1vTLBs[i]

		// smsp.VectorMemModules = &mem.SinglePortMapper{
		// 	Port: rob.GetPortByName("Top").AsRemote(),
		// }
		// b.connectWithDirectConnection(smsp.ToVectorMem,
		// 	rob.GetPortByName("Top"), 8)

		// atTopPort := at.GetPortByName("Top")
		// rob.BottomUnit = atTopPort
		// b.connectWithDirectConnection(
		// 	rob.GetPortByName("Bottom"), atTopPort, 8)

		// tlbTopPort := tlb.GetPortByName("Top")
		// at.SetTranslationProvider(tlbTopPort.AsRemote())
		// b.connectWithDirectConnection(
		// 	at.GetPortByName("Translation"), tlbTopPort, 8)

		// at.SetAddressToPortMapper(&mem.SinglePortMapper{
		// 	Port: l1v.GetPortByName("Top").AsRemote(),
		// })
		// b.connectWithDirectConnection(l1v.GetPortByName("Top"),
		// 	at.GetPortByName("Bottom"), 8)

		b.connectWithDirectConnection(smsp.ToVectorMem, l1v.GetPortByName("Top"))
	}
}

func (b *SMBuilder) connectWithDirectConnection(
	port1, port2 sim.Port,
	// bufferSize int,
) {
	name := fmt.Sprintf("%s.Conn[%d]", b.name, b.connectionCount)
	// fmt.Printf("Connecting %s with %s through %s\n", port1.Name(), port2.Name(), name)
	b.connectionCount++

	conn := directconnection.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		Build(name)

	b.simulation.RegisterComponent(conn)

	conn.PlugIn(port1)
	conn.PlugIn(port2)
}

// func (b *SMBuilder) buildL1Caches(sm *SMController) {
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
