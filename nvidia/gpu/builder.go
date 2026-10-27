package gpu

import (
	"fmt"
	"github.com/sarchlab/mgpusim/v5/nvidia/trace"

	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/sim/messaging"
	directconnection "github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"

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

// DefaultSpec returns the default component configuration.
func DefaultSpec() Spec {
	return Spec{Freq: 1 * timing.GHz, MaxCTAPerSM: 1}
}

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
	traces    *trace.Store
	toDriver  messaging.RemotePort
	registrar modeling.Simulation
	freq      timing.Freq
	cfg       Config
}

// MakeBuilder creates a GPU builder.
func MakeBuilder() Builder {
	return Builder{freq: DefaultSpec().Freq}
}

// WithSimulation sets the simulation that components register to.
func (b Builder) WithSimulation(r modeling.Simulation) Builder {
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

// WithDriverRemotePort sets the immutable parent destination.
func (b Builder) WithDriverRemotePort(remote messaging.RemotePort) Builder {
	b.toDriver = remote
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
	l1AddressMapper := mem.AddressMapper{InterleavingSize: 1 << b.cfg.Log2CacheLineSize}
	for _, l2 := range l2s {
		l1AddressMapper.Destinations = append(l1AddressMapper.Destinations,
			l2.Ports.Top.AsRemote())
	}

	b.buildSMs(g, l1AddressMapper)

	b.connectGPUAndSMs(g)
	b.connectL1ToL2(g, l2s)
	b.connectL2ToDRAM(name, l2s, drams)

	return g
}

func (b Builder) buildController(name string) *GPUController {
	spec := Spec{
		ToDriver:                           b.toDriver,
		Freq:                               b.freq,
		SMThreadCapacity:                   b.cfg.SMThreadCapacity,
		GPU2SMThreadBlockAllocationLatency: b.cfg.GPU2SMThreadBlockAllocationLatency,
		GPUReceiveSMLatency:                b.cfg.GPUReceiveSMLatency,
		GPUReceiveCTALatencyUnit:           b.cfg.GPUReceiveCTALatencyUnit,
		CWDIssueWidth:                      b.cfg.CWDIssueWidth,
		SMResponseHandleWidth:              b.cfg.SMResponseHandleWidth,
		MaxCTAPerSM:                        max(b.cfg.MaxCTAPerSM, 1),
	}
	for i := range b.cfg.NumSMs {
		child := fmt.Sprintf("%s.SM[%d]", name, i)
		spec.SMs = append(spec.SMs, SMPeer{Name: child, Port: messaging.RemotePort(child + ".ToGPU")})
	}
	c := modeling.MakeBuilder[*Comp](spec).WithResources(Resources{Traces: b.traces}).
		WithSimulation(b.registrar).Build(name)
	g := &GPUController{Comp: c}
	b.buildPort(g, "ToDriver", portBufSize)
	b.buildPort(g, "ToSMs", portBufSize)

	return g
}

func (b Builder) buildPort(
	comp interface{ BindPort(string, messaging.Port) },
	name string,
	bufSize int,
) messaging.Port {
	port := twowaybuffered.MakeBuilder().
		WithSimulation(b.registrar).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: bufSize, OutgoingCapacity: bufSize}).
		Build()
	comp.BindPort(name, port)
	return port
}

func (b Builder) buildSMs(
	g *GPUController,
	l1AddressMapper mem.AddressMapper,
) {
	smBuilder := sm.MakeBuilder().WithTraceStore(b.traces).
		WithSimulation(b.registrar).
		WithFreq(b.freq).
		WithGPURemotePort(g.Ports.ToSMs.AsRemote()).
		WithSMSPsCount(b.cfg.NumSMSPsPerSM).
		WithLog2CacheLineSize(b.cfg.Log2CacheLineSize).
		WithL1AddressMapper(l1AddressMapper).
		WithSM2SMSPWarpIssueLatency(b.cfg.SM2SMSPWarpIssueLatency).
		WithSMReceiveGPULatency(b.cfg.SMReceiveGPULatency).
		WithSMSPResponseHandleWidth(b.cfg.SMSPResponseHandleWidth).
		WithMemResponseHandleWidth(b.cfg.MemResponseHandleWidth)

	for i := range b.cfg.NumSMs {
		s := smBuilder.Build(fmt.Sprintf("%s.SM[%d]", g.Name(), i))
		peer := g.Spec.SMs[i]
		if peer.Name != s.Name() || peer.Port != s.Ports.ToGPU.AsRemote() ||
			s.Spec.ToGPU != g.Ports.ToSMs.AsRemote() {
			panic("built SM does not match GPU peer configuration")
		}
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

	drams := make([]*idealmemcontroller.Comp, 0, b.cfg.NumMemoryBanks)

	for i := range b.cfg.NumMemoryBanks {
		dram := modeling.MakeBuilder[*idealmemcontroller.Comp](spec).
			WithSimulation(b.registrar).
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
	spec.NumWays = 16
	spec.NumSets = int(b.cfg.L2CacheSize / uint64(b.cfg.NumMemoryBanks) / (uint64(spec.NumWays) << spec.Log2BlockSize))
	spec.NumMSHREntries = 64
	spec.NumReqPerCycle = 16
	spec.DirectoryWidth = 16
	spec.BankWidth = 16

	l2s := make([]*writeback.Comp, 0, len(drams))

	for i, dram := range drams {
		spec.Bottom = mem.AddressMapper{Destinations: []messaging.RemotePort{dram.Ports.Top.AsRemote()}}
		storage := mem.MakeStorageBuilder().
			WithSimulation(b.registrar).
			WithCapacity(spec.ByteSize()).
			Build(fmt.Sprintf("%s.L2Cache[%d].Storage", gpuName, i))
		l2 := modeling.MakeBuilder[*writeback.Comp](spec).WithResources(writeback.Resources{Storage: storage}).
			WithSimulation(b.registrar).
			Build(fmt.Sprintf("%s.L2Cache[%d]", gpuName, i))

		b.buildPort(l2, "Top", memPortBufSize)
		b.buildPort(l2, "Bottom", memPortBufSize)
		b.buildPort(l2, "Control", memPortBufSize)

		l2s = append(l2s, l2)
	}

	return l2s
}

func (b Builder) newConnection(name string) *directconnection.Connection {
	return directconnection.NewConnection(name, b.registrar, b.freq)
}

func (b Builder) connectGPUAndSMs(g *GPUController) {
	conn := b.newConnection(g.Name() + ".GPUToSMs")
	conn.BindPort(g.Ports.ToSMs)

	for _, s := range g.SMList {
		conn.BindPort(s.Ports.ToGPU)
	}
}

func (b Builder) connectL1ToL2(g *GPUController, l2s []*writeback.Comp) {
	conn := b.newConnection(g.Name() + ".L1ToL2")

	for _, l2 := range l2s {
		conn.BindPort(l2.Ports.Top)
	}

	for _, s := range g.SMList {
		for _, l1 := range s.L1VCaches {
			conn.BindPort(l1.Ports.Bottom)
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
		conn.BindPort(l2.Ports.Bottom)
	}

	for _, dram := range drams {
		conn.BindPort(dram.Ports.Top)
	}
}

// WithTraceStore supplies the shared checkpointed trace owner.
func (b Builder) WithTraceStore(store *trace.Store) Builder { b.traces = store; return b }
