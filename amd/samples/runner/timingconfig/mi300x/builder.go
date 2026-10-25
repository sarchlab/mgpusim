// Package mi300x contains the configuration of GPUs similar to AMD Instinct
// MI300X.
package mi300x

import (
	"fmt"

	monitoring2 "github.com/sarchlab/akita/v5/monitoring"
	"github.com/sarchlab/mgpusim/v5/amd/sampling"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	writethroughcache "github.com/sarchlab/akita/v5/mem/cache/writethrough"
	"github.com/sarchlab/akita/v5/mem/rob"
	"github.com/sarchlab/akita/v5/mem/simplebankedmemory"
	"github.com/sarchlab/akita/v5/mem/vm/addresstranslator"
	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	directconnection "github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/emu/cdna3"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/gpubuilder"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/shaderarray"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cu"
	"github.com/sarchlab/mgpusim/v5/amd/timing/rdma"
)

// MI300X hardware configuration constants.
//
// The MI300X has 304 active compute units organized as 8 XCDs (accelerator
// complex dies) of 38 CUs each. Per the CDNA3 white paper a shader array holds
// 2 CUs (sharing one 64 KB L1 instruction cache), so each XCD is 19 shader
// arrays and the device is 8 x 19 x 2 = 304 CUs. We model that hierarchy
// directly: NumXCD dies, each NumShaderArrayPerXCD shader arrays, each
// NumCUPerShaderArray CUs. The XCD grouping is what the per-die dispatchers use.
//
// The L2/MALL cache hierarchy is still modeled as a single device-wide L2
// (4 MB, address-interleaved over numMemoryBank banks) backed by a 256 MB MALL.
// A true per-XCD L2 (8 x 4 MB = 32 MB) and 128 B cache lines are a separate
// follow-up; they are entangled with the cache_latency calibration and are not
// needed for per-die dispatch.
const (
	// NumCUPerShaderArray is the number of compute units per shader array
	// (CDNA3: 2 CUs share one L1 instruction cache).
	NumCUPerShaderArray = 2
	// NumShaderArrayPerXCD is the number of shader arrays in one XCD
	// (19 x 2 CU = 38 CUs per XCD).
	NumShaderArrayPerXCD = 19
	// NumXCD is the number of accelerator complex dies.
	NumXCD = 8
	// NumShaderArray is the total number of shader arrays in the GPU
	// (8 XCD x 19 SA = 152 SAs; 152 x 2 CU = 304 CUs).
	NumShaderArray = NumXCD * NumShaderArrayPerXCD
)

// Port buffer sizes. The CP and DMA-to-CP ports mirror the v4 4096-deep
// buffers (v4 used 40M for DMA ToCP; 4096 is plenty). The other sizes mirror
// the v4 component builders' internal port sizes.
const (
	cpPortBufSize      = 4096
	dmaToCPBufSize     = 4096
	dmaToMemBufSize    = 64
	rdmaPortBufSize    = 128
	dramTopPortBufSize = 1024 // v4: WithTopPortBufferSize(1024)
	l2TLBPortBufSize   = 1024
	ctrlPortBufSize    = 1
	l2CachePortBufSize = 128 // v4 writeback: 2 * NumReqPerCycle(64)
)

// Builder builds a hardware platform for timing simulation.
type Builder struct {
	sampler    *sampling.SampledEngine
	simulation *simulation.Simulation
	monitor    *monitoring2.Monitor

	gpuID                          uint64
	name                           string
	freq                           timing.Freq
	numCUPerShaderArray            int
	numShaderArray                 int
	l2CacheSize                    uint64
	l2BankLatency                  int
	mallCacheSize                  uint64
	mallBankLatency                int
	numMemoryBank                  int
	log2CacheLineSize              uint64
	log2PageSize                   uint64
	log2MemoryBankInterleavingSize uint64
	memAddrOffset                  uint64
	dramSize                       uint64
	globalStorage                  *mem.Storage
	mmu                            *mmu.Comp
	rdmaAddressMapper              mem.AddressToPortMapper
	driverPort                     messaging.RemotePort

	gpu                  *gpubuilder.GPU
	cp                   *cp.Comp
	rdmaEngine           *rdma.Comp
	dmaEngine            *cp.DMAComp
	sas                  []*shaderarray.ShaderArray
	l2Caches             []*writeback.Comp
	malls                []*writethroughcache.Comp
	l2TLBs               []*tlb.Comp
	drams                []*simplebankedmemory.Comp
	internalConn         *directconnection.Connection
	l2ToMALLConnection   *directconnection.Connection
	mallToDramConnection *directconnection.Connection
	l1AddressMapper      *mem.AddressMapper
	l1TLBAddressMapper   *mem.AddressMapper
	dmaLocalDataSource   *mem.AddressMapper
}

