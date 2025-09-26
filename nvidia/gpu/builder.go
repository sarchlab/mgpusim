package gpu

import (
	"fmt"

	"github.com/sarchlab/akita/v4/mem/cache/writeback"
	"github.com/sarchlab/akita/v4/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection" //
	"github.com/sarchlab/akita/v4/simulation"
	"github.com/sarchlab/mgpusim/v4/nvidia/sm"
	"github.com/tebeka/atexit"
)

type GPUBuilder struct {
	simulation *simulation.Simulation
	gpuName    string
	gpu        *GPUController

	engine sim.Engine
	freq   sim.Freq

	smsCount        uint64
	smspsCountPerSM uint64

	DramSize                       uint64
	log2CacheLineSize              uint64
	numMemoryBank                  int
	log2MemoryBankInterleavingSize uint64

	// cache updates
	// drams       []sim.Component // *idealmemcontroller.Comp
	DRAMs           []*idealmemcontroller.Comp
	l2Caches        []*writeback.Comp
	l2CacheSize     uint64
	l1AddressMapper *mem.InterleavedAddressPortMapper
	memAddrOffset   uint64

	// l1ToL2Connection        *directconnection.Comp
	l2ToDramConnection *directconnection.Comp
	// rdmaEngine              *rdma.Comp
	// dmaEngine               *cp.DMAEngine
	// pageMigrationController *pagemigrationcontroller.PageMigrationController
	GPU2SMThreadBlockAllocationLatency uint64
	SM2SMSPWarpIssueLatency            uint64
	SMReceiveGPULatency                uint64
	GPUReceiveSMLatency                uint64
}

func (b *GPUBuilder) WithEngine(engine sim.Engine) *GPUBuilder {
	b.engine = engine
	return b
}

func (b *GPUBuilder) WithFreq(freq sim.Freq) *GPUBuilder {
	b.freq = freq
	return b
}

func (b *GPUBuilder) WithSimulation(sim *simulation.Simulation) *GPUBuilder {
	b.simulation = sim
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

// WithL2CacheSize sets the size of the L2 cache.
func (b GPUBuilder) WithL2CacheSize(size uint64) GPUBuilder {
	b.l2CacheSize = size
	return b
}

// WithDRAMSize sets the size of DRAMs in the GPU.
func (b GPUBuilder) WithDRAMSize(size uint64) GPUBuilder {
	b.DramSize = size
	return b
}

// WithLog2CacheLineSize sets the cache line size with the power of 2.
func (b GPUBuilder) WithLog2CacheLineSize(
	log2CacheLine uint64,
) GPUBuilder {
	b.log2CacheLineSize = log2CacheLine
	return b
}

// WithNumMemoryBank sets the number of L2 cache modules and number of memory
// controllers in each GPU.
func (b GPUBuilder) WithNumMemoryBank(n int) GPUBuilder {
	b.numMemoryBank = n
	return b
}

func (b GPUBuilder) WithGPU2SMThreadBlockAllocationLatency(l uint64) GPUBuilder {
	b.GPU2SMThreadBlockAllocationLatency = l
	return b
}

func (b GPUBuilder) WithSM2SMSPWarpIssueLatency(l uint64) GPUBuilder {
	b.SM2SMSPWarpIssueLatency = l
	return b
}

func (b GPUBuilder) WithSMReceiveGPULatency(l uint64) GPUBuilder {
	b.SMReceiveGPULatency = l
	return b
}

func (b GPUBuilder) WithGPUReceiveSMLatency(l uint64) GPUBuilder {
	b.GPUReceiveSMLatency = l
	return b
}

func (b *GPUBuilder) Build(name string) *GPUController {
	// g := &GPUController{
	// 	ID:  sim.GetIDGenerator().Generate(),
	// 	SMs: make(map[string]*sm.SMController),
	// }
	b.log2MemoryBankInterleavingSize = 7

	b.createGPU(name)

	b.l1AddressMapper = mem.NewInterleavedAddressPortMapper(
		1 << b.log2MemoryBankInterleavingSize,
	)
	b.memAddrOffset = uint64(0)
	// b.l1AddressMapper.LowAddress = b.memAddrOffset
	// b.l1AddressMapper.HighAddress = b.memAddrOffset + b.DramSize
	b.l1AddressMapper.UseAddressSpaceLimitation = false

	b.gpu.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, b.gpu)
	b.buildPortsForGPU(b.gpu, name)
	sms := b.buildSMs(name)
	b.buildDRAMControllers()
	b.buildL2Caches()

	b.connectGPUWithSMs(b.gpu, sms)
	b.connectL2AndDRAM()
	b.connectL1ToL2()

	for i := uint64(0); i < b.smsCount; i++ {
		smID := b.gpu.SMList[i].ID
		b.gpu.SMAssignedThreadTable[smID] = 0
		b.gpu.SMAssignedCTACountTable[smID] = 0
	}

	// b.connectGPUWithDRAM(b.gpu, b.Dram)
	// b.connectGPUControllerToDRAM(b.gpu, b.DRAM)
	// b.connectGPUControllerToSMSPs(b.gpu, sms, b.DRAM)

	// b.gpu.PendingWriteReq = make(map[string]*mem.WriteReq)
	// b.gpu.PendingReadReq = make(map[string]*mem.ReadReq)
	// b.gpu.PendingSMSPtoGPUControllerMemReadReq = make(map[string]*message.SMSPToGPUControllerMemReadMsg)
	// b.gpu.PendingSMSPtoGPUControllerMemWriteReq = make(map[string]*message.SMSPToGPUControllerMemWriteMsg)
	// b.gpu.PendingCacheReadReq = make(map[string]*message.GPUControllerToCachesMemReadMsg)
	// b.gpu.PendingCacheWriteReq = make(map[string]*message.GPUControllerToCachesMemWriteMsg)

	// b.connectL1ToL2()

	atexit.Register(b.gpu.LogStatus)

	return b.gpu
}

