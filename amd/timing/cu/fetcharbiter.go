package cu

import (
	"math"

	"github.com/sarchlab/akita/v5/timing"
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
) []*wavefront.Wavefront {
	list := make([]*wavefront.Wavefront, 0, 1)

	oldestTime := timing.VTimeInPicoSec(math.MaxUint64)
	var toFetch *wavefront.Wavefront
	for _, wfPool := range wfPools {
		for _, wf := range wfPool.wfs {
			if !a.canFetchFromWF(wf) {
				continue
			}

			if wf.LastFetchTime < oldestTime {
				toFetch = wf
				oldestTime = wf.LastFetchTime
			}
		}
	}

	if toFetch != nil {
		list = append(list, toFetch)
	}

	return list
}

func (a *FetchArbiter) canFetchFromWF(wf *wavefront.Wavefront) bool {
	if wf.IsFetching {
		return false
	}

	if wf.State == wavefront.WfCompleted {
		return false
	}

	if len(wf.InstBuffer) >= a.InstBufByteSize {
		return false
	}

	if wf.CodeObject != nil && wf.CodeObject.Symbol != nil {
		lastPCInBinary := wf.CodeObject.Symbol.Size +
			wf.WG.Packet.KernelObject
		lastPCInInstBuffer := wf.InstBufferStartPC +
			uint64(len(wf.InstBuffer))
		if lastPCInInstBuffer >= lastPCInBinary {
			return false
		}
	}

	return true
}
