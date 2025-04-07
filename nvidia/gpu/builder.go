package gpu

import (
	"fmt"

	"github.com/sarchlab/akita/v4/mem/cache/writeback"
	"github.com/sarchlab/akita/v4/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"github.com/sarchlab/mgpusim/v4/amd/timing/cp"
	"github.com/sarchlab/mgpusim/v4/amd/timing/pagemigrationcontroller"
	"github.com/sarchlab/mgpusim/v4/amd/timing/rdma" //
	"github.com/sarchlab/mgpusim/v4/nvidia/sm"
	"github.com/tebeka/atexit"
)

type GPUBuilder struct {
	gpuName string
	gpu     *GPU

	engine sim.Engine
	freq   sim.Freq

	smsCount        uint64
	smspsCountPerSM uint64

	L2Caches                       []*writeback.Comp
	L2CacheSize                    uint64
	Drams                          []*idealmemcontroller.Comp //[]*dram.Comp
	DramSize                       uint64
	log2CacheLineSize              uint64
	numMemoryBank                  int
	log2MemoryBankInterleavingSize uint64

	l1ToL2Connection        *directconnection.Comp
	l2ToDramConnection      *directconnection.Comp
	rdmaEngine              *rdma.Comp
	dmaEngine               *cp.DMAEngine
	pageMigrationController *pagemigrationcontroller.PageMigrationController
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

// WithL2CacheSize set the total L2 cache size. The size of the L2 cache is
// split between memory banks.
func (b GPUBuilder) WithL2CacheSize(size uint64) GPUBuilder {
	b.L2CacheSize = size
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

func (b *GPUBuilder) Build(name string) *GPU {
	// g := &GPU{
	// 	ID:  sim.GetIDGenerator().Generate(),
	// 	SMs: make(map[string]*sm.SM),
	// }
	b.createGPU(name)

	b.gpu.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, b.gpu)
	b.buildPortsForGPU(b.gpu, name)
	sms := b.buildSMs(name)
	b.connectGPUWithSMs(b.gpu, sms)

	b.buildL2Caches()
	b.buildDRAMControllers()

	b.buildDMAEngine()
	// b.buildRDMAEngine()
	// b.buildPageMigrationController()

	// b.connectL2AndDRAM()
	// b.connectL1ToL2()

	atexit.Register(b.gpu.LogStatus)

	return b.gpu
}

func (b *GPUBuilder) createGPU(name string) {
	b.gpuName = name

	b.gpu = &GPU{
		gpuName: name,
		ID:      sim.GetIDGenerator().Generate(),
		SMs:     make(map[string]*sm.SM),
	}
	// b.gpu.Domain = sim.NewDomain(b.gpuName)
	// b.gpuID = id
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

func (b *GPUBuilder) buildL2Caches() {
	byteSize := b.L2CacheSize / uint64(b.numMemoryBank)
	l2Builder := writeback.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithLog2BlockSize(b.log2CacheLineSize).
		WithWayAssociativity(16).
		WithByteSize(byteSize).
		WithNumMSHREntry(64).
		WithNumReqPerCycle(16)

	for i := 0; i < b.numMemoryBank; i++ {
		cacheName := fmt.Sprintf("%s.L2[%d]", b.gpuName, i)
		l2 := l2Builder.WithInterleaving(
			1<<(b.log2MemoryBankInterleavingSize-b.log2CacheLineSize),
			b.numMemoryBank,
			i,
		).Build(cacheName)
		b.L2Caches = append(b.L2Caches, l2)
		b.gpu.L2Caches = append(b.gpu.L2Caches, l2)

		// if b.enableVisTracing {
		// 	tracing.CollectTrace(l2, b.visTracer)
		// }

		// if b.enableMemTracing {
		// 	tracing.CollectTrace(l2, b.memTracer)
		// }

		// if b.monitor != nil {
		// 	b.monitor.RegisterComponent(l2)
		// }
	}
}

func (b *GPUBuilder) buildDRAMControllers() {
	// memCtrlBuilder := b.createDramControllerBuilder()

	for i := 0; i < b.numMemoryBank; i++ {
		// dramName := fmt.Sprintf("%s.DRAM[%d]", b.gpuName, i)
		// dram := memCtrlBuilder.
		// 	Build(dramName)
		// 	fmt.Sprintf("%s.DRAM_%d", b.gpuName, i),
		idealmemcontrollerbuilder := idealmemcontroller.MakeBuilder()
		dram := idealmemcontrollerbuilder.Build("IMC")
		b.Drams = append(b.Drams, dram)
		// b.gpu.MemControllers = append(b.gpu.MemControllers, dram)

		// if b.enableMemTracing {
		// 	tracing.CollectTrace(dram, b.memTracer)
		// }

		// if b.monitor != nil {
		// 	b.monitor.RegisterComponent(dram)
		// }
	}
}

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

func (b *GPUBuilder) buildDMAEngine() {
	b.dmaEngine = cp.NewDMAEngine(
		fmt.Sprintf("%s.DMA", b.gpuName),
		b.engine,
		nil)

	// if b.enableVisTracing {
	// 	tracing.CollectTrace(b.dmaEngine, b.visTracer)
	// }

	// if b.monitor != nil {
	// 	b.monitor.RegisterComponent(b.dmaEngine)
	// }
}

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