func (b *GPUBuilder) createGPU(name string) {
	b.gpuName = name

	b.gpu = &GPUController{
		gpuName:                 name,
		ID:                      sim.GetIDGenerator().Generate(),
		SMs:                     make(map[string]*sm.SMController),
		SMIssueIndex:            0,
		smsCount:                b.smsCount,
		SMAssignedThreadTable:   make(map[string]uint64),
		SMAssignedCTACountTable: make(map[string]uint64),
		SMThreadCapacity:        2048,

		GPU2SMThreadBlockAllocationLatency:          b.GPU2SMThreadBlockAllocationLatency,
		GPU2SMThreadBlockAllocationLatencyRemaining: b.GPU2SMThreadBlockAllocationLatency,
		GPUReceiveSMLatency:                         b.GPUReceiveSMLatency,
		GPUReceiveSMLatencyRemaining:                b.GPUReceiveSMLatency,
		// threadBlockAllocationLatency:   b.threadBlockAllocationLatency,
	}
	// b.gpu.Domain = sim.NewDomain(b.gpuName)
	// b.gpuID = id
}

func (b *GPUBuilder) buildPortsForGPU(g *GPUController, name string) {
	g.toDriver = sim.NewPort(g, 4096, 4096, fmt.Sprintf("%s.ToDriver", name))
	g.toSMs = sim.NewPort(g, 4096, 4096, fmt.Sprintf("%s.ToSMs", name))
	g.AddPort(fmt.Sprintf("%s.ToDriver", name), g.toDriver)
	g.AddPort(fmt.Sprintf("%s.ToSMs", name), g.toSMs)

	// cache updates
	// 	g.toSMMem = sim.NewPort(g,4096, 4096, fmt.Sprintf("%s.ToSMMem", name))
	// 	g.AddPort(fmt.Sprintf("%s.ToSMMem", name), g.toSMMem)

	// g.ToCaches = sim.NewPort(g, 4096, 4096, fmt.Sprintf("%s.ToCaches", name))
	// g.AddPort(fmt.Sprintf("%s.ToCaches", name), g.ToCaches)
	// g.ToSMSPsMem = sim.NewPort(g, 4096, 4096, fmt.Sprintf("%s.ToSMSPsMem", name))
	// g.AddPort(fmt.Sprintf("%s.ToSMSPsMem", name), g.ToSMSPsMem)
}