// WithSampler shares the simulation's recoverable wavefront sampling state.
func (b Builder) WithSampler(s *sampling.SampledEngine) Builder { b.sampler = s; return b }

// MakeBuilder creates a new builder with MI300X default configuration.
func MakeBuilder() Builder {
	return Builder{
		// MI300X boost clock (TechPowerUp). CUs, caches, and the dispatcher run
		// in this clock domain. The cycle-denominated latencies below were
		// re-derived for 2.1 GHz (scaled x2100/1700 from the old 1.7 GHz values)
		// so their nanosecond targets are preserved. NOTE: the DRAM controllers
		// are pinned at 1 GHz (see buildDRAMControllers), so HBM bandwidth is
		// independent of this clock and was not rescaled.
		freq:                2100 * timing.MHz,
		numCUPerShaderArray: NumCUPerShaderArray,
		numShaderArray:      NumShaderArray,
		// MI300X L2 is a per-XCD 4 MB cache (16-way, 16x256 KB channels). A
		// single work-item touches only its own XCD's L2, so the cache_latency
		// pointer chase sees 4 MB here, not the device-wide aggregate.
		l2CacheSize: 4 * mem.MB,
		// L2 bank latency in cycles. Tuned so an L1-miss/L2-hit dependent load
		// lands near the ~60-100 ns the real MI300X L2 shows in cache_latency.
		// 124 cyc / 2.1 GHz = ~59 ns (was 100 cyc / 1.7 GHz = ~59 ns).
		l2BankLatency: 124,
		// MI300X adds a 256 MB MALL (Memory-Attached Last-Level / Infinity
		// Cache) on the IODs between the L2 and HBM. Modeling it gives the
		// cache_latency curve its third plateau (8 MB-256 MB working sets).
		mallCacheSize: 256 * mem.MB,
		// MALL bank latency in cycles. Tuned so an L2-miss/MALL-hit dependent
		// load lands near the ~170-330 ns the real MI300X MALL shows.
		// 371 cyc / 2.1 GHz = ~177 ns (was 300 cyc / 1.7 GHz = ~176 ns).
		mallBankLatency:                371,
		numMemoryBank:                  16,
		log2CacheLineSize:              6,
		log2PageSize:                   12,
		log2MemoryBankInterleavingSize: 7,
		memAddrOffset:                  0,
		dramSize:                       4 * mem.GB,
	}
}

// WithMonitor sets the optional dispatch progress monitor.
func (b Builder) WithMonitor(m *monitoring2.Monitor) Builder { b.monitor = m; return b }

// WithSimulation sets the simulation to use.
func (b Builder) WithSimulation(sim *simulation.Simulation) Builder {
	b.simulation = sim
	return b
}

// WithGPUID sets the GPU ID to use.
func (b Builder) WithGPUID(id uint64) gpubuilder.GPUBuilder {
	b.gpuID = id
	return b
}

// WithFreq sets the frequency that the GPU works at.
func (b Builder) WithFreq(freq timing.Freq) Builder {
	b.freq = freq
	return b
}

// WithLog2MemoryBankInterleavingSize sets the log2 memory bank interleaving
// size.
func (b Builder) WithLog2MemoryBankInterleavingSize(size uint64) Builder {
	b.log2MemoryBankInterleavingSize = size
	return b
}

// WithLog2CacheLineSize sets the log2 cache line size.
func (b Builder) WithLog2CacheLineSize(size uint64) Builder {
	b.log2CacheLineSize = size
	return b
}

// WithLog2PageSize sets the log2 page size.
func (b Builder) WithLog2PageSize(size uint64) Builder {
	b.log2PageSize = size
	return b
}

// WithMemAddrOffset sets the memory address offset.
func (b Builder) WithMemAddrOffset(offset uint64) gpubuilder.GPUBuilder {
	b.memAddrOffset = offset
	return b
}

// WithNumCUPerShaderArray sets the number of CUs per shader array.
func (b Builder) WithNumCUPerShaderArray(numCUPerShaderArray int) Builder {
	b.numCUPerShaderArray = numCUPerShaderArray
	return b
}

// WithNumShaderArray sets the number of shader arrays.
func (b Builder) WithNumShaderArray(numShaderArray int) Builder {
	b.numShaderArray = numShaderArray
	return b
}

