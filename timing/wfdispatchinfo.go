package timing

import "gitlab.com/akita/gcn3/kernels"

// WfDispatchInfo preservers the information from a work-group mapping to
// guarantee a wavefront to be dispatching to its designated location
type WfDispatchInfo struct {
	Wavefront  *kernels.Wavefront
	SIMDID     int
	VGPROffset int
	SGPROffset int
	LDSOffset  int
}