func (b *GPUBuilder) buildSMs(gpuName string) []*sm.SMController {
	smBuilder := sm.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithSimulation(b.simulation).
		WithSMSPsCount(b.smspsCountPerSM).
		WithL1AddressMapper(b.l1AddressMapper).
		WithSM2SMSPWarpIssueLatency(b.SM2SMSPWarpIssueLatency).
		WithSMReceiveGPULatency(b.SMReceiveGPULatency)

	sms := []*sm.SMController{}
	for i := uint64(0); i < b.smsCount; i++ {
		sm := smBuilder.Build(fmt.Sprintf("%s.SM(%d)", gpuName, i))
		// b.simulation.RegisterComponent(sm)
		sms = append(sms, sm)
	}

	return sms
}

func (b *GPUBuilder) connectGPUWithSMs(gpu *GPUController, sms []*sm.SMController) {
	// 	conn := sim.NewDirectConnection("GPUToSMs", b.engine, 1*sim.GHz)
	// conn.PlugIn(gpu.toSMs, 4)
	conn := directconnection.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(1 * sim.GHz).
		Build("GPUToSMs")
	conn.PlugIn(gpu.toSMs)
	b.simulation.RegisterComponent(conn)
	// conn.PlugIn(gpu.toSMMem)

	for i := range sms {
		sm := sms[i]

		// gpu.freeSMs = append(gpu.freeSMs, sm)
		gpu.SMList = append(gpu.SMList, sm)
		gpu.SMs[sm.ID] = sm

		sm.SetGPURemotePort(gpu.toSMs)
		// fmt.Printf("GPU %s set ToSMSPsMem to %s\n", gpu.Name(), gpu.ToSMSPsMem.Name())
		// sm.SetGPUControllerCachesPort(gpu.ToSMSPsMem)
		// sm.SetGPUMemRemotePort(gpu.toSMMem)
		// for _, smspID := range sm.SMSPsIDs {
		// 	// sm.SMSPs[smspID].SetMemRemote(d.GetPortByName("Top"))
		// }

		conn.PlugIn(sm.GetPortByName(fmt.Sprintf("%s.ToGPU", sms[i].Name())))
		// conn.PlugIn(sm.GetPortByName(fmt.Sprintf("%s.ToGPUMem", sms[i].Name())))
	}
}

// func (b *GPUBuilder) connectGPUWithDRAM(gpu *GPUController, d *idealmemcontroller.Comp) {
// 	// 	conn := sim.NewDirectConnection("GPUToSMs", b.engine, 1*sim.GHz)
// 	// conn.PlugIn(gpu.toSMs, 4)
// 	conn := directconnection.MakeBuilder().
// 		WithEngine(b.engine).
// 		WithFreq(1 * sim.GHz).
// 		Build("GPUToDRAM")
// 	conn.PlugIn(gpu.toDRAM)

// 	gpu.toDRAMRemote = d.GetPortByName("Top")
// 	conn.PlugIn(gpu.toDRAMRemote)
// }

// For DRAM-only version
// func (b *GPUBuilder) connectGPUControllerToDRAM(gpu *GPUController, d *idealmemcontroller.Comp) {
// 	// 	conn := sim.NewDirectConnection("GPUToSMs", b.engine, 1*sim.GHz)
// 	// conn.PlugIn(gpu.toSMs, 4)
// 	conn := directconnection.MakeBuilder().
// 		WithEngine(b.engine).
// 		WithFreq(1 * sim.GHz).
// 		Build("GPUControllerToDRAM")
// 	conn.PlugIn(gpu.ToCaches)

// 	gpu.ToDRAM = d.GetPortByName("Top")
// 	conn.PlugIn(gpu.ToDRAM)
// }

// func (b *GPUBuilder) connectGPUControllerToSMSPs(gpu *GPUController, sms []*sm.SMController, d *idealmemcontroller.Comp) {
// 	// 	conn := sim.NewDirectConnection("GPUToSMs", b.engine, 1*sim.GHz)
// 	// conn.PlugIn(gpu.toSMs, 4)
// 	// conn := directconnection.MakeBuilder().
// 	// 	WithEngine(b.engine).
// 	// 	WithFreq(1 * sim.GHz).
// 	// 	Build("GPUControllerToSMSPs")
// 	// conn.PlugIn(gpu.ToSMSPsMem)

// 	conn := directconnection.MakeBuilder().
// 		WithEngine(b.engine).
// 		WithFreq(1 * sim.GHz).
// 		Build("SMSPsToMem")
// 	conn.PlugIn(d.GetPortByName("Top"))
// 	b.simulation.RegisterComponent(conn)