// WithL2CacheSize sets the size of the L2 cache.
func (b Builder) WithL2CacheSize(size uint64) Builder {
	b.l2CacheSize = size
	return b
}

// WithL2BankLatency sets the L2 cache bank latency in cycles.
func (b Builder) WithL2BankLatency(latency int) Builder {
	b.l2BankLatency = latency
	return b
}

// WithMALLCacheSize sets the size of the MALL (Infinity Cache) last-level
// cache.
func (b Builder) WithMALLCacheSize(size uint64) Builder {
	b.mallCacheSize = size
	return b
}

// WithMALLBankLatency sets the MALL cache bank latency in cycles.
func (b Builder) WithMALLBankLatency(latency int) Builder {
	b.mallBankLatency = latency
	return b
}

// WithNumMemoryBank sets the number of memory banks.
func (b Builder) WithNumMemoryBank(numMemoryBank int) Builder {
	b.numMemoryBank = numMemoryBank
	return b
}

// WithDramSize sets the size of the DRAM.
func (b Builder) WithDramSize(size uint64) Builder {
	b.dramSize = size
	return b
}

// WithMMU sets the MMU that can provide the ultimate address translation.
func (b Builder) WithMMU(mmu *mmu.Comp) Builder {
	b.mmu = mmu
	return b
}

// WithGlobalStorage sets the global storage.
func (b Builder) WithGlobalStorage(
	globalStorage *mem.Storage,
) Builder {
	b.globalStorage = globalStorage
	return b
}

// WithRDMAAddressMapper sets the RDMA address mapper.
func (b Builder) WithRDMAAddressMapper(
	mapper mem.AddressToPortMapper,
) gpubuilder.GPUBuilder {
	b.rdmaAddressMapper = mapper
	return b
}

// WithDriverPort sets the driver port that the command processor responds
// to.
func (b Builder) WithDriverPort(
	port messaging.RemotePort,
) gpubuilder.GPUBuilder {
	b.driverPort = port
	return b
}

// Build builds the hardware platform.
func (b Builder) Build(name string) *gpubuilder.GPU {
	b.name = name

	b.l1AddressMapper = &mem.AddressMapper{InterleavingSize: 1 << b.log2MemoryBankInterleavingSize}
	b.l1AddressMapper.RangeLow = b.memAddrOffset
	b.l1AddressMapper.RangeHigh = b.memAddrOffset + b.dramSize

	b.l1TLBAddressMapper = &mem.AddressMapper{}

	// Build order is bottom-up so the mappers the shader arrays snapshot at
	// build time (the v5 caches inline the mapper contents into their Spec)
	// are fully populated before the shader arrays are built.
	b.buildDRAMControllers()
	b.buildMALL()
	b.buildL2Caches()
	b.buildCP()
	b.buildL2TLB()
	b.buildSAs()

	b.connectCP()
	b.connectL2AndMALL()
	b.connectMALLAndDRAM()
	b.connectL1ToL2()
	b.connectL1TLBToL2TLB()

	b.populateGPU()

	return b.gpu
}

// buildPort creates a port instance for a declared port and assigns it to
// the component.
func (b *Builder) buildPort(
	comp interface{ BindPort(string, messaging.Port) },
	name string,
	bufSize int,
) messaging.Port {
	port := twowaybuffered.MakeBuilder().
		WithSimulation(b.simulation).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: bufSize, OutgoingCapacity: bufSize}).
		Build()
	comp.BindPort(name, port)

	return port
}

func (b *Builder) populateGPU() {
	b.gpu = &gpubuilder.GPU{
		Name:                 b.name,
		CommandProcessor:     b.cp,
		CommandProcessorPort: b.cp.Ports.ToDriver,
		RDMARequestPort:      b.rdmaEngine.Ports.RDMARequestOutside,
		RDMADataPort:         b.rdmaEngine.Ports.RDMADataOutside,
	}

	for _, l2TLB := range b.l2TLBs {
		b.gpu.TranslationPorts = append(b.gpu.TranslationPorts,
			l2TLB.Ports.Bottom)
	}
}

