package gpu

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"

	"github.com/sarchlab/mgpusim/v5/nvidia/sm"
)

const (
	portBufSize    = 4096
	memPortBufSize = 32

	// The trace carries the virtual addresses of the traced program, which
	// can be anywhere in the 47-bit user address space. The backing storage
	// is allocated lazily, so a large capacity costs nothing.
	storageCapacity = 1 << 48
)

// Config describes the GPU to build.
type Config struct {
	NumSMs            uint64
	NumSMSPsPerSM     uint64
	L2CacheSize       uint64
	NumMemoryBanks    int
	Log2CacheLineSize uint64
	L2BankLatency     int
	DRAMLatency       int
	SMThreadCapacity  uint64
	MaxCTAPerSM       uint64

	GPU2SMThreadBlockAllocationLatency uint64
	SMReceiveGPULatency                uint64
	SM2SMSPWarpIssueLatency            uint64
	GPUReceiveSMLatency                uint64
	GPUReceiveCTALatencyUnit           float64
	CWDIssueWidth                      uint64
	SMResponseHandleWidth              uint64
	SMSPResponseHandleWidth            uint64
	MemResponseHandleWidth             uint64
}

// Builder builds a GPU: a GPU controller, its SMs, the L2 cache banks, and one
// DRAM controller per L2 bank.
type Builder struct {
	registrar modeling.Registrar
	freq      timing.Freq
	cfg       Config
}

// MakeBuilder creates a GPU builder.
func MakeBuilder() Builder {
	return Builder{freq: 1 * timing.GHz}
}

// WithRegistrar sets the simulation that components register to.
func (b Builder) WithRegistrar(r modeling.Registrar) Builder {
	b.registrar = r
	return b
}

// WithFreq sets the frequency of all the components in the GPU.
func (b Builder) WithFreq(freq timing.Freq) Builder {
	b.freq = freq
	return b
}

// WithConfig sets the GPU configuration.
func (b Builder) WithConfig(cfg Config) Builder {
	b.cfg = cfg
	return b
}

// Build creates a GPU with the given name. The GPU controller exposes a
// "ToDriver" port.
func (b Builder) Build(name string) *GPUController {
	g := b.buildController(name)

	drams := b.buildDRAMs(name)
	l2s := b.buildL2Caches(name, drams)

	// The L1 caches copy the L2 port list when they are built, so the L2
	// caches must be built first.
	l1AddressMapper := mem.NewInterleavedAddressPortMapper(
		1 << b.cfg.Log2CacheLineSize)
	for _, l2 := range l2s {
		l1AddressMapper.LowModules = append(l1AddressMapper.LowModules,
			l2.GetPortByName("Top").AsRemote())
	}

	b.buildSMs(g, l1AddressMapper)

	b.connectGPUAndSMs(g)
	b.connectL1ToL2(g, l2s)
	b.connectL2ToDRAM(name, l2s, drams)

	return g
}

func (b Builder) buildController(name string) *GPUController {
	g := &GPUController{
		ID:                                 name,
		SMAssignedThreadTable:              make(map[string]uint64),
		SMAssignedCTACountTable:            make(map[string]uint64),
		SMThreadCapacity:                   b.cfg.SMThreadCapacity,
		GPU2SMThreadBlockAllocationLatency: b.cfg.GPU2SMThreadBlockAllocationLatency,
		GPU2SMThreadBlockAllocationLatencyRemaining: b.cfg.
			GPU2SMThreadBlockAllocationLatency,
		GPUReceiveSMLatency:          b.cfg.GPUReceiveSMLatency,
		GPUReceiveSMLatencyRemaining: b.cfg.GPUReceiveSMLatency,
		GPUReceiveCTALatencyUnit:     b.cfg.GPUReceiveCTALatencyUnit,
		CWDIssueWidth:                b.cfg.CWDIssueWidth,
		SMResponseHandleWidth:        b.cfg.SMResponseHandleWidth,
		MaxCTAPerSM:                  max(b.cfg.MaxCTAPerSM, 1),
	}
	g.TickingComponent = modeling.NewTickingComponent(
		name, b.registrar.GetEngine(), b.freq, g)
	b.registrar.RegisterComponent(g)

	g.DeclarePort("ToDriver")
	g.DeclarePort("ToSMs")
	g.toDriver = b.buildPort(g, "ToDriver", portBufSize)
	g.toSMs = b.buildPort(g, "ToSMs", portBufSize)

	return g
}

func (b Builder) buildPort(
	comp messaging.Component,
	name string,
	bufSize int,
) messaging.Port {
	port := modeling.MakePortBuilder().
		WithRegistrar(b.registrar).
		WithComponent(comp).
		WithSpec(modeling.PortSpec{BufSize: bufSize}).
		Build(name)
	comp.AssignPort(name, port)

	return port
}

