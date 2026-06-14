package gcn3

import "github.com/sarchlab/mgpusim/v5/amd/emu"

// Unexported helper aliases for the bit-reinterpretation utilities used by the
// GCN3 per-format ALU emulation code. They delegate to the shared exported
// helpers in the emu package.
func asInt16(bits uint16) int16        { return emu.AsInt16(bits) }
func asInt32(bits uint32) int32        { return emu.AsInt32(bits) }
func asInt64(bits uint64) int64        { return emu.AsInt64(bits) }
func asFloat32(bits uint32) float32    { return emu.AsFloat32(bits) }
func int16ToBits(num int16) uint16     { return emu.Int16ToBits(num) }
func int32ToBits(num int32) uint32     { return emu.Int32ToBits(num) }
func int64ToBits(num int64) uint64     { return emu.Int64ToBits(num) }
func float32ToBits(num float32) uint32 { return emu.Float32ToBits(num) }
