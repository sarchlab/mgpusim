package emu

import "unsafe"

func asInt32(bits uint32) int32 {
	return *((*int32)((unsafe.Pointer(&bits))))
}

func asInt64(bits uint64) int64 {
	return *((*int64)((unsafe.Pointer(&bits))))
}

func asFloat32(bits uint32) float32 {
	return *((*float32)((unsafe.Pointer(&bits))))
}

func asFloat64(bits uint64) float64 {
	return *((*float64)((unsafe.Pointer(&bits))))
}

func int32ToBits(num int32) uint32 {
	return *((*uint32)((unsafe.Pointer(&num))))
}

func int64ToBits(num int64) uint64 {
	return *((*uint64)((unsafe.Pointer(&num))))
}

func float32ToBits(num float32) uint32 {
	return *((*uint32)((unsafe.Pointer(&num))))
}

func float64ToBits(num float64) uint64 {
	return *((*uint64)((unsafe.Pointer(&num))))
}
