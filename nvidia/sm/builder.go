package sm

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache/writethroughcache"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"

	"github.com/sarchlab/mgpusim/v5/nvidia/smsp"
	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

const portBufSize = 4096

// SMBuilder builds SMs. Each SM contains its SMSPs and one L1 vector cache
// per SMSP.
type SMBuilder struct {
	registrar modeling.Registrar
	freq      timing.Freq

	smspsCount        uint64
	log2CacheLineSize uint64
	l1AddressMapper   mem.AddressToPortMapper

	SM2SMSPWarpIssueLatency uint64
	SMReceiveGPULatency     uint64
	SMSPResponseHandleWidth uint64
	MemResponseHandleWidth  uint64
}

// MakeBuilder creates an SMBuilder with default parameters.
func MakeBuilder() SMBuilder {
	return SMBuilder{
		freq:              1 * timing.GHz,
		smspsCount:        4,
		log2CacheLineSize: 7,
	}
}

// WithRegistrar sets the simulation that components register to.
func (b SMBuilder) WithRegistrar(r modeling.Registrar) SMBuilder {
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
func (b SMBuilder) WithL1AddressMapper(m mem.AddressToPortMapper) SMBuilder {
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
	s := &SMController{
		ID:                               name,
		threadblockWarpCount:             make(map[trace.Dim3]uint64),
		threadblockWarpCountOrigin:       make(map[trace.Dim3]uint64),
		SM2SMSPWarpIssueLatency:          b.SM2SMSPWarpIssueLatency,
		SM2SMSPWarpIssueLatencyRemaining: b.SM2SMSPWarpIssueLatency,
		SMReceiveGPULatency:              b.SMReceiveGPULatency,
		SMReceiveGPULatencyRemaining:     b.SMReceiveGPULatency,
		SMSPResponseHandleWidth:          b.SMSPResponseHandleWidth,
	}
	s.TickingComponent = modeling.NewTickingComponent(
		name, b.registrar.GetEngine(), b.freq, s)
	b.registrar.RegisterComponent(s)

	s.DeclarePort("ToGPU")
	s.DeclarePort("ToSMSPs")
	s.toGPU = b.buildPort(s, "ToGPU", portBufSize)
	s.toSMSPs = b.buildPort(s, "ToSMSPs", portBufSize)

	b.buildSMSPs(s)
	b.buildL1VCaches(s)
	b.connectSMSPs(s)

	return s
}

func (b SMBuilder) buildPort(
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

func (b SMBuilder) buildSMSPs(s *SMController) {
	smspBuilder := new(smsp.SMSPBuilder).
		WithRegistrar(b.registrar).
		WithFreq(b.freq).
		WithLog2CacheLineSize(b.log2CacheLineSize).
		WithMemResponseHandleWidth(b.MemResponseHandleWidth)

	for i := range b.smspsCount {
		sp := smspBuilder.Build(fmt.Sprintf("%s.SMSP[%d]", s.Name(), i))
		sp.SetSMRemotePort(s.toSMSPs.AsRemote())
		s.SMSPs = append(s.SMSPs, sp)
	}
}

func (b SMBuilder) buildL1VCaches(s *SMController) {
	spec := writethroughcache.DefaultSpec()
	spec.Freq = b.freq
	spec.WritePolicyType = "write-around"
	spec.BankLatency = 27
	spec.NumBanks = 1
	spec.Log2BlockSize = b.log2CacheLineSize
	spec.WayAssociativity = 4
	spec.NumMSHREntry = 16
	spec.NumReqPerCycle = 4
	spec.MaxNumConcurrentTrans = 64
	spec.TotalByteSize = 64 * mem.KB

	for i := range b.smspsCount {
		cache := writethroughcache.MakeBuilder().
			WithRegistrar(b.registrar).
			WithSpec(spec).
			WithResources(writethroughcache.Resources{
				AddressMapper: b.l1AddressMapper,
			}).
			Build(fmt.Sprintf("%s.L1VCache[%d]", s.Name(), i))

		b.buildPort(cache, "Top", spec.NumReqPerCycle)
		b.buildPort(cache, "Bottom", spec.NumReqPerCycle)
		b.buildPort(cache, "Control", spec.NumReqPerCycle)

		s.L1VCaches = append(s.L1VCaches, cache)
	}
}

func (b SMBuilder) connectSMSPs(s *SMController) {
	smToSMSPs := directconnection.MakeBuilder().
		WithRegistrar(b.registrar).
		WithSpec(directconnection.Spec{Freq: b.freq}).
		Build(s.Name() + ".SMToSMSPs")
	smToSMSPs.PlugIn(s.toSMSPs)

	for i, sp := range s.SMSPs {
		smToSMSPs.PlugIn(sp.GetPortByName("ToSM"))

		l1Top := s.L1VCaches[i].GetPortByName("Top")
		sp.SetVectorMemRemote(l1Top.AsRemote())

		conn := directconnection.MakeBuilder().
			WithRegistrar(b.registrar).
			WithSpec(directconnection.Spec{Freq: b.freq}).
			Build(fmt.Sprintf("%s.SMSPToL1V[%d]", s.Name(), i))
		conn.PlugIn(sp.GetPortByName("ToVectorMem"))
		conn.PlugIn(l1Top)
	}
}