func (b *Builder) connectCP() {
	b.internalConn = directconnection.NewConnection(b.name+".InternalConn", b.simulation, b.freq)

	b.internalConn.BindPort(b.cp.Ports.ToDMA)
	b.internalConn.BindPort(b.cp.Ports.ToCaches)
	b.internalConn.BindPort(b.cp.Ports.ToCUs)
	b.internalConn.BindPort(b.cp.Ports.ToTLBs)
	b.internalConn.BindPort(b.cp.Ports.ToAddressTranslators)
	b.internalConn.BindPort(b.cp.Ports.ToRDMA)

	rdmaCtrlPort := b.rdmaEngine.Ports.Ctrl
	b.cp.State.RDMA = rdmaCtrlPort.AsRemote()
	b.internalConn.BindPort(rdmaCtrlPort)

	dmaToCPPort := b.dmaEngine.Ports.ToCP
	b.cp.State.DMAEngine = dmaToCPPort.AsRemote()
	b.internalConn.BindPort(dmaToCPPort)

	b.connectCPWithCUs()
	b.connectCPWithAddressTranslators()
	b.connectCPWithTLBs()
	b.connectCPWithCaches()
	b.connectCPWithDRAMControllers()
}

func (b *Builder) connectCPWithCUs() {
	for _, sa := range b.sas {
		for _, cuComp := range sa.CUs {
			cp.RegisterCU(b.cp, cu.DispatcherView{CU: cuComp})

			b.internalConn.BindPort(cuComp.Ports.Top)
			b.internalConn.BindPort(cuComp.Ports.Ctrl)
		}
	}
}

// connectCPWithAddressTranslators wires the Control ports of the address
// translators and the reorder buffers to the CP. The ROB list is new in v5:
// the CP requires the ROB control ports split from the AT control ports
// (v4 mixed them into the AT list).
func (b *Builder) connectCPWithAddressTranslators() {
	addAT := func(at *addresstranslator.Comp) {
		ctrlPort := at.Ports.Control
		b.cp.State.AddressTranslators = append(
			b.cp.State.AddressTranslators, ctrlPort.AsRemote())
		b.internalConn.BindPort(ctrlPort)
	}
	addROB := func(robComp *rob.Comp) {
		ctrlPort := robComp.Ports.Control
		b.cp.State.ROBs = append(b.cp.State.ROBs, ctrlPort.AsRemote())
		b.internalConn.BindPort(ctrlPort)
	}

	for _, sa := range b.sas {
		for i := range b.numCUPerShaderArray {
			addAT(sa.L1VATs[i])
			addROB(sa.L1VROBs[i])
		}

		addAT(sa.L1SAT)
		addROB(sa.L1SROB)

		addAT(sa.L1IAT)
		addROB(sa.L1IROB)
	}
}

func (b *Builder) connectCPWithTLBs() {
	addTLB := func(tlbComp *tlb.Comp) {
		ctrlPort := tlbComp.Ports.Control
		b.cp.State.TLBs = append(b.cp.State.TLBs, ctrlPort.AsRemote())
		b.internalConn.BindPort(ctrlPort)
	}

	for _, sa := range b.sas {
		for i := range b.numCUPerShaderArray {
			addTLB(sa.L1VTLBs[i])
		}

		addTLB(sa.L1STLB)
		addTLB(sa.L1ITLB)
	}

	for _, l2TLB := range b.l2TLBs {
		addTLB(l2TLB)
	}
}

func (b *Builder) connectCPWithCaches() {
	for _, sa := range b.sas {
		for i := range b.numCUPerShaderArray {
			ctrlPort := sa.L1VCaches[i].Ports.Control
			b.cp.State.L1VCaches = append(
				b.cp.State.L1VCaches, ctrlPort.AsRemote())
			b.internalConn.BindPort(ctrlPort)
		}

		l1sCtrlPort := sa.L1SCache.Ports.Control
		b.cp.State.L1SCaches = append(
			b.cp.State.L1SCaches, l1sCtrlPort.AsRemote())
		b.internalConn.BindPort(l1sCtrlPort)

		l1iCtrlPort := sa.L1ICache.Ports.Control
		b.cp.State.L1ICaches = append(
			b.cp.State.L1ICaches, l1iCtrlPort.AsRemote())
		b.internalConn.BindPort(l1iCtrlPort)
	}

	for _, c := range b.l2Caches {
		ctrlPort := c.Ports.Control
		b.cp.State.L2Caches = append(b.cp.State.L2Caches, ctrlPort.AsRemote())
		b.internalConn.BindPort(ctrlPort)
	}

	// The MALL is deliberately NOT registered in the CP control list: it is a
	// write-around cache backing authoritative DRAM, so it never needs draining
	// or flushing. Its Control port is still plugged into the internal
	// connection so the control protocol topology stays fully connected.
	for _, c := range b.malls {
		b.internalConn.BindPort(c.Ports.Control)
	}
}

