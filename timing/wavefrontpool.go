package timing

import "gitlab.com/yaotsu/core"

// A WavefrontPool holds the wavefronts that will be scheduled in one SIMD
// unit
type WavefrontPool struct {
	Capacity int
	wfs      []*Wavefront
	VRegFile core.Component
}

// NewWavefrontPool creates and returns a new WavefrontPool
func NewWavefrontPool(capacity int) *WavefrontPool {
	p := new(WavefrontPool)

	p.Capacity = capacity
	p.wfs = make([]*Wavefront, 0)

	return p
}

// AddWf will add an wavefront to the wavefront pool
func (wfp *WavefrontPool) AddWf(wf *Wavefront) {
	wfp.wfs = append(wfp.wfs, wf)
}

// Availability returns the number of extra Wavefront that the wavefront pool
// can hold
func (wfp *WavefrontPool) Availability() int {
	return wfp.Capacity - len(wfp.wfs)
}

// RemoveWf removes a wavefront from a wavefront pool
func (wfp *WavefrontPool) RemoveWf(wf *Wavefront) {
	for i, wfToRemove := range wfp.wfs {
		if wfToRemove == wf {
			wfp.wfs = append(wfp.wfs[:i], wfp.wfs[i+1:]...)
			return
		}
	}
}
