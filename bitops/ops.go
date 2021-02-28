package bitops

// ExtractBitsFromU64 will get the bits from loInclude to hiInclude.
func ExtractBitsFromU64(num uint64, loInclude, hiInclude int) uint64 {
	var mask uint64
	var extracted uint64
	mask = ((1 << (hiInclude - loInclude + 1)) - 1) << loInclude
	extracted = (num & mask) >> loInclude
	return extracted
}

// ExtractBitsFromU32 will get the bits from loInclude to hiInclude.
func ExtractBitsFromU32(num uint32, loInclude, hiInclude int) uint32 {
	var mask uint32
	var extracted uint32
	mask = ((1 << (hiInclude - loInclude + 1)) - 1) << loInclude
	extracted = (num & mask) >> loInclude
	return extracted
}

// SignExt updates all the bits beyond the signBit to be the same as the sign
// bit.
func SignExt(in uint64, signBit int) (out uint64) {
	out = in

	var mask uint64
	mask = ^((1 << (signBit + 1)) - 1)

	sign := (in >> signBit) & 1

	if sign > 0 {
		out = out | mask
	} else {
		mask = ^mask
		out = out & mask
	}

	return out
}
