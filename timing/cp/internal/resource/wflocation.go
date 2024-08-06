package resource

import "github.com/sarchlab/mgpusim/v4/kernels"

// WfLocation defines where the wavefront should be placed.
type WfLocation struct {
	Wavefront  *kernels.Wavefront
	SIMDID     int
	VGPROffset int
	SGPROffset int
	LDSOffset  int
}
