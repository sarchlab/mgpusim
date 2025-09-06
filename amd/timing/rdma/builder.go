package rdma

import (
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

type Builder struct {
	name                   string
	engine                 sim.Engine
	freq                   sim.Freq
	localModules           mem.AddressToPortMapper
	RemoteRDMAAddressTable mem.AddressToPortMapper
	bufferSize             int

	incomingReqPerCycle int
	incomingRspPerCycle int
	outgoingReqPerCycle int
	outgoingRspPerCycle int
}

// MakeBuilder creates a new builder with default configuration values.
func MakeBuilder() Builder {
	return Builder{
		freq:                1 * sim.GHz,
		bufferSize:          128,
		incomingReqPerCycle: 1,
		incomingRspPerCycle: 1,
		outgoingReqPerCycle: 1,
		outgoingRspPerCycle: 1,
	}
}

// WithEngine sets the even-driven simulation engine to use.
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency that the Command Processor works at.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithBufferSize sets the number of transactions that the buffer can handle.
func (b Builder) WithBufferSize(n int) Builder {
	b.bufferSize = n
	return b
}

// WithLocalModules sets the local modules.
func (b Builder) WithLocalModules(m mem.AddressToPortMapper) Builder {
	b.localModules = m
	return b
}

// WithRemoteModules sets the remote modules.
func (b Builder) WithRemoteModules(m mem.AddressToPortMapper) Builder {
	b.RemoteRDMAAddressTable = m
	return b
}

func (b Builder) WithIncomingReqPerCycle(n int) Builder {
	b.incomingReqPerCycle = n
	return b
}

func (b Builder) WithIncomingRspPerCycle(n int) Builder {
	b.incomingRspPerCycle = n
	return b
}

func (b Builder) WithOutgoingReqPerCycle(n int) Builder {
	b.outgoingReqPerCycle = n
	return b
}

func (b Builder) WithOutgoingRspPerCycle(n int) Builder {
	b.outgoingRspPerCycle = n
	return b
}

// Build creates a RDMA with the given parameters.
func (b Builder) Build(name string) *Comp {
	rdma := &Comp{}

	rdma.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, rdma)

	rdma.localModules = b.localModules
	rdma.RemoteRDMAAddressTable = b.RemoteRDMAAddressTable
	rdma.incomingReqPerCycle = b.incomingReqPerCycle
	rdma.incomingRspPerCycle = b.incomingRspPerCycle
	rdma.outgoingReqPerCycle = b.outgoingReqPerCycle
	rdma.outgoingRspPerCycle = b.outgoingRspPerCycle

	rdma.ToL1 = sim.NewPort(rdma, b.bufferSize, b.bufferSize, name+".ToL1")
	rdma.ToL2 = sim.NewPort(rdma, b.bufferSize, b.bufferSize, name+".ToL2")
	rdma.CtrlPort = sim.NewPort(rdma, b.bufferSize, b.bufferSize, name+".CtrlPort")
	rdma.L2Outside = sim.NewPort(rdma, b.bufferSize, b.bufferSize, name+".L2Outside")
	rdma.L1Outside = sim.NewPort(rdma, b.bufferSize, b.bufferSize, name+".L1Outside")

	rdma.AddPort("ToL1", rdma.ToL1)
	rdma.AddPort("ToL2", rdma.ToL2)
	rdma.AddPort("CtrlPort", rdma.CtrlPort)
	rdma.AddPort("L2Outside", rdma.L2Outside)
	rdma.AddPort("L1Outside", rdma.L1Outside)

	return rdma
}