func (b Builder) buildSMs(
	g *GPUController,
	l1AddressMapper mem.AddressToPortMapper,
) {
	smBuilder := sm.MakeBuilder().
		WithRegistrar(b.registrar).
		WithFreq(b.freq).
		WithSMSPsCount(b.cfg.NumSMSPsPerSM).
		WithLog2CacheLineSize(b.cfg.Log2CacheLineSize).
		WithL1AddressMapper(l1AddressMapper).
		WithSM2SMSPWarpIssueLatency(b.cfg.SM2SMSPWarpIssueLatency).
		WithSMReceiveGPULatency(b.cfg.SMReceiveGPULatency).
		WithSMSPResponseHandleWidth(b.cfg.SMSPResponseHandleWidth).
		WithMemResponseHandleWidth(b.cfg.MemResponseHandleWidth)

	for i := range b.cfg.NumSMs {
		s := smBuilder.Build(fmt.Sprintf("%s.SM[%d]", g.Name(), i))
		s.SetGPURemotePort(g.toSMs.AsRemote())
		g.SMList = append(g.SMList, s)
	}
}

func (b Builder) buildDRAMs(gpuName string) []*idealmemcontroller.Comp {
	storage := mem.MakeStorageBuilder().
		WithCapacity(storageCapacity).
		WithSimulation(b.registrar).
		Build(gpuName + ".Storage")

	spec := idealmemcontroller.DefaultSpec()
	spec.Freq = b.freq
	spec.Latency = b.cfg.DRAMLatency
	spec.Capacity = storageCapacity

	drams := make([]*idealmemcontroller.Comp, 0, b.cfg.NumMemoryBanks)

	for i := range b.cfg.NumMemoryBanks {
		dram := idealmemcontroller.MakeBuilder().
			WithRegistrar(b.registrar).
			WithSpec(spec).
			WithResources(idealmemcontroller.Resources{Storage: storage}).
			Build(fmt.Sprintf("%s.DRAM[%d]", gpuName, i))

		b.buildPort(dram, "Top", memPortBufSize)
		b.buildPort(dram, "Control", memPortBufSize)

		drams = append(drams, dram)
	}

	return drams
}

func (b Builder) buildL2Caches(
	gpuName string,
	drams []*idealmemcontroller.Comp,
) []*writeback.Comp {
	spec := writeback.DefaultSpec()
	spec.Freq = b.freq
	spec.BankLatency = b.cfg.L2BankLatency
	spec.Log2BlockSize = b.cfg.Log2CacheLineSize
	spec.WayAssociativity = 16
	spec.TotalByteSize = b.cfg.L2CacheSize / uint64(b.cfg.NumMemoryBanks)
	spec.NumMSHREntry = 64
	spec.NumReqPerCycle = 16

	l2s := make([]*writeback.Comp, 0, len(drams))

	for i, dram := range drams {
		l2 := writeback.MakeBuilder().
			WithRegistrar(b.registrar).
			WithSpec(spec).
			WithResources(writeback.Resources{
				AddressToPortMapper: &mem.SinglePortMapper{
					Port: dram.GetPortByName("Top").AsRemote(),
				},
			}).
			Build(fmt.Sprintf("%s.L2Cache[%d]", gpuName, i))

		b.buildPort(l2, "Top", memPortBufSize)
		b.buildPort(l2, "Bottom", memPortBufSize)
		b.buildPort(l2, "Control", memPortBufSize)

		l2s = append(l2s, l2)
	}

	return l2s
}

func (b Builder) newConnection(name string) *directconnection.Comp {
	return directconnection.MakeBuilder().
		WithRegistrar(b.registrar).
		WithSpec(directconnection.Spec{Freq: b.freq}).
		Build(name)
}

func (b Builder) connectGPUAndSMs(g *GPUController) {
	conn := b.newConnection(g.Name() + ".GPUToSMs")
	conn.PlugIn(g.toSMs)

	for _, s := range g.SMList {
		conn.PlugIn(s.GetPortByName("ToGPU"))
	}
}

func (b Builder) connectL1ToL2(g *GPUController, l2s []*writeback.Comp) {
	conn := b.newConnection(g.Name() + ".L1ToL2")

	for _, l2 := range l2s {
		conn.PlugIn(l2.GetPortByName("Top"))
	}

	for _, s := range g.SMList {
		for _, l1 := range s.L1VCaches {
			conn.PlugIn(l1.GetPortByName("Bottom"))
		}
	}
}

func (b Builder) connectL2ToDRAM(
	gpuName string,
	l2s []*writeback.Comp,
	drams []*idealmemcontroller.Comp,
) {
	conn := b.newConnection(gpuName + ".L2ToDRAM")

	for _, l2 := range l2s {
		conn.PlugIn(l2.GetPortByName("Bottom"))
	}

	for _, dram := range drams {
		conn.PlugIn(dram.GetPortByName("Top"))
	}
}
