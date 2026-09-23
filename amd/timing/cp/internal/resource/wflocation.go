package resource

import "github.com/sarchlab/mgpusim/v5/amd/kernels"

// WfLocation defines where the wavefront should be placed.
type WfLocation struct {
	Wavefront  *kernels.Wavefront
	SIMDID     int
	VGPROffset int
	SGPROffset int
	LDSOffset  int
}
