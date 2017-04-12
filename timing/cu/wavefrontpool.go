package cu

// A WavefrontPool holds the wavefronts that will be scheduled in one SIMD
// unit
type WavefrontPool struct {
	capacity int

	Wfs         []*Wavefront
	FetchBuffer []*FetchInfo
}

// NewWavefrontPool creates and returns a new WavefrontPool
func NewWavefrontPool() *WavefrontPool {
	p := new(WavefrontPool)

	p.Wfs = make([]*Wavefront, 0, 0)

	return p
}
