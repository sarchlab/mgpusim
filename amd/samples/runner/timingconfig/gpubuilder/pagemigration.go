package gpubuilder

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/datamover"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/shaderarray"
)

// Page-migration copy configuration of the data mover of a GPU: it keeps up
// to dataMoverBufferBytes of a page copy in flight.
const (
	dataMoverBufferBytes = 64 << 10
	dataMoverCtrlBufSize = 4
	dataMoverMemBufSize  = 256
)

// BuildDataMover builds the data mover that copies the pages that migrate
// to a GPU. The destination pulls the data: Outside reads the source page
// from the source GPU's RDMADataOutside (rdmaMapper), which reads through
// the source L2, so dirty lines are copied; Inside writes the new frame to
// the local memory (localMapper), around the L2. The caller connects Top and
// Control to the command processor, Inside to the local memory, and Outside
// to the inter-device connection.
func BuildDataMover(
	sim modeling.Simulation,
	name string,
	freq timing.Freq,
	lineBytes uint64,
	localMapper, rdmaMapper mem.AddressMapper,
) *datamover.Comp {
	spec := datamover.DefaultSpec()
	spec.Freq = freq
	spec.BufferSize = dataMoverBufferBytes
	spec.InsideByteGranularity = lineBytes
	spec.OutsideByteGranularity = lineBytes
	spec.Inside = localMapper
	spec.Outside = rdmaMapper

	mover := modeling.MakeBuilder[*datamover.Comp](spec).
		WithSimulation(sim).
		Build(name)

	for _, p := range []struct {
		name    string
		bufSize int
	}{
		{"Top", dataMoverCtrlBufSize},
		{"Control", dataMoverCtrlBufSize},
		{"Inside", dataMoverMemBufSize},
		{"Outside", dataMoverMemBufSize},
	} {
		port := twowaybuffered.MakeBuilder().
			WithSimulation(sim).
			WithSpec(twowaybuffered.Spec{
				IncomingCapacity: p.bufSize, OutgoingCapacity: p.bufSize,
			}).
			Build()
		mover.BindPort(p.name, port)
	}

	return mover
}

// MigrationPeers returns the Control ports of every address translator
// (vector, scalar, and instruction) and every TLB of a GPU, which the
// command processor controls during page migration.
func MigrationPeers(
	sas []*shaderarray.ShaderArray,
	l2TLBs []*tlb.Comp,
) (ats, tlbs []messaging.Port) {
	for _, sa := range sas {
		for _, at := range sa.L1VATs {
			ats = append(ats, at.Ports.Control)
		}

		ats = append(ats, sa.L1SAT.Ports.Control, sa.L1IAT.Ports.Control)

		for _, t := range sa.L1VTLBs {
			tlbs = append(tlbs, t.Ports.Control)
		}

		tlbs = append(tlbs, sa.L1STLB.Ports.Control, sa.L1ITLB.Ports.Control)
	}

	for _, t := range l2TLBs {
		tlbs = append(tlbs, t.Ports.Control)
	}

	return ats, tlbs
}

// RemotePorts returns the addresses of ports.
func RemotePorts(ports []messaging.Port) []messaging.RemotePort {
	remotes := make([]messaging.RemotePort, len(ports))
	for i, p := range ports {
		remotes[i] = p.AsRemote()
	}

	return remotes
}
