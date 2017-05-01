package cu

import "gitlab.com/yaotsu/core"

// A WavefrontPool holds the wavefronts that will be scheduled in one SIMD
// unit
type WavefrontPool struct {
	Capacity int
	Wfs      []*Wavefront
	VRegFile core.Component
}

// NewWavefrontPool creates and returns a new WavefrontPool
func NewWavefrontPool(capacity int) *WavefrontPool {
	p := new(WavefrontPool)

	p.Capacity = capacity
	p.Wfs = make([]*Wavefront, 0, 0)

	return p
}

// Availability returns the number of extra Wavefront that the wavefront pool
// can hold
func (wfp *WavefrontPool) Availability() int {
	return wfp.Capacity - len(wfp.Wfs)
}
