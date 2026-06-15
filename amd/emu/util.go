package emu

import "unsafe"

// AsInt16 converts uint16 bits to int16.
func AsInt16(bits uint16) int16 {
	return *((*int16)((unsafe.Pointer(&bits))))
}

// AsInt32 converts uint32 bits to int32.
func AsInt32(bits uint32) int32 {
	return *((*int32)((unsafe.Pointer(&bits))))
}

// AsInt64 converts uint64 bits to int64.
func AsInt64(bits uint64) int64 {
	return *((*int64)((unsafe.Pointer(&bits))))
}

// AsFloat32 converts uint32 bits to float32.
func AsFloat32(bits uint32) float32 {
	return *((*float32)((unsafe.Pointer(&bits))))
}

// AsFloat64 converts uint64 bits to float64.
func AsFloat64(bits uint64) float64 {
	return *((*float64)((unsafe.Pointer(&bits))))
}

// Int16ToBits converts int16 to uint16 bits.
func Int16ToBits(num int16) uint16 {
	return *((*uint16)((unsafe.Pointer(&num))))
}

// Int32ToBits converts int32 to uint32 bits.
func Int32ToBits(num int32) uint32 {
	return *((*uint32)((unsafe.Pointer(&num))))
}

// Int64ToBits converts int64 to uint64 bits.
func Int64ToBits(num int64) uint64 {
	return *((*uint64)((unsafe.Pointer(&num))))
}

// Float32ToBits converts float32 to uint32 bits.
func Float32ToBits(num float32) uint32 {
	return *((*uint32)((unsafe.Pointer(&num))))
}

// Float64ToBits converts float64 to uint64 bits.
func Float64ToBits(num float64) uint64 {
	return *((*uint64)((unsafe.Pointer(&num))))
}

// Unexported aliases for backward compatibility within emu package
func asInt16(bits uint16) int16     { return AsInt16(bits) }
func asInt32(bits uint32) int32     { return AsInt32(bits) }
func asInt64(bits uint64) int64     { return AsInt64(bits) }
func asFloat32(bits uint32) float32 { return AsFloat32(bits) }
func asFloat64(bits uint64) float64 { return AsFloat64(bits) }
func int16ToBits(num int16) uint16  { return Int16ToBits(num) }
func int32ToBits(num int32) uint32  { return Int32ToBits(num) }
func int64ToBits(num int64) uint64  { return Int64ToBits(num) }
func float32ToBits(num float32) uint32 { return Float32ToBits(num) }
func float64ToBits(num float64) uint64 { return Float64ToBits(num) }

// LaneMasked checks if a lane is active in the EXEC mask.
func LaneMasked(Exec uint64, laneID uint) bool {
	return Exec&(1<<laneID) > 0
}

// laneMasked is the unexported wrapper for backward compatibility.
func laneMasked(Exec uint64, laneID uint) bool {
	return LaneMasked(Exec, laneID)
}
