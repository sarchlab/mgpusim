package cu

// An WfArbitor can decide which wavefront can take action,
// in a list of wavefront pools
type WfArbitor interface {
	Arbitrate(wfpools []*WavefrontPool) []*Wavefront
}
