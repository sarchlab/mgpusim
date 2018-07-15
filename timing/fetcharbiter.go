package timing

import "gitlab.com/yaotsu/core"
import "math"

// A FetchArbiter can decide which wavefront in a scheduler can fetch
// instructions
type FetchArbiter struct {
	InstBufByteSize int
}

// Arbitrate decide which wavefront can fetch the next instruction
func (a *FetchArbiter) Arbitrate(wfPools []*WavefrontPool) []*Wavefront {
	list := make([]*Wavefront, 0, 1)

	oldestTime := core.VTimeInSec(math.MaxFloat64)
	var toFetch *Wavefront
	for _, wfPool := range wfPools {
		for _, wf := range wfPool.wfs {
			wf.RLock()

			if wf.IsFetching {
				wf.RUnlock()
				continue
			}

			if len(wf.InstBuffer) >= a.InstBufByteSize {
				wf.RUnlock()
				continue
			}

			if wf.LastFetchTime < oldestTime {
				toFetch = wf
			}
			wf.RUnlock()
		}
	}

	if toFetch != nil {
		list = append(list, toFetch)
	}

	return list
}
