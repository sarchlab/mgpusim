package resource

// WfLocation defines where the wavefront should be placed.
type WfLocation struct {
	WavefrontID uint64
	SIMDID      int
	VGPROffset  int
	SGPROffset  int
	LDSOffset   int
}
