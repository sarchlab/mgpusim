package insts

import "encoding/binary"

// Uint8ToBytes returns the bytes representation of a uint32 value
func Uint8ToBytes(num uint8) []byte {
	data := make([]byte, 1)
	data[0] = num
	return data
}

// BytesToUint8 decode a uint32 number from bytes
func BytesToUint8(data []byte) uint8 {
	return data[0]
}

// Uint32ToBytes returns the bytes representation of a uint32 value
func Uint32ToBytes(num uint32) []byte {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, num)
	return data
}

// BytesToUint32 decode a uint32 number from bytes
func BytesToUint32(data []byte) uint32 {
	return binary.LittleEndian.Uint32(data)
}

// Uint64ToBytes returns the bytes representation of a uint64 value
func Uint64ToBytes(num uint64) []byte {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, num)
	return data
}

// BytesToUint64 decode a uint64 number from bytes
func BytesToUint64(data []byte) uint64 {
	return binary.LittleEndian.Uint64(data)
}