// 	for i := range sms {
// 		sm := sms[i]
// 		for j := uint64(0); j < b.smspsCountPerSM; j++ {
// 			smspID := sm.SMSPsIDs[j]
// 			smsp := sm.SMSPs[smspID]
// 			conn.PlugIn(smsp.ToMem)
// 		}
// 	}
// }

// func (b *GPUBuilder) buildL2Caches() {
// 	byteSize := b.L2CacheSize / uint64(b.numMemoryBank)
// 	l2Builder := writeback.MakeBuilder().
// 		WithEngine(b.engine).
// 		WithFreq(b.freq).
// 		WithLog2BlockSize(b.log2CacheLineSize).
// 		WithWayAssociativity(16).
// 		WithByteSize(byteSize).
// 		WithNumMSHREntry(64).
// 		WithNumReqPerCycle(16)

// 	for i := 0; i < b.numMemoryBank; i++ {
// 		cacheName := fmt.Sprintf("%s.L2[%d]", b.gpuName, i)
// 		l2 := l2Builder.WithInterleaving(
// 			1<<(b.log2MemoryBankInterleavingSize-b.log2CacheLineSize),
// 			b.numMemoryBank,
// 			i,
// 		).Build(cacheName)
// 		b.L2Caches = append(b.L2Caches, l2)
// 		b.gpu.L2Caches = append(b.gpu.L2Caches, l2)

// 		// if b.enableVisTracing {
// 		// 	tracing.CollectTrace(l2, b.visTracer)
// 		// }

// 		// if b.enableMemTracing {
// 		// 	tracing.CollectTrace(l2, b.memTracer)
// 		// }

// 		// if b.monitor != nil {
// 		// 	b.monitor.RegisterComponent(l2)
// 		// }
// 	}
// }

// cache updates

func (b *GPUBuilder) buildDRAMControllers() {
	// memCtrlBuilder := b.createDramControllerBuilder()

	// for i := 0; i < b.numMemoryBank; i++ {
	// 	dramName := fmt.Sprintf("%s.DRAM[%d]", b.gpuName, i)
	// 	dram := idealmemcontroller.MakeBuilder().
	// 		WithEngine(b.engine).
	// 		WithFreq(b.freq).
	// 		// WithLatency(100).
	// 		// WithStorage(b.globalStorage).
	// 		Build(dramName)
	// 	b.drams = append(b.drams, dram)

	// 	// if b.enableMemTracing {
	// 	// 	tracing.CollectTrace(dram, b.memTracer)
	// 	// }
	// }
	for i := 0; i < b.numMemoryBank; i++ {
		dramName := fmt.Sprintf("%s.DRAM[%d]", b.gpuName, i)
		dram := idealmemcontroller.MakeBuilder().
			WithEngine(b.engine).
			WithFreq(b.freq).
			WithLatency(100).
			WithStorage(mem.NewStorage(b.DramSize / uint64(b.numMemoryBank))).
			Build(dramName)
		b.simulation.RegisterComponent(dram)
		b.DRAMs = append(b.DRAMs, dram)
	}
}

func (b *GPUBuilder) buildL2Caches() {
	byteSize := b.l2CacheSize / uint64(b.numMemoryBank)
	l2Builder := writeback.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithLog2BlockSize(b.log2CacheLineSize).
		WithWayAssociativity(16).
		WithByteSize(byteSize).
		WithNumMSHREntry(64).
		WithNumReqPerCycle(16)

	for i := 0; i < b.numMemoryBank; i++ {
		cacheName := fmt.Sprintf("%s.L2Cache[%d]", b.gpuName, i)
		l2 := l2Builder.WithInterleaving(
			1<<(b.log2MemoryBankInterleavingSize-b.log2CacheLineSize),
			b.numMemoryBank,
			i).
			WithAddressMapperType("single").
			WithRemotePorts(b.DRAMs[i].GetPortByName("Top").AsRemote()).
			Build(cacheName)
		b.simulation.RegisterComponent(l2)
		b.l2Caches = append(b.l2Caches, l2)
	}
}

