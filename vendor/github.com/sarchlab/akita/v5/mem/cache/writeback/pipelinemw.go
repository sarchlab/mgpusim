package writeback

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
)

// pipelineMW holds all non-serializable infrastructure for the writeback
// cache pipeline. It implements the Tick method and delegates NamedHookable
// to comp. All mutable state is in comp.State.
type pipelineMW struct {
	comp *modeling.Component[Spec, State, Resources]

	storage *mem.Storage

	topParser   *topParser
	writeBuffer *writeBufferStage
	dirStage    *directoryStage
	bankStages  []*bankStage
	mshrStage   *mshrStage
}

// GetSpec returns the immutable specification.
func (m *pipelineMW) GetSpec() Spec {
	return m.comp.Spec()
}

// topPort resolves the "Top" port by name. The port instance is assigned
// externally after Build, so it is resolved lazily on every use rather than
// cached at build time.
func (m *pipelineMW) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

// bottomPort resolves the "Bottom" port by name, lazily, for the same reason
// as topPort.
func (m *pipelineMW) bottomPort() messaging.Port {
	return m.comp.GetPortByName("Bottom")
}

// findPort resolves an address to a remote port using data from Spec.
func (m *pipelineMW) findPort(address uint64) messaging.RemotePort {
	spec := m.comp.Spec()

	switch spec.AddressMapperType {
	case "single":
		if len(spec.RemotePortNames) > 0 {
			name := spec.RemotePortNames[0]
			if name != "" {
				return messaging.RemotePort(name)
			}
		}
	case "interleaved":
		if n := uint64(len(spec.RemotePortNames)); n > 0 {
			idx := address / spec.InterleavingSize % n
			name := spec.RemotePortNames[idx]
			if name != "" {
				return messaging.RemotePort(name)
			}
		}
	}

	panic("findPort: no valid address mapping for address; " +
		"Spec.AddressMapperType=" + spec.AddressMapperType)
}

// Tick updates the internal states of the Cache pipeline.
func (m *pipelineMW) Tick() bool {
	next := &m.comp.State
	madeProgress := false

	if cacheState(next.CacheState) != cacheStatePaused {
		madeProgress = m.runPipeline() || madeProgress
	}

	return madeProgress
}

func (m *pipelineMW) runPipeline() bool {
	madeProgress := false

	spec := m.comp.Spec()

	madeProgress = m.runStage(m.mshrStage, spec.NumReqPerCycle) || madeProgress

	for _, bs := range m.bankStages {
		madeProgress = bs.Tick() || madeProgress
	}

	madeProgress = m.runStage(m.writeBuffer, spec.NumReqPerCycle) || madeProgress
	madeProgress = m.runStage(m.dirStage, spec.NumReqPerCycle) || madeProgress
	madeProgress = m.runStage(m.topParser, spec.NumReqPerCycle) || madeProgress

	return madeProgress
}

func (m *pipelineMW) runStage(stage modeling.Ticker, n int) bool {
	madeProgress := false
	for range n {
		madeProgress = stage.Tick() || madeProgress
	}

	return madeProgress
}