// connectCPWithDRAMControllers records the Control ports of the DRAM
// controllers in the CP state. The CP currently sends no commands to them,
// but the ports must be connected so the control protocol is reachable.
func (b *Builder) connectCPWithDRAMControllers() {
	for _, dramComp := range b.drams {
		ctrlPort := dramComp.Ports.Control
		b.cp.State.DRAMControllers = append(
			b.cp.State.DRAMControllers, ctrlPort.AsRemote())
		b.internalConn.BindPort(ctrlPort)
	}
}

func (b *Builder) connectL1ToL2() {
	l1ToL2Conn := directconnection.NewConnection(b.name+".L1ToL2", b.simulation, b.freq)

	l1ToL2Conn.BindPort(b.rdmaEngine.Ports.RDMARequestInside)
	l1ToL2Conn.BindPort(b.rdmaEngine.Ports.RDMADataInside)

	for _, l2 := range b.l2Caches {
		l1ToL2Conn.BindPort(l2.Ports.Top)
	}

	for _, sa := range b.sas {
		for i := range b.numCUPerShaderArray {
			l1ToL2Conn.BindPort(sa.L1VCaches[i].Ports.Bottom)
		}

		l1ToL2Conn.BindPort(sa.L1SCache.Ports.Bottom)
		// The instruction path egress to L2 is the L1I address translator's
		// bottom port (the L1I cache sits above its AT).
		l1ToL2Conn.BindPort(sa.L1IAT.Ports.Bottom)
	}
}

func (b *Builder) connectL2AndMALL() {
	b.l2ToMALLConnection = directconnection.NewConnection(b.name+".L2ToMALL", b.simulation, b.freq)

	for _, l2 := range b.l2Caches {
		b.l2ToMALLConnection.BindPort(l2.Ports.Bottom)
	}

	for _, mall := range b.malls {
		topPort := mall.Ports.Top
		b.l2ToMALLConnection.BindPort(topPort)

		// The MALL is a memory-side cache: it fronts DRAM for ALL memory
		// traffic, not just GPU L2 misses. So the DMA engine (host MemCopy)
		// targets the MALL too, via the same interleaving the L1->L2 path
		// uses. This keeps the MALL coherent with host copies and lets an
		// H2D copy populate it (write-through), so a subsequent kernel reads
		// resident data instead of always missing to DRAM.
		b.dmaLocalDataSource.Destinations = append(
			b.dmaLocalDataSource.Destinations, topPort.AsRemote())
	}

	b.l2ToMALLConnection.BindPort(b.dmaEngine.Ports.ToMem)
}

func (b *Builder) connectMALLAndDRAM() {
	b.mallToDramConnection = directconnection.NewConnection(b.name+".MALLToDRAM", b.simulation, b.freq)

	// MALL is the sole front end for DRAM: the only things on this connection
	// are the MALL bottoms and the DRAM tops. GPU L2 misses and host DMA both
	// reach DRAM through the MALL (wired in connectL2AndMALL), so the MALL is a
	// true proxy of DRAM for every memory access.
	for _, mall := range b.malls {
		b.mallToDramConnection.BindPort(mall.Ports.Bottom)
	}

	for _, dramComp := range b.drams {
		b.mallToDramConnection.BindPort(dramComp.Ports.Top)
	}
}

func (b *Builder) connectL1TLBToL2TLB() {
	tlbConn := directconnection.NewConnection(b.name+".L1TLBToL2TLB", b.simulation, b.freq)

	tlbConn.BindPort(b.l2TLBs[0].Ports.Top)

	for _, sa := range b.sas {
		for i := range b.numCUPerShaderArray {
			tlbConn.BindPort(sa.L1VTLBs[i].Ports.Bottom)
		}

		tlbConn.BindPort(sa.L1STLB.Ports.Bottom)
		tlbConn.BindPort(sa.L1ITLB.Ports.Bottom)
	}
}

