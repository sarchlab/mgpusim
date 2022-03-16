package resource

import "gitlab.com/akita/mgpusim/v3/kernels"

// WfLocation defines where the wavefront should be placed.
type WfLocation struct {
	Wavefront  *kernels.Wavefront
	SIMDID     int
	VGPROffset int
	SGPROffset int
	LDSOffset  int
}
