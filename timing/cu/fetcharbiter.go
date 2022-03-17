package cu

import (
	"math"

	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/mgpusim/v3/timing/wavefront"
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

	oldestTime := sim.VTimeInSec(math.MaxFloat64)
	var toFetch *wavefront.Wavefront
	for _, wfPool := range wfPools {
		for _, wf := range wfPool.wfs {
			wf.RLock()

			if wf.IsFetching {
				wf.RUnlock()
				continue
			}

			if wf.State == wavefront.WfCompleted {
				wf.RUnlock()
				continue
			}

			if len(wf.InstBuffer) >= a.InstBufByteSize {
				wf.RUnlock()
				continue
			}

			if wf.LastFetchTime < oldestTime {
				toFetch = wf
				oldestTime = wf.LastFetchTime
			}
			wf.RUnlock()
		}
	}

	if toFetch != nil {
		list = append(list, toFetch)
	}

	return list
}