func (b *Builder) buildSAs() {
	saBuilder := shaderarray.MakeBuilder().WithSampler(b.sampler).
		WithSimulation(b.simulation).
		WithFreq(b.freq).
		WithGPUID(b.gpuID).
		WithNumCUs(b.numCUPerShaderArray).
		WithLog2CacheLineSize(b.log2CacheLineSize).
		WithLog2PageSize(b.log2PageSize).
		WithL1AddressMapper(b.l1AddressMapper).
		WithL1TLBAddressMapper(b.l1TLBAddressMapper).
		// Use the CDNA3 ALU for MI300X timing simulation.
		WithALUBuilder(func() emu.ALU { return cdna3.NewALU(nil) }).
		// Decode with CDNA3 semantics so FLAT/GLOBAL SADDR addressing is
		// resolved correctly (the default disassembler decodes as GCN3, which
		// mis-handles SADDR=0 -> wrong address / "page not found").
		WithDecoderBuilder(func() emu.Decoder {
			d := insts.NewDisassembler()
			d.IsCDNA3 = true
			return d
		}).
		WithWfPoolSize(8).
		WithVGPRCount([]int{32768, 32768, 32768, 32768}).
		WithNumSinglePrecisionUnits(16).
		WithVecMemInstPipelineStages(2).
		WithVecMemTransPipelineStages(4).
		WithVecMemTransPipelineWidth(8).
		WithCUMemPipelineBufferSize(64).
		WithL1VCacheSize(32 * mem.KB).
		WithL1VBankLatency(9). // ~4.3 ns at 2.1 GHz (was 7 cyc / 1.7 GHz)
		WithMemPipelineBufferSize(64).
		WithMaxCoalescingPenalty(3).
		WithRegisterScoreboard(true)

	for i := 0; i < b.numShaderArray; i++ {
		saName := fmt.Sprintf("%s.SA[%d]", b.name, i)
		sa := saBuilder.Build(saName)

		b.sas = append(b.sas, sa)
	}
}

func (b *Builder) buildL2Caches() {
	byteSize := b.l2CacheSize / uint64(b.numMemoryBank)

	spec := writeback.DefaultSpec()
	spec.Freq = b.freq
	spec.Log2BlockSize = b.log2CacheLineSize
	spec.NumWays = 16
	spec.NumSets = int((byteSize) / (uint64(spec.NumWays) * (1 << spec.Log2BlockSize)))
	spec.NumMSHREntries = 512
	spec.NumReqPerCycle = 64
	spec.DirectoryWidth = spec.NumReqPerCycle
	spec.BankWidth = spec.NumReqPerCycle
	spec.BankLatency = b.l2BankLatency
	spec.DirLatency = 2
	spec.MaxInFlightFetches = 512
	spec.MaxInFlightEvictions = 512

	for i := 0; i < b.numMemoryBank; i++ {
		cacheName := fmt.Sprintf("%s.L2Cache[%d]", b.name, i)
		// L2 bank i misses fetch from MALL bank i (which in turn backs DRAM
		// controller i), inserting the Infinity Cache between L2 and HBM.
		spec.Bottom = mem.AddressMapper{Destinations: []messaging.RemotePort{b.malls[i].Ports.Top.AsRemote()}}
		l2 := modeling.MakeBuilder[*writeback.Comp](spec).WithSimulation(b.simulation).
			WithResources(writeback.Resources{
				Storage: mem.MakeStorageBuilder().
					WithSimulation(b.simulation).
					WithCapacity(spec.ByteSize()).
					Build(cacheName + ".Storage"),
			}).
			Build(cacheName)

		b.buildPort(l2, "Top", l2CachePortBufSize)
		b.buildPort(l2, "Bottom", l2CachePortBufSize)
		b.buildPort(l2, "Control", l2CachePortBufSize)

		b.l2Caches = append(b.l2Caches, l2)

		b.l1AddressMapper.Destinations = append(
			b.l1AddressMapper.Destinations,
			l2.Ports.Top.AsRemote(),
		)
	}
}