func (b *GPUBuilder) connectL2AndDRAM() {
	b.l2ToDramConnection = directconnection.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		Build(b.gpuName + ".L2ToDRAM")
	b.simulation.RegisterComponent(b.l2ToDramConnection)

	lowModuleFinder := mem.NewInterleavedAddressPortMapper(
		1 << b.log2MemoryBankInterleavingSize)

	for i, l2 := range b.l2Caches {
		b.l2ToDramConnection.PlugIn(l2.GetPortByName("Bottom"))
		l2.SetAddressToPortMapper(&mem.SinglePortMapper{
			Port: b.DRAMs[i].GetPortByName("Top").AsRemote(),
		})
	}

	for _, dram := range b.DRAMs {
		b.l2ToDramConnection.PlugIn(dram.GetPortByName("Top"))
		lowModuleFinder.LowModules = append(lowModuleFinder.LowModules,
			dram.GetPortByName("Top").AsRemote())
	}

	// 	// b.dmaEngine.SetLocalDataSource(lowModuleFinder)
	// 	// b.l2ToDramConnection.PlugIn(b.dmaEngine.ToMem)

	// // b.pmc.MemCtrlFinder = lowModuleFinder
	// // b.l2ToDramConnection.PlugIn(
	// // 	b.pmc.GetPortByName("LocalMem"))
}

func (b *GPUBuilder) connectL1ToL2() {
	l1ToL2Conn := directconnection.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		Build(b.gpuName + ".L1ToL2")

	for _, l2 := range b.l2Caches {
		l1ToL2Conn.PlugIn(l2.GetPortByName("Top"))
		b.l1AddressMapper.LowModules = append(
			b.l1AddressMapper.LowModules,
			l2.GetPortByName("Top").AsRemote())
	}

	for _, sm := range b.gpu.SMs {
		for i := range b.smspsCountPerSM {
			l1ToL2Conn.PlugIn(
				sm.GetPortByName(fmt.Sprintf("L1VCacheBottom[%d]", i)))
		}
		// l1ToL2Conn.PlugIn(sm.GetPortByName("L1CacheBottom"))

		// l1ToL2Conn.PlugIn(sm.GetPortByName("L1SCacheBottom"))
		// l1ToL2Conn.PlugIn(sm.GetPortByName("L1ICacheBottom"))
	}
}

// func (b *GPUBuilder) buildDRAMControllers() {
// 	// memCtrlBuilder := b.createDramControllerBuilder()

// 	for i := 0; i < b.numMemoryBank; i++ {
// 		// dramName := fmt.Sprintf("%s.DRAM[%d]", b.gpuName, i)
// 		// dram := memCtrlBuilder.
// 		// 	Build(dramName)
// 		// 	fmt.Sprintf("%s.DRAM_%d", b.gpuName, i),
// 		idealmemcontrollerbuilder := idealmemcontroller.MakeBuilder()
// 		dram := idealmemcontrollerbuilder.Build("IMC")
// 		b.Drams = append(b.Drams, dram)
// 		// b.gpu.MemControllers = append(b.gpu.MemControllers, dram)

// 		// if b.enableMemTracing {
// 		// 	tracing.CollectTrace(dram, b.memTracer)
// 		// }

// 		// if b.monitor != nil {
// 		// 	b.monitor.RegisterComponent(dram)
// 		// }
// 	}
// }

// func (b *GPUBuilder) buildPageMigrationController() {
// 	b.pageMigrationController =
// 		pagemigrationcontroller.NewPageMigrationController(
// 			fmt.Sprintf("%s.PMC", b.gpuName),
// 			b.engine,
// 			b.lowModuleFinderForPMC, // ??
// 			nil)
// 	// b.gpu.PMC = b.pageMigrationController

// 	// if b.monitor != nil {
// 	// 	b.monitor.RegisterComponent(b.pageMigrationController)
// 	// }
// }

// func (b *GPUBuilder) buildDMAEngine() {
// 	b.dmaEngine = cp.NewDMAEngine(
// 		fmt.Sprintf("%s.DMA", b.gpuName),
// 		b.engine,
// 		nil)

// 	// if b.enableVisTracing {
// 	// 	tracing.CollectTrace(b.dmaEngine, b.visTracer)
// 	// }

// 	// if b.monitor != nil {
// 	// 	b.monitor.RegisterComponent(b.dmaEngine)
// 	// }
// }

