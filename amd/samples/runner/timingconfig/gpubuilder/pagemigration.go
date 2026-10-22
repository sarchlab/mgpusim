package gpubuilder

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	"github.com/sarchlab/akita/v5/mem/vm/addresstranslator"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/shaderarray"
	"github.com/sarchlab/mgpusim/v5/amd/timing/pagemigration"
	"github.com/sarchlab/mgpusim/v5/amd/timing/rdma"
)

// Port buffer sizes of the page-migration engine.
const (
	pmRemotePortBufSize  = 64
	pmTLBCtrlPortBufSize = 64
)

// PageMigrationGPU is what the page-migration manager needs from a GPU.
type PageMigrationGPU struct {
	GPUID  uint64
	SAs    []*shaderarray.ShaderArray
	L2TLBs []*tlb.Comp
	L2s    []*writeback.Comp
	RDMA   *rdma.Comp

	// L2Mapper finds the L2 slice of a local address (the L1-to-L2
	// interleaving). MemMapper finds the memory-side module that the DMA
	// engine writes to.
	L2Mapper  mem.AddressToPortMapper
	MemMapper mem.AddressToPortMapper
}

// BuildPageMigrationEngine builds the page-migration engine of a GPU. It
// returns nil when the policy does not migrate (the observe policy must not
// add any port to the platform). The caller plugs the "TLBControl" port into
// the connection of the TLB Control ports, the "L2" port into the L1-to-L2
// connection, the "DRAM" port into the connection that the DMA engine writes
// to, and exports the "Remote" port with GPU.PageMigrationPort.
func BuildPageMigrationEngine(
	reg modeling.Registrar,
	m *pagemigration.Manager,
	name string,
	g PageMigrationGPU,
) *pagemigration.Engine {
	if m == nil || !m.Spec().Migrates() {
		return nil
	}

	spec := m.Spec().EngineSpec()
	for _, t := range allTLBs(g) {
		spec.TLBs = append(spec.TLBs,
			t.GetPortByName("Control").AsRemote())
	}

	e := pagemigration.MakeEngineBuilder().
		WithRegistrar(reg).
		WithSpec(spec).
		WithResources(pagemigration.EngineResources{
			L2Mapper:   g.L2Mapper,
			DRAMMapper: g.MemMapper,
		}).
		Build(name)

	memBufSize := 2 * spec.MemWindow
	ports := []struct {
		name    string
		bufSize int
	}{
		{"Remote", pmRemotePortBufSize},
		{"L2", memBufSize},
		{"DRAM", memBufSize},
		{"TLBControl", pmTLBCtrlPortBufSize},
	}

	for _, p := range ports {
		port := modeling.MakePortBuilder().
			WithRegistrar(reg).
			WithComponent(e).
			WithSpec(modeling.PortSpec{BufSize: p.bufSize}).
			Build(p.name)
		e.AssignPort(p.name, port)
	}

	return e
}

// RegisterPageMigrationGPU registers a built GPU with the manager and hooks
// its RDMA engine for the access counters and the statistics.
func RegisterPageMigrationGPU(
	m *pagemigration.Manager,
	g PageMigrationGPU,
	e *pagemigration.Engine,
) {
	if m == nil {
		return
	}

	var ats []*addresstranslator.Comp

	for _, sa := range g.SAs {
		ats = append(ats, sa.L1VATs...)
		ats = append(ats, sa.L1SAT, sa.L1IAT)
	}

	m.RegisterGPU(pagemigration.GPU{
		DeviceID:           g.GPUID,
		Engine:             e,
		AddressTranslators: ats,
		HoldsLine:          l2HoldsLine(g.L2Mapper, g.L2s),
	})

	g.RDMA.GetPortByName("RDMARequestInside").
		AcceptHook(m.RemoteAccessHook(g.GPUID))
}

func allTLBs(g PageMigrationGPU) []messaging.Component {
	var tlbs []messaging.Component

	for _, sa := range g.SAs {
		for _, t := range sa.L1VTLBs {
			tlbs = append(tlbs, t)
		}

		tlbs = append(tlbs, sa.L1STLB, sa.L1ITLB)
	}

	for _, t := range g.L2TLBs {
		tlbs = append(tlbs, t)
	}

	return tlbs
}

// l2HoldsLine returns a function that reports whether the L2 slice of a
// local address holds the line or has an MSHR entry for it.
func l2HoldsLine(
	l2Mapper mem.AddressToPortMapper,
	l2s []*writeback.Comp,
) func(uint64) bool {
	byPort := make(map[messaging.RemotePort]*writeback.Comp, len(l2s))
	for _, l2 := range l2s {
		byPort[l2.GetPortByName("Top").AsRemote()] = l2
	}

	return func(addr uint64) bool {
		l2, ok := byPort[l2Mapper.Find(addr)]
		if !ok {
			return false
		}

		blockSize := uint64(1) << l2.Spec().Log2BlockSize
		line := addr &^ (blockSize - 1)
		dir := &l2.State.DirectoryState

		if len(dir.Sets) == 0 {
			return false
		}

		if _, _, found := cache.DirectoryLookup(
			dir, len(dir.Sets), int(blockSize), 0, line); found {
			return true
		}

		_, found := cache.MSHRQuery(&l2.State.MSHRState, 0, line)

		return found
	}
}
