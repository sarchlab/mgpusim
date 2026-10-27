package cu

import (
	"math"

	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// A FetchArbiter can decide which wavefront in a scheduler can fetch
// instructions
type FetchArbiter struct {
	InstBufByteSize int
}

// Arbitrate decide which wavefront can fetch the next instruction
func (a *FetchArbiter) Arbitrate(
	wfPools []*WavefrontPool,
) []*Wavefront {
	list := make([]*Wavefront, 0, 1)

	oldestTime := timing.VTimeInPicoSec(math.MaxUint64)
	var toFetch *Wavefront
	for _, wfPool := range wfPools {
		for _, handle := range wfPool.ids() {
			wf := wfPool.cu.wave(handle)
			if !a.canFetchFromWF(wf) {
				continue
			}

			if wf.execution().LastFetchTime < oldestTime {
				toFetch = wf
				oldestTime = wf.execution().LastFetchTime
			}
		}
	}

	if toFetch != nil {
		list = append(list, toFetch)
	}

	return list
}

func (a *FetchArbiter) canFetchFromWF(wf *Wavefront) bool {
	state := wf.execution()
	if wf.state().StopFetch || state.IsFetching {
		return false
	}

	if state.State == wavefront.WfCompleted {
		return false
	}

	if len(state.InstBuffer) >= a.InstBufByteSize {
		return false
	}

	group := wf.group()
	co := &group.raw().CodeObject
	if co.Present && co.HasSymbol {
		lastPCInBinary := co.Symbol.Size +
			group.packet().KernelObject
		lastPCInInstBuffer := state.InstBufferStartPC +
			uint64(len(state.InstBuffer))
		if lastPCInInstBuffer >= lastPCInBinary {
			return false
		}
	}

	return true
}
