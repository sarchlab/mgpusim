package writeback

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// pipelineMW holds all non-serializable infrastructure for the writeback
// cache pipeline. It implements the Tick method and delegates NamedHookable
// to comp. All mutable state is in State, accessed via comp.GetNextState().
type pipelineMW struct {
	comp *modeling.Component[Spec, State]

	topPort    sim.Port
	bottomPort sim.Port

	storage *mem.Storage

	topParser   *topParser
	writeBuffer *writeBufferStage
	dirStage    *directoryStage
	bankStages  []*bankStage
	mshrStage   *mshrStage
}

// GetSpec returns the immutable specification.
func (m *pipelineMW) GetSpec() Spec {
	return m.comp.GetSpec()
}

// findPort resolves an address to a remote port using data from Spec.
func (m *pipelineMW) findPort(address uint64) sim.RemotePort {
	spec := m.comp.GetSpec()

	switch spec.AddressMapperType {
	case "single":
		if len(spec.RemotePortNames) > 0 {
			name := spec.RemotePortNames[0]
			if name != "" {
				return sim.RemotePort(name)
			}
		}
	case "interleaved":
		if n := uint64(len(spec.RemotePortNames)); n > 0 {
			idx := address / spec.InterleavingSize % n
			name := spec.RemotePortNames[idx]
			if name != "" {
				return sim.RemotePort(name)
			}
		}
	}

	panic("findPort: no valid address mapping for address; " +
		"Spec.AddressMapperType=" + spec.AddressMapperType)
}

// Tick updates the internal states of the Cache pipeline.
func (m *pipelineMW) Tick() bool {
	next := m.comp.GetNextState()
	madeProgress := false

	if cacheState(next.CacheState) != cacheStatePaused {
		madeProgress = m.runPipeline() || madeProgress
	}

	return madeProgress
}

// syncForTest synchronizes state so that GetNextState reflects prior mutations.
// In tests, state is set up via GetNextState() without going through
// Component.Tick(), so we commit and re-derive.
func (m *pipelineMW) syncForTest() {
	next := m.comp.GetNextState()
	m.comp.SetState(*next)
}

func (m *pipelineMW) runPipeline() bool {
	madeProgress := false

	spec := m.comp.GetSpec()

	madeProgress = m.runStage(m.mshrStage, spec.NumReqPerCycle) || madeProgress

	for _, bs := range m.bankStages {
		madeProgress = bs.Tick() || madeProgress
	}

	madeProgress = m.runStage(m.writeBuffer, spec.NumReqPerCycle) || madeProgress
	madeProgress = m.runStage(m.dirStage, spec.NumReqPerCycle) || madeProgress
	madeProgress = m.runStage(m.topParser, spec.NumReqPerCycle) || madeProgress

	return madeProgress
}

func (m *pipelineMW) runStage(stage sim.Ticker, n int) bool {
	madeProgress := false
	for range n {
		madeProgress = stage.Tick() || madeProgress
	}

	return madeProgress
}

func (m *pipelineMW) discardInflightTransactions() {
	next := m.comp.GetNextState()

	for i := range next.DirectoryState.Sets {
		for j := range next.DirectoryState.Sets[i].Blocks {
			next.DirectoryState.Sets[i].Blocks[j].ReadCount = 0
			next.DirectoryState.Sets[i].Blocks[j].IsLocked = false
		}
	}

	// Clear all buffers and pipelines
	next.DirStageBuf.Clear()
	next.DirPipeline.Stages = nil
	next.DirPostPipelineBuf.Clear()
	for i := range next.DirToBankBufs {
		next.DirToBankBufs[i].Clear()
	}
	for i := range next.WriteBufferToBankBufs {
		next.WriteBufferToBankBufs[i].Clear()
	}
	for i := range next.BankPipelines {
		next.BankPipelines[i].Stages = nil
	}
	for i := range next.BankPostPipelineBufs {
		next.BankPostPipelineBufs[i].Clear()
	}
	next.MSHRStageBuf.Clear()
	next.WriteBufferBuf.Clear()

	for i := range next.BankInflightTransCounts {
		next.BankInflightTransCounts[i] = 0
		next.BankDownwardInflightTransCounts[i] = 0
	}

	// Clear MSHR stage state
	next.HasProcessingMSHREntry = false
	next.ProcessingMSHREntryIdx = 0

	// Clear write buffer stage state
	next.PendingEvictionIndices = nil
	next.InflightFetchIndices = nil
	next.InflightEvictionIndices = nil

	clearPort(m.topPort)

	next.Transactions = nil
}