// buildMALL builds the 256 MB MALL (Memory-Attached Last-Level cache, a.k.a.
// Infinity Cache) that MI300X places on the IODs between the per-XCD L2 caches
// and HBM. It is banked the same way as the L2 (one bank per memory channel,
// each backing its own DRAM controller), so the banks together form a single
// device-wide last-level cache striped across the channels.
//
// The MALL is modeled as a read- and write-allocating write-through cache
// rather than a write-back cache. As a memory-side cache it fronts DRAM for
// every memory access (GPU L2 misses AND host DMA), so write-through (write
// allocates the line and also writes DRAM) keeps DRAM authoritative while
// letting host copies populate the MALL — a key behavior for the DRAM-proxy
// model. Write-through also means the MALL never holds dirty data, so it needs
// no flush and stays out of the CP control list; this avoids the deadlock that
// stacking two write-back caches (L2 over MALL) and flushing both at every
// kernel boundary would cause (the L2 writes back into a MALL that is itself
// flushing), and the O(256 MB) write-back scan per kernel.
func (b *Builder) buildMALL() {
	byteSize := b.mallCacheSize / uint64(b.numMemoryBank)

	spec := writethroughcache.DefaultSpec()
	spec.Freq = b.freq
	spec.WritePolicy = "write-through"
	spec.Log2BlockSize = b.log2CacheLineSize
	spec.NumWays = 16
	spec.NumSets = int((byteSize) / (uint64(spec.NumWays) * (1 << spec.Log2BlockSize)))
	spec.NumMSHREntries = 512
	spec.NumReqPerCycle = 64
	spec.DirectoryWidth = spec.NumReqPerCycle
	spec.BankWidth = spec.NumReqPerCycle
	spec.NumBanks = 16
	spec.BankLatency = b.mallBankLatency
	spec.DirLatency = 2
	spec.MaxInFlightRequests = 512

	for i := 0; i < b.numMemoryBank; i++ {
		cacheName := fmt.Sprintf("%s.MALL[%d]", b.name, i)
		spec.Bottom = mem.AddressMapper{Destinations: []messaging.RemotePort{b.drams[i].Ports.Top.AsRemote()}}
		mall := modeling.MakeBuilder[*writethroughcache.Comp](spec).WithSimulation(b.simulation).
			WithResources(writethroughcache.Resources{
				Storage: mem.MakeStorageBuilder().
					WithSimulation(b.simulation).
					WithCapacity(spec.ByteSize()).
					Build(cacheName + ".Storage"),
			}).
			Build(cacheName)

		b.buildPort(mall, "Top", l2CachePortBufSize)
		b.buildPort(mall, "Bottom", l2CachePortBufSize)
		b.buildPort(mall, "Control", l2CachePortBufSize)

		b.malls = append(b.malls, mall)
	}
}

func (b *Builder) buildDRAMControllers() {
	b.dmaLocalDataSource = &mem.AddressMapper{InterleavingSize: 1 << b.log2MemoryBankInterleavingSize}

	for i := 0; i < b.numMemoryBank; i++ {
		dramName := fmt.Sprintf("%s.DRAM[%d]", b.name, i)

		// Storage is global: all 16 DRAM controllers share one mem.Storage and
		// read/write it at the request's address (no storage conversion).
		//
		// The L2->DRAM mapper interleaves addresses across the 16 controllers
		// at log2MemoryBankInterleavingSize (128 B), so each controller sees a
		// strided address. The bank selector is a contiguous-bit modulo, so to
		// stripe finely across all 16 banks we first strip the inter-controller
		// interleave with the bank-selection conversion (BankAddrConv*);
		// BankSelectorLog2InterleaveSize=6 then stripes the resulting
		// controller-local address at 64 B granularity. The conversion affects
		// bank selection only — storage stays global.
		spec := simplebankedmemory.DefaultSpec()
		spec.Freq = 1 * timing.GHz
		spec.NumBanks = 16
		spec.BankPipelineWidth = 1
		spec.BankPipelineDepth = 5
		spec.StageLatency = 1
		spec.BankSelectorLog2InterleaveSize = 6
		spec.PostPipelineBufSize = 128
		spec.BankAddressConversion.Kind = "interleaving"
		spec.BankAddressConversion.InterleavingSize = 1 << b.log2MemoryBankInterleavingSize
		spec.BankAddressConversion.NumElements = b.numMemoryBank
		spec.BankAddressConversion.ElementIndex = i

		dramComp := modeling.MakeBuilder[*simplebankedmemory.Comp](spec).WithSimulation(b.simulation).
			WithResources(simplebankedmemory.Resources{
				Storage: b.globalStorage,
			}).
			Build(dramName)

		b.buildPort(dramComp, "Top", dramTopPortBufSize)
		b.buildPort(dramComp, "Control", ctrlPortBufSize)

		b.drams = append(b.drams, dramComp)
	}
}

func (b *Builder) buildRDMAEngine() {
	name := fmt.Sprintf("%s.RDMA", b.name)

	spec := rdma.DefaultSpec()
	spec.Freq = 1 * timing.GHz

	b.rdmaEngine = rdma.MakeBuilder().
		WithSimulation(b.simulation).
		WithSpec(spec).
		WithResources(rdma.Resources{
			LocalModules:           b.l1AddressMapper,
			RemoteRDMAAddressTable: b.rdmaAddressMapper,
		}).
		Build(name)

	b.buildPort(b.rdmaEngine, "RDMARequestInside", rdmaPortBufSize)
	b.buildPort(b.rdmaEngine, "RDMARequestOutside", rdmaPortBufSize)
	b.buildPort(b.rdmaEngine, "RDMADataInside", rdmaPortBufSize)
	b.buildPort(b.rdmaEngine, "RDMADataOutside", rdmaPortBufSize)
	b.buildPort(b.rdmaEngine, "Ctrl", rdmaPortBufSize)

	b.l1AddressMapper.Fallback =
		b.rdmaEngine.Ports.RDMARequestInside.AsRemote()
}