// func (b *GPUBuilder) buildRDMAEngine() {
// 	name := fmt.Sprintf("%s.RDMA", b.gpuName)
// 	b.rdmaEngine = rdma.MakeBuilder().
// 		WithEngine(b.engine).
// 		WithFreq(1 * sim.GHz).
// 		WithLocalModules(b.lowModuleFinderForL1). // ??
// 		Build(name)
// 	b.gpu.RDMAEngine = b.rdmaEngine

// 	// if b.monitor != nil {
// 	// 	b.monitor.RegisterComponent(b.rdmaEngine)
// 	// }

// 	// if b.enableVisTracing {
// 	// 	tracing.CollectTrace(b.rdmaEngine, b.visTracer)
// 	// }
// }

// func (b *GPUBuilder) connectL1ToL2() {
// 	lowModuleFinder := mem.NewInterleavedAddressPortMapper(
// 		1 << b.log2MemoryBankInterleavingSize)
// 	lowModuleFinder.ModuleForOtherAddresses = b.rdmaEngine.ToL1.AsRemote()
// 	lowModuleFinder.UseAddressSpaceLimitation = true
// 	lowModuleFinder.LowAddress = b.memAddrOffset
// 	lowModuleFinder.HighAddress = b.memAddrOffset + 4*mem.GB

// 	l1ToL2Conn := directconnection.MakeBuilder().
// 		WithEngine(b.engine).
// 		WithFreq(b.freq).
// 		Build(b.gpuName + ".L1ToL2")

// 	b.rdmaEngine.SetLocalModuleFinder(lowModuleFinder)
// 	l1ToL2Conn.PlugIn(b.rdmaEngine.ToL1)
// 	l1ToL2Conn.PlugIn(b.rdmaEngine.ToL2)

// 	for _, l2 := range b.L2Caches {
// 		lowModuleFinder.LowModules = append(lowModuleFinder.LowModules,
// 			l2.GetPortByName("Top").AsRemote())
// 		l1ToL2Conn.PlugIn(l2.GetPortByName("Top"))
// 	}

// 	for _, l1 := range b.L1Caches {
// 		l1.SetAddressToPortMapper(lowModuleFinder)
// 		l1ToL2Conn.PlugIn(l1.GetPortByName("Bottom"))
// 	}

// 	// for _, l1s := range b.l1sCaches {
// 	// 	l1s.SetAddressToPortMapper(lowModuleFinder)
// 	// 	l1ToL2Conn.PlugIn(l1s.GetPortByName("Bottom"))
// 	// }

// 	// for _, l1iAT := range b.l1iAddrTrans {
// 	// 	l1iAT.SetAddressToPortMapper(lowModuleFinder)
// 	// 	l1ToL2Conn.PlugIn(l1iAT.GetPortByName("Bottom"))
// 	// }
// }

// func (b *GPUBuilder) connectL2AndDRAM() {
// 	b.l2ToDramConnection = directconnection.MakeBuilder().
// 		WithEngine(b.engine).
// 		WithFreq(b.freq).
// 		Build(b.gpuName + ".L2ToDRAM")

// 	lowModuleFinder := mem.NewInterleavedAddressPortMapper(
// 		1 << b.log2MemoryBankInterleavingSize)

// 	for i, l2 := range b.L2Caches {
// 		b.l2ToDramConnection.PlugIn(l2.GetPortByName("Bottom"))
// 		l2.SetAddressToPortMapper(&mem.SinglePortMapper{
// 			Port: b.Drams[i].GetPortByName("Top").AsRemote(),
// 		})
// 	}

// 	for _, dram := range b.Drams {
// 		b.l2ToDramConnection.PlugIn(dram.GetPortByName("Top"))
// 		lowModuleFinder.LowModules = append(lowModuleFinder.LowModules,
// 			dram.GetPortByName("Top").AsRemote())
// 	}

// 	b.dmaEngine.SetLocalDataSource(lowModuleFinder)
// 	b.l2ToDramConnection.PlugIn(b.dmaEngine.ToMem)

// 	b.pageMigrationController.MemCtrlFinder = lowModuleFinder
// 	b.l2ToDramConnection.PlugIn(
// 		b.pageMigrationController.GetPortByName("LocalMem"))
// }
