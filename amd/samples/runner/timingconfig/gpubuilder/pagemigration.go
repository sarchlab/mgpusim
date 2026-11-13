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
// to dataMoverBufferBytes of the page copies in flight.
const (
	dataMoverBufferBytes = 64 << 10
	dataMoverCtrlBufSize = 4
	dataMoverMemBufSize  = 256
)

// PageMigrationConfig is what a GPU builder needs to know about page
// migration: where the access counters of the RDMA engine report to, if
// anywhere, and how its data mover copies.
type PageMigrationConfig struct {
	// AccessCounterHandler receives the access-counter notifications of the
	// RDMA engine; empty turns the counters off. AccessCounterThreshold is
	// the number of remote accesses of the GPU to a physical page frame that
	// notify it.
	AccessCounterHandler   messaging.RemotePort
	AccessCounterThreshold uint64

	// DataMoverPages is the number of page copies that the data mover of the
	// GPU keeps in flight.
	DataMoverPages int

	// PeerMemory selects, for a physical address of another GPU, the port of
	// that GPU that reaches its memory below the L2 caches (the
	// MigrationOutside port of its RDMA engine).
	PeerMemory mem.AddressMapper
}

// BuildDataMover builds the data mover of a GPU, which pulls the pages that
// migrate to the GPU. It copies around the L2 caches on both ends: Outside
// reaches the memory below the L2 of the other GPUs (cfg.PeerMemory), where
// it reads, and Inside the local memory below the L2 (localMemory), where it
// writes. The caller connects Top and Control to the command processor,
// Inside to the connection of the local memory, and Outside to the
// inter-device connection.
func BuildDataMover(
	sim modeling.Simulation,
	gpuName string,
	freq timing.Freq,
	lineBytes uint64,
	localMemory mem.AddressMapper,
	cfg PageMigrationConfig,
) *datamover.Comp {
	spec := datamover.DefaultSpec()
	spec.Freq = freq
	spec.MaxTransactions = max(1, cfg.DataMoverPages)
	spec.BufferSize = dataMoverBufferBytes
	spec.InsideByteGranularity = lineBytes
	spec.OutsideByteGranularity = lineBytes
	spec.Inside = localMemory
	spec.Outside = cfg.PeerMemory

	mover := modeling.MakeBuilder[*datamover.Comp](spec).
		WithSimulation(sim).
		Build(gpuName + ".DataMover")

	for _, p := range []struct {
		name    string
		bufSize int
	}{
		{"Top", max(dataMoverCtrlBufSize, cfg.DataMoverPages)},
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
