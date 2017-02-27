package disasm

import (
	"bytes"
	"encoding/binary"
)

// An HsaCo is the kernel code to be executed on an AMD GPU
type HsaCo struct {
	*HsaCoHeader
	Data []byte
}

// HsaCoHeader contains the header information of an HSACO
type HsaCoHeader struct {
	CodeVersionMajor                uint32
	CodeVersionMinor                uint32
	MachineKind                     uint16
	MachineVersionMajor             uint16
	MachineVersionMinor             uint16
	MachineVersionStepping          uint16
	KernelCodeEntryByteOffset       uint64
	KernelCodePrefetchByteOffset    uint64
	KernelCodePrefetchByteSize      uint64
	MaxScratchBackingMemoryByteSize uint64
	ComputePgmRsrc1                 uint32
	ComputePgmRsrc2                 uint32
	Flags                           uint32
	WIPrivateSegmentByteSize        uint32
	WGGroupSegmentByteSize          uint32
	GDSSegmentByteSize              uint32
	KernargSegmentByteSize          uint64
	WGFBarrierCount                 uint32
	WFSgprCount                     uint16
	WIVgprCount                     uint16
	ReservedVgprFirst               uint16
	ReservedVgprCount               uint16
	ReservedSgprFirst               uint16
	ReservedSgprCount               uint16
	DebugWfPrivateSegmentOffsetSgpr uint16
	DebugPrivateSegmentBufferSgpr   uint16
	KernargSegmentAlignment         uint8
	GroupSegmentAlignment           uint8
	PrivateSegmentAlignment         uint8
	WavefrontSize                   uint8
	CallConvention                  uint32
	Reserved                        [12]byte
	RuntimeLoaderKernelSymbol       uint64
	ControlDirective                [128]byte
}

// NewHsaCo creates an HsaCo with the provided data
func NewHsaCo(data []byte) *HsaCo {
	o := new(HsaCo)
	o.Data = data

	header := new(HsaCoHeader)
	binary.Read(bytes.NewReader(data), binary.LittleEndian, header)
	o.HsaCoHeader = header

	return o
}

// InstructionData returns the instruction binaries in the HsaCo
func (o *HsaCo) InstructionData() []byte {
	return o.Data[256:]
}
