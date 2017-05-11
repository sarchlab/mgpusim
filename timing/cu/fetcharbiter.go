package cu

import "gitlab.com/yaotsu/core"
import "math"

// A FetchArbiter can decide which wavefront in a scheduler can fetch
// instructions
type FetchArbiter struct {
}

// Arbitrate decide which wavefront can fetch the next instruction
func (a *FetchArbiter) Arbitrate(wfPools []*WavefrontPool) []*Wavefront {
	list := make([]*Wavefront, 1, 1)

	oldestTime := core.VTimeInSec(math.MaxFloat64)
	for _, wfPool := range wfPools {
		for _, wf := range wfPool.wfs {
			if wf.State != WfReady {
				break
			}

			if wf.LastFetchTime < oldestTime {
				list[0] = wf
			}
		}
	}

	return list
}