func (b *Builder) buildDMAEngine() {
	b.dmaEngine = cp.MakeDMAEngineBuilder().
		WithSimulation(b.simulation).
		WithSpec(cp.DefaultDMASpec()).
		WithResources(cp.DMAResources{
			LocalDataSource: b.dmaLocalDataSource,
		}).
		Build(fmt.Sprintf("%s.DMA", b.name))

	b.buildPort(b.dmaEngine, "ToCP", dmaToCPBufSize)
	b.buildPort(b.dmaEngine, "ToMem", dmaToMemBufSize)
}

func (b *Builder) buildCP() {
	spec := cp.DefaultSpec()
	spec.Freq = b.freq
	// No fixed kernel launch/teardown overhead is modeled. The empty_kernel floor
	// (~8.66 us at 1 WG) is launch_sync_us -- a host round-trip (CPU launch + API +
	// device sync) that is NOT GPU time. Modeling it as a fixed GPU launch latency
	// would fake host cost as device cost; the calibration metric anchors the floor
	// out (error is measured relative to the smallest-grid point), so only the
	// per-WG dispatch slope matters and it is unaffected. Leave all three overheads
	// at zero unless/until a GPU-side launch measurement exists.
	spec.ConstantKernelLaunchOverhead = 0
	spec.SubsequentKernelLaunchOverhead = 0
	spec.ConstantKernelOverhead = 0

	// Per-die (per-XCD) dispatch: a kernel's work-groups are split into NumXCD
	// contiguous blocks (one per die) and dispatched across the 8 XCDs in
	// parallel, each XCD dispatching 1 wavefront / 2 cycles. For a 256-thread
	// (4-wavefront) WG that is 8 cycles/WG per die, so 8 dies aggregate to ~1
	// WG/cycle = ~0.48 ns/WG at 2.1 GHz, matching the empty_kernel sweep slope.
	spec.Alg = "per-die"
	spec.NumDies = NumXCD
	spec.WavefrontDispatchCycles = 2

	b.cp = cp.MakeBuilder().WithSampler(b.sampler).
		WithSimulation(b.simulation).
		WithSpec(spec).
		WithMonitor(b.monitor).
		WithDriver(b.driverPort).
		Build(b.name + ".CommandProcessor")

	b.buildPort(b.cp, "ToDriver", cpPortBufSize)
	b.buildPort(b.cp, "ToDMA", cpPortBufSize)
	b.buildPort(b.cp, "ToCUs", cpPortBufSize)
	b.buildPort(b.cp, "ToTLBs", cpPortBufSize)
	b.buildPort(b.cp, "ToAddressTranslators", cpPortBufSize)
	b.buildPort(b.cp, "ToCaches", cpPortBufSize)
	b.buildPort(b.cp, "ToRDMA", cpPortBufSize)

	b.buildDMAEngine()
	b.buildRDMAEngine()
}

func (b *Builder) buildL2TLB() {
	numWays := 64

	spec := tlb.DefaultSpec()
	spec.Freq = b.freq
	spec.NumWays = numWays
	spec.NumSets = 64
	spec.NumMSHREntries = 64
	spec.NumReqPerCycle = 1024
	spec.Log2PageSize = b.log2PageSize

	spec.Bottom = mem.AddressMapper{Destinations: []messaging.RemotePort{b.mmu.Ports.Top.AsRemote()}}
	l2TLB := modeling.MakeBuilder[*tlb.Comp](spec).WithSimulation(b.simulation).
		Build(fmt.Sprintf("%s.L2TLB", b.name))

	b.buildPort(l2TLB, "Top", l2TLBPortBufSize)
	b.buildPort(l2TLB, "Bottom", l2TLBPortBufSize)
	b.buildPort(l2TLB, "Control", ctrlPortBufSize)

	b.l2TLBs = append(b.l2TLBs, l2TLB)

	b.l1TLBAddressMapper.Destinations = []messaging.RemotePort{l2TLB.Ports.Top.AsRemote()}
}
