package timing

import "gitlab.com/akita/mgpusim/timing/wavefront"

// An WfArbiter can decide which wavefront can take action,
// in a list of wavefront pools
type WfArbiter interface {
	Arbitrate(wfpools []*WavefrontPool) []*wavefront.Wavefront
}
