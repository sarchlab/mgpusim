package sm

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"

	"github.com/sarchlab/akita/v5/mem"
	writethroughcache "github.com/sarchlab/akita/v5/mem/cache/writethrough"
	"github.com/sarchlab/akita/v5/sim/messaging"
	directconnection "github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"

	"github.com/sarchlab/mgpusim/v5/nvidia/smsp"
)

const portBufSize = 4096

// DefaultSpec returns the default component configuration.
func DefaultSpec() Spec {
	return Spec{Freq: 1 * timing.GHz}
}

// SMBuilder builds SMs. Each SM contains its SMSPs and one L1 vector cache
// per SMSP.
type SMBuilder struct {
	registrar modeling.Simulation
	freq      timing.Freq

	smspsCount        uint64
	log2CacheLineSize uint64
	l1AddressMapper   mem.AddressMapper

	SM2SMSPWarpIssueLatency uint64
	SMReceiveGPULatency     uint64
	SMSPResponseHandleWidth uint64
	MemResponseHandleWidth  uint64
}

// MakeBuilder creates an SMBuilder with default parameters.
func MakeBuilder() SMBuilder {
	return SMBuilder{
		freq:              DefaultSpec().Freq,
		smspsCount:        4,
		log2CacheLineSize: 7,
	}
}

// WithSimulation sets the simulation that components register to.
func (b SMBuilder) WithSimulation(r modeling.Simulation) SMBuilder {
	b.registrar = r
	return b
}

// WithFreq sets the frequency of the SM and its sub-components.
func (b SMBuilder) WithFreq(freq timing.Freq) SMBuilder {
	b.freq = freq
	return b
}

// WithSMSPsCount sets the number of SMSPs per SM.
func (b SMBuilder) WithSMSPsCount(count uint64) SMBuilder {
	b.smspsCount = count
	return b
}

// WithL1AddressMapper sets how the L1 caches find the L2 cache banks.
func (b SMBuilder) WithL1AddressMapper(m mem.AddressMapper) SMBuilder {
	b.l1AddressMapper = m
	return b
}

// WithLog2CacheLineSize sets the cache line size of the L1 caches.
func (b SMBuilder) WithLog2CacheLineSize(size uint64) SMBuilder {
	b.log2CacheLineSize = size
	return b
}

// WithSM2SMSPWarpIssueLatency sets the cycles between two warp dispatches.
func (b SMBuilder) WithSM2SMSPWarpIssueLatency(l uint64) SMBuilder {
	b.SM2SMSPWarpIssueLatency = l
	return b
}

// WithSMReceiveGPULatency sets the cycles to accept a thread block.
func (b SMBuilder) WithSMReceiveGPULatency(l uint64) SMBuilder {
	b.SMReceiveGPULatency = l
	return b
}

// WithSMSPResponseHandleWidth sets how many SMSP messages the SM handles per
// cycle.
func (b SMBuilder) WithSMSPResponseHandleWidth(w uint64) SMBuilder {
	b.SMSPResponseHandleWidth = w
	return b
}

// WithMemResponseHandleWidth sets how many memory responses each SMSP
// handles per cycle.
func (b SMBuilder) WithMemResponseHandleWidth(w uint64) SMBuilder {
	b.MemResponseHandleWidth = w
	return b
}

// Build creates an SM with the given name. The SM exposes a "ToGPU" port and
// the L1 caches expose their "Bottom" ports for the connection to L2.
func (b SMBuilder) Build(name string) *SMController {
	t := &topology{ID: name}
	spec := Spec{
		Freq:                    b.freq,
		SM2SMSPWarpIssueLatency: b.SM2SMSPWarpIssueLatency,
		SMReceiveGPULatency:     b.SMReceiveGPULatency,
		SMSPResponseHandleWidth: b.SMSPResponseHandleWidth,
	}
	c := modeling.MakeBuilder[*Comp](spec).WithSimulation(b.registrar).
		WithResources(Resources{Topology: t}).Build(name)
	s := &SMController{Comp: c, topology: t}
	b.buildPort(s, "ToGPU", portBufSize)
	b.buildPort(s, "ToSMSPs", portBufSize)

	b.buildSMSPs(s)
	b.buildL1VCaches(s)
	b.connectSMSPs(s)

	return s
}

func (b SMBuilder) buildPort(
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

func (b SMBuilder) buildSMSPs(s *SMController) {
	smspBuilder := new(smsp.SMSPBuilder).
		WithSimulation(b.registrar).
		WithFreq(b.freq).
		WithLog2CacheLineSize(b.log2CacheLineSize).
		WithMemResponseHandleWidth(b.MemResponseHandleWidth)

	for i := range b.smspsCount {
		sp := smspBuilder.Build(fmt.Sprintf("%s.SMSP[%d]", s.Name(), i))
		sp.SetSMRemotePort(s.Ports.ToSMSPs.AsRemote())
		s.SMSPs = append(s.SMSPs, sp)
	}
}

func (b SMBuilder) buildL1VCaches(s *SMController) {
	spec := writethroughcache.DefaultSpec()
	spec.Freq = b.freq
	spec.WritePolicy = "write-around"
	spec.BankLatency = 27
	spec.NumBanks = 1
	spec.Log2BlockSize = b.log2CacheLineSize
	spec.NumWays = 4
	spec.NumMSHREntries = 16
	spec.NumReqPerCycle = 4
	spec.MaxInFlightRequests = 64
	spec.NumSets = int(64 * mem.KB / (uint64(spec.NumWays) << spec.Log2BlockSize))
	spec.Bottom = b.l1AddressMapper

	for i := range b.smspsCount {
		storage := mem.MakeStorageBuilder().
			WithSimulation(b.registrar).
			WithCapacity(spec.ByteSize()).
			Build(fmt.Sprintf("%s.L1VCache[%d].Storage", s.Name(), i))
		cache := modeling.MakeBuilder[*writethroughcache.Comp](
			spec,
		).WithResources(writethroughcache.Resources{Storage: storage}).
			WithSimulation(b.registrar).
			Build(fmt.Sprintf("%s.L1VCache[%d]", s.Name(), i))

		b.buildPort(cache, "Top", spec.NumReqPerCycle)
		b.buildPort(cache, "Bottom", spec.NumReqPerCycle)
		b.buildPort(cache, "Control", spec.NumReqPerCycle)

		s.L1VCaches = append(s.L1VCaches, cache)
	}
}

func (b SMBuilder) connectSMSPs(s *SMController) {
	smToSMSPs := directconnection.NewConnection(s.Name()+".SMToSMSPs", b.registrar, b.freq)
	smToSMSPs.BindPort(s.Ports.ToSMSPs)

	for i, sp := range s.SMSPs {
		smToSMSPs.BindPort(sp.Ports.ToSM)

		l1Top := s.L1VCaches[i].Ports.Top
		sp.SetVectorMemRemote(l1Top.AsRemote())

		conn := directconnection.NewConnection(fmt.Sprintf("%s.SMSPToL1V[%d]", s.Name(), i), b.registrar, b.freq)
		conn.BindPort(sp.Ports.ToVectorMem)
		conn.BindPort(l1Top)
	}
}
