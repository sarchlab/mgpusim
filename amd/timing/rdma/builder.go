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

	rdma.RDMARequestInside = sim.NewPort(rdma, b.bufferSize, b.bufferSize, name+".RDMARequestInside")
	rdma.RDMARequestOutside = sim.NewPort(rdma, b.bufferSize, b.bufferSize, name+".RDMARequestOutside")
	rdma.RDMADataInside = sim.NewPort(rdma, b.bufferSize, b.bufferSize, name+".RDMADataInside")
	rdma.RDMADataOutside = sim.NewPort(rdma, b.bufferSize, b.bufferSize, name+".RDMADataOutside")
	rdma.CtrlPort = sim.NewPort(rdma, b.bufferSize, b.bufferSize, name+".CtrlPort")

	rdma.AddPort("RDMARequestInside", rdma.RDMARequestInside)
	rdma.AddPort("RDMARequestOutside", rdma.RDMARequestOutside)
	rdma.AddPort("RDMADataOutside", rdma.RDMADataOutside)
	rdma.AddPort("RDMADataInside", rdma.RDMADataInside)
	rdma.AddPort("CtrlPort", rdma.CtrlPort)

	return rdma
}
