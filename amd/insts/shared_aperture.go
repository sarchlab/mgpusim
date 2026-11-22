package insts

// CDNA3SharedApertureBase is the model's aligned base for the 4 GiB shared
// aperture. It is outside the global addresses allocated by the simulator.
const CDNA3SharedApertureBase = uint64(1) << 56

// CDNA3SharedOffset decodes a generic address in the shared aperture.
func CDNA3SharedOffset(address uint64) (uint32, bool) {
	offset := address - CDNA3SharedApertureBase
	return uint32(offset), offset < 1<<32
}
