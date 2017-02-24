package disasm

import (
	"encoding/binary"
	"fmt"
)

// An HsaCo is the kernel code to be executed on an AMD GPU
type HsaCo struct {
	Data []byte
}

type HsaCoHeader struct {
	CodeVersionMajor uint32
}

// NewHsaCo creates an HsaCo with the provided data
func NewHsaCo(data []byte) *HsaCo {
	o := new(HsaCo)
	o.Data = data

	return o
}

// CodeVersionMajor return the HSA code version major
func (o *HsaCo) CodeVersionMajor() uint32 {
	version, _ := binary.Uvarint(o.Data[0:4])
	return uint32(version)
}

// CodeVersionMinor returns the code version minor
func (o *HsaCo) CodeVersionMinor() uint32 {
	version, _ := binary.Uvarint(o.Data[4:8])
	return uint32(version)
}

// MachineType returns the machine type of the HSACO
func (o *HsaCo) MachineType() uint16 {
	return 0
}

// MachineVersionMajor returns the machine version major
func (o *HsaCo) MachineVersionMajor() uint16 {
	return 0
}

// MachineVersionMinor returns the machien version minor
func (o *HsaCo) MachineVersionMinor() uint16 {
	return 0
}

// MachineVersionStepping returns the machine versino stepping
func (o *HsaCo) MachineVersionStepping() uint16 {
	return 0
}

// KernelCodeEntryByteOffset returns the kernel code entry byte offset
func (o *HsaCo) KernelCodeEntryByteOffset() uint64 {
	offset, n := binary.Uvarint(o.Data[16:24])
	if n != 8 {
		_ = fmt.Errorf("Cannot decode the KernelCodeEntryByteOffsetField")
	}
	return offset
}

// KernelCodePrefetchByteOffset returns the kernel code prefetch byte offset
func (o *HsaCo) KernelCodePrefetchByteOffset() uint64 {
	return 0
}

// KernelCodePrefetchByteSize returns the kernel code prefetch byte size
func (o *HsaCo) KernelCodePrefetchByteSize() uint64 {
	return 0
}

// MaxScratchBackingMemoryByteSize returns the max scratch backing memory byte size
func (o *HsaCo) MaxScratchBackingMemoryByteSize() uint64 {
	return 0
}

// ComputePgmRsrc1 returns the compute PGM RSRC 1
func (o *HsaCo) ComputePgmRsrc1() uint32 {
	return 0
}

// ComputePgmRsrc2 returns the compute PGM RSRC 2
func (o *HsaCo) ComputePgmRsrc2() uint32 {
	return 0
}

// WIPrivateSegmentByteSize returns the work-item private segment byte size
func (o *HsaCo) WIPrivateSegmentByteSize() uint32 {
	return 0
}

// WGGroupSegmentByteSize returns the work-group segment byte size
func (o *HsaCo) WGGroupSegmentByteSize() uint32 {
	return 0
}

// GDSSegmentByteSize returns the GDS segment byte size
func (o *HsaCo) GDSSegmentByteSize() uint32 {
	return 0
}

// KernargSegmentByteSize returns the Kernarg segment byte size
func (o *HsaCo) KernargSegmentByteSize() uint64 {
	return 0
}

// WGFBarrierCount returns the work-group Fbarrier count
func (o *HsaCo) WGFBarrierCount() uint32 {
	return 0
}

// WFSgprCount returns the wavefront SGPR Count
func (o *HsaCo) WFSgprCount() uint16 {
	return 0
}

// WIVgprCount returns the work-item VGPR count
func (o *HsaCo) WIVgprCount() uint16 {
	return 0
}

// ReservedVgprFirst returns the reserved VGPR first
func (o *HsaCo) ReservedVgprFirst() uint16 {
	return 0
}

// ReservedVgprCount returns the reserved VGPR count
func (o *HsaCo) ReservedVgprCount() uint16 {
	return 0
}

// ReservedSgprFirst returns the reserved SGPR first
func (o *HsaCo) ReservedSgprFirst() uint16 {
	return 0
}

// ReservedSgprCount returns the reserved SGPR count
func (o *HsaCo) ReservedSgprCount() uint16 {
	return 0
}

// DebugWFPrivateSegmentOffsetSgpr returns the debug wavefront private segment
// offset SGPR
func (o *HsaCo) DebugWFPrivateSegmentOffsetSgpr() uint16 {
	return 0
}

// DebugPrivateSegmentBufferSgpr returns the debug private segment buffer sgpr
func (o *HsaCo) DebugPrivateSegmentBufferSgpr() uint16 {
	return 0
}

// KernargSegmentAlignment returns the kernarg segment alignment
func (o *HsaCo) KernargSegmentAlignment() uint8 {
	return 0
}

// GroupSegmentAlignment returns the group segment alignment
func (o *HsaCo) GroupSegmentAlignment() uint8 {
	return 0
}

// PrivateSegmentAlignment returns the private segment alignment
func (o *HsaCo) PrivateSegmentAlignment() uint8 {
	return 0
}

// WavefrontSize returns the wavefront size
func (o *HsaCo) WavefrontSize() uint8 {
	return 0
}

// CallConvention returns the call convention
func (o *HsaCo) CallConvention() uint32 {
	return 0
}

// RuntimeLoaderKernelSymbol returns the runtime loader kernel symbol
func (o *HsaCo) RuntimeLoaderKernelSymbol() uint64 {
	symbol, _ := binary.Uvarint(o.Data[120:128])
	return symbol
}

// ControlDirective returns the control directive
func (o *HsaCo) ControlDirective() []byte {
	return nil
}

// InstructionData returns the actual instruction binary of the HSACO
func (o *HsaCo) InstructionData() []byte {
	return o.Data[256:]
}
