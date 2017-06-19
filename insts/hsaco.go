package insts

import (
	"bytes"
	"encoding/binary"
	"fmt"
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

// NewHsaCo creates a zero-filled HsaCo object
func NewHsaCo() *HsaCo {
	co := new(HsaCo)
	co.HsaCoHeader = new(HsaCoHeader)
	return co
}

// NewHsaCoFromData creates an HsaCo with the provided data
func NewHsaCoFromData(data []byte) *HsaCo {
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

// WorkItemVgprCount returns the number of VGPRs used by each work-item
func (h *HsaCoHeader) WorkItemVgprCount() uint32 {
	return extractBits(h.ComputePgmRsrc1, 0, 5)
}

// WavefrontSgprCount returns the number of SGPRs used by each wavefront
func (h *HsaCoHeader) WavefrontSgprCount() uint32 {
	return extractBits(h.ComputePgmRsrc1, 6, 9)
}

// Priority returns the priority of the kernel
func (h *HsaCoHeader) Priority() uint32 {
	return extractBits(h.ComputePgmRsrc1, 10, 11)
}

// EnableSgprPrivateSegemtWaveByteOffset
func (h *HsaCoHeader) EnableSgprPrivateSegmentWaveByteOffset() bool {
	return extractBits(h.ComputePgmRsrc2, 0, 0) != 0
}

func (h *HsaCoHeader) UserSgprCount() uint32 {
	return extractBits(h.ComputePgmRsrc2, 1, 5)
}

func (h *HsaCoHeader) EnableSgprWorkGroupIdX() bool {
	return extractBits(h.ComputePgmRsrc2, 7, 7) != 0
}

func (h *HsaCoHeader) EnableSgprWorkGroupIdY() bool {
	return extractBits(h.ComputePgmRsrc2, 8, 8) != 0
}

func (h *HsaCoHeader) EnableSgprWorkGroupIdZ() bool {
	return extractBits(h.ComputePgmRsrc2, 9, 9) != 0
}

// EnableSgprWorkGroupInfo
func (h *HsaCoHeader) EnableSgprWorkGroupInfo() bool {
	return extractBits(h.ComputePgmRsrc2, 10, 10) != 0
}

// EnableVpgrWorkItemId checks if the setup of the work-item is enabled
func (h *HsaCoHeader) EnableVgprWorkItemId() uint32 {
	return extractBits(h.ComputePgmRsrc2, 11, 12)
}

func (h *HsaCoHeader) EnableExceptionAddressWatch() bool {
	return extractBits(h.ComputePgmRsrc2, 13, 13) != 0
}

func (h *HsaCoHeader) EnableExceptionMemoryViolation() bool {
	return extractBits(h.ComputePgmRsrc2, 14, 14) != 0
}

// EnableSgpPrivateSegmentBuffer checks if the private segment buffer
// information need to write into the SGPR
func (h *HsaCoHeader) EnableSgprPrivateSegmentBuffer() bool {
	return extractBits(h.Flags, 0, 0) != 0
}

func (h *HsaCoHeader) EnableSgprDispatchPtr() bool {
	return extractBits(h.Flags, 1, 1) != 0
}

func (h *HsaCoHeader) EnableSgprQueuePtr() bool {
	return extractBits(h.Flags, 2, 2) != 0
}

func (h *HsaCoHeader) EnableSgprKernelArgSegmentPtr() bool {
	return extractBits(h.Flags, 3, 3) != 0
}

func (h *HsaCoHeader) EnableSgprDispatchId() bool {
	return extractBits(h.Flags, 4, 4) != 0
}

func (h *HsaCoHeader) EnableSgprFlatScratchInit() bool {
	return extractBits(h.Flags, 5, 5) != 0
}

func (h *HsaCoHeader) EnableSgprPrivateSegementSize() bool {
	return extractBits(h.Flags, 6, 6) != 0
}

func (h *HsaCoHeader) EnableSgprGridWorkGroupCountX() bool {
	return extractBits(h.Flags, 7, 7) != 0
}

func (h *HsaCoHeader) EnableSgprGridWorkGroupCountY() bool {
	return extractBits(h.Flags, 8, 8) != 0
}

func (h *HsaCoHeader) EnableSgprGridWorkGroupCountZ() bool {
	return extractBits(h.Flags, 9, 9) != 0
}

// Info prints the human readable information that is carried by the HsaCoHeader
func (h *HsaCoHeader) Info() string {
	s := "HSA Code Object:\n"
	s += fmt.Sprintf("\tVersion: %d.%d\n", h.CodeVersionMajor, h.CodeVersionMinor)
	s += fmt.Sprintf("\tMachine: %d.%d.%d\n", h.MachineVersionMajor, h.MachineVersionMinor, h.MachineVersionStepping)
	s += fmt.Sprintf("\tCode Entry Byte Offset: %d\n", h.KernelCodeEntryByteOffset)
	s += fmt.Sprintf("\tPrefetch: %d (size: %d)\n", h.KernelCodePrefetchByteOffset, h.KernelCodePrefetchByteSize)
	s += fmt.Sprintf("\tMax Scratch Memory: %d\n", h.MaxScratchBackingMemoryByteSize)
	s += fmt.Sprintf("\tGranulated WI VGPR Count:%d\n", h.WIVgprCount)
	s += fmt.Sprintf("\tGranulated Wf SGPR Count:%d\n", h.WFSgprCount)
	s += fmt.Sprintf("\tRegisters:\n")
	s += fmt.Sprintf("\t\tEnable SGPR Private SegmentBuffer: %t\n", h.EnableSgprPrivateSegmentBuffer())
	s += fmt.Sprintf("\t\tEnable SGPR Dispatch Ptr: %t\n", h.EnableSgprDispatchPtr())
	s += fmt.Sprintf("\t\tEnable SGPR Queue Ptr: %t\n", h.EnableSgprQueuePtr())
	s += fmt.Sprintf("\t\tEnable SGPR Kernarg Segment Ptr: %t\n", h.EnableSgprKernelArgSegmentPtr())
	s += fmt.Sprintf("\t\tEnable SGPR Dispatch ID: %t\n", h.EnableSgprDispatchId())
	s += fmt.Sprintf("\t\tEnable SGPR Flat Scratch Init: %t\n", h.EnableSgprFlatScratchInit())
	s += fmt.Sprintf("\t\tEnable SGPR Private Segment Size: %t\n", h.EnableSgprPrivateSegementSize())
	s += fmt.Sprintf("\t\tEnable SGPR Work-Group Count (X, Y, Z): %t %t %t\n",
		h.EnableSgprGridWorkGroupCountX(),
		h.EnableSgprGridWorkGroupCountY(),
		h.EnableSgprGridWorkGroupCountZ())
	s += fmt.Sprintf("\t\tEnable SGPR Work-Group ID (X, Y, Z): %t %t %t\n",
		h.EnableSgprWorkGroupIdX(),
		h.EnableSgprWorkGroupIdY(),
		h.EnableSgprWorkGroupIdZ())
	s += fmt.Sprintf("\t\tEnable SGPR Work-Group Info %t\n", h.EnableSgprWorkGroupInfo())
	s += fmt.Sprintf("\t\tEnable SGPR Private Segment Wave Byte Offset: %t\n", h.EnableSgprPrivateSegmentWaveByteOffset())

	s += fmt.Sprintf("\t\tEnable VGPR Work-Item ID X: %t\n", true)
	s += fmt.Sprintf("\t\tEnable VGPR Work-Item ID Y: %t\n", h.EnableVgprWorkItemId() > 0)
	s += fmt.Sprintf("\t\tEnable VGPR Work-Item ID Z: %t\n", h.EnableVgprWorkItemId() > 1)

	return s
}
