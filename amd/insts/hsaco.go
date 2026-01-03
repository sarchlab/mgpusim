package insts

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
)

// CodeObjectVersion represents the AMDGPU code object version
type CodeObjectVersion int

const (
	// CodeObjectV2 is the legacy code object format (GCN3)
	CodeObjectV2 CodeObjectVersion = 2
	// CodeObjectV3 is the code object format with 256-byte header
	CodeObjectV3 CodeObjectVersion = 3
	// CodeObjectV5 is the new code object format (GFX9+)
	CodeObjectV5 CodeObjectVersion = 5
)

// An HsaCo is the kernel code to be executed on an AMD GPU
type HsaCo struct {
	*HsaCoMeta
	Symbol  *elf.Symbol
	Data    []byte // Instruction data only (no header)
	Version CodeObjectVersion
}

// HsaCoMeta contains the metadata of an HSACO kernel
// This struct is populated from either V2/V3 header or V5 kernel descriptor
type HsaCoMeta struct {
	// Common fields across versions
	ComputePgmRsrc1        uint32
	ComputePgmRsrc2        uint32
	ComputePgmRsrc3        uint32 // V5 only, 0 for V2/V3
	KernargSegmentByteSize uint64
	GroupSegmentByteSize   uint32
	PrivateSegmentByteSize uint32

	// Kernel entry point offset (from start of code object)
	// For V2/V3: typically 256 (instructions after header)
	// For V5: typically 0 (instructions at start of .text)
	KernelCodeEntryByteOffset uint64

	// Flags/Properties - unified from V2/V3 Flags and V5 KernelCodeProperties
	EnableSgprPrivateSegmentBuffer bool
	EnableSgprDispatchPtr          bool
	EnableSgprQueuePtr             bool
	EnableSgprKernargSegmentPtr    bool
	EnableSgprDispatchID           bool
	EnableSgprFlatScratchInit      bool
	EnableSgprPrivateSegmentSize   bool
	EnableSgprGridWorkgroupCountX  bool
	EnableSgprGridWorkgroupCountY  bool
	EnableSgprGridWorkgroupCountZ  bool

	// V2/V3 specific fields (kept for compatibility)
	CodeVersionMajor         uint32
	CodeVersionMinor         uint32
	MachineKind              uint16
	MachineVersionMajor      uint16
	MachineVersionMinor      uint16
	MachineVersionStepping   uint16
	WFSgprCount              uint16
	WIVgprCount              uint16
}

// NewHsaCo creates a zero-filled HsaCo object
func NewHsaCo() *HsaCo {
	co := new(HsaCo)
	co.HsaCoMeta = new(HsaCoMeta)
	return co
}

// NewHsaCoFromELF creates an HsaCo from an ELF file
// It auto-detects the code object version and extracts metadata and instructions
// from the appropriate sections
func NewHsaCoFromELF(elfFile *elf.File) *HsaCo {
	o := new(HsaCo)

	textSec := elfFile.Section(".text")
	if textSec == nil {
		return nil
	}

	textData, err := textSec.Data()
	if err != nil {
		return nil
	}

	// Detect version by checking if .text starts with V2/V3 header
	if len(textData) >= 256 && isV2V3Header(textData) {
		// V2/V3 format: 256-byte header followed by instructions in .text
		o.HsaCoMeta = parseV2V3Header(textData)
		o.Data = textData[256:] // Instructions start after 256-byte header
		o.Version = CodeObjectV3
	} else {
		// V5 format: metadata in .rodata, instructions in .text
		o.Data = textData
		o.Version = CodeObjectV5

		// Try to parse kernel descriptor from .rodata
		rodataSec := elfFile.Section(".rodata")
		if rodataSec != nil {
			rodataData, err := rodataSec.Data()
			if err == nil && len(rodataData) >= 64 {
				o.HsaCoMeta = parseV5KernelDescriptor(rodataData)
			} else {
				o.HsaCoMeta = new(HsaCoMeta)
			}
		} else {
			o.HsaCoMeta = new(HsaCoMeta)
		}
	}

	return o
}

// isV2V3Header checks if data looks like a V2/V3 kernel header
func isV2V3Header(data []byte) bool {
	if len(data) < 256 {
		return false
	}

	// V2/V3 header signature:
	// - CodeVersionMajor (offset 0-3) = 1
	// - CodeVersionMinor (offset 4-7) = 0, 1, or 2
	// - MachineKind (offset 8-9) = 1 (AMDGPU)
	codeVersionMajor := binary.LittleEndian.Uint32(data[0:4])
	codeVersionMinor := binary.LittleEndian.Uint32(data[4:8])
	machineKind := binary.LittleEndian.Uint16(data[8:10])

	// Check for valid V2/V3 header values
	if codeVersionMajor == 1 && codeVersionMinor <= 2 && machineKind == 1 {
		return true
	}

	return false
}

// parseV2V3Header parses the 256-byte V2/V3 kernel header
func parseV2V3Header(data []byte) *HsaCoMeta {
	meta := new(HsaCoMeta)

	// Parse fields from 256-byte header using little-endian
	meta.CodeVersionMajor = binary.LittleEndian.Uint32(data[0:4])
	meta.CodeVersionMinor = binary.LittleEndian.Uint32(data[4:8])
	meta.MachineKind = binary.LittleEndian.Uint16(data[8:10])
	meta.MachineVersionMajor = binary.LittleEndian.Uint16(data[10:12])
	meta.MachineVersionMinor = binary.LittleEndian.Uint16(data[12:14])
	meta.MachineVersionStepping = binary.LittleEndian.Uint16(data[14:16])
	meta.KernelCodeEntryByteOffset = binary.LittleEndian.Uint64(data[16:24])
	// KernelCodePrefetchByteOffset at 24:32 (skip)
	// KernelCodePrefetchByteSize at 32:40 (skip)
	// MaxScratchBackingMemoryByteSize at 40:48 (skip)
	meta.ComputePgmRsrc1 = binary.LittleEndian.Uint32(data[48:52])
	meta.ComputePgmRsrc2 = binary.LittleEndian.Uint32(data[52:56])

	flags := binary.LittleEndian.Uint32(data[56:60])
	meta.EnableSgprPrivateSegmentBuffer = (flags & (1 << 0)) != 0
	meta.EnableSgprDispatchPtr = (flags & (1 << 1)) != 0
	meta.EnableSgprQueuePtr = (flags & (1 << 2)) != 0
	meta.EnableSgprKernargSegmentPtr = (flags & (1 << 3)) != 0
	meta.EnableSgprDispatchID = (flags & (1 << 4)) != 0
	meta.EnableSgprFlatScratchInit = (flags & (1 << 5)) != 0
	meta.EnableSgprPrivateSegmentSize = (flags & (1 << 6)) != 0
	meta.EnableSgprGridWorkgroupCountX = (flags & (1 << 7)) != 0
	meta.EnableSgprGridWorkgroupCountY = (flags & (1 << 8)) != 0
	meta.EnableSgprGridWorkgroupCountZ = (flags & (1 << 9)) != 0

	meta.PrivateSegmentByteSize = binary.LittleEndian.Uint32(data[60:64])
	meta.GroupSegmentByteSize = binary.LittleEndian.Uint32(data[64:68])
	// GDSSegmentByteSize at 68:72 (skip)
	meta.KernargSegmentByteSize = binary.LittleEndian.Uint64(data[72:80])
	// WGFBarrierCount at 80:84 (skip)
	meta.WFSgprCount = binary.LittleEndian.Uint16(data[84:86])
	meta.WIVgprCount = binary.LittleEndian.Uint16(data[86:88])

	return meta
}

// parseV5KernelDescriptor parses the 64-byte V5 kernel descriptor
func parseV5KernelDescriptor(data []byte) *HsaCoMeta {
	meta := new(HsaCoMeta)

	// V5 Kernel Descriptor layout (64 bytes):
	// 0:4   - group_segment_fixed_size
	// 4:8   - private_segment_fixed_size
	// 8:12  - kernarg_size
	// 12:16 - reserved
	// 16:24 - kernel_code_entry_byte_offset
	// 24:32 - reserved
	// 32:40 - reserved
	// 40:44 - compute_pgm_rsrc3
	// 44:48 - compute_pgm_rsrc1
	// 48:52 - compute_pgm_rsrc2
	// 52:54 - kernel_code_properties
	// 54:56 - kernarg_preload
	// 56:60 - reserved

	meta.GroupSegmentByteSize = binary.LittleEndian.Uint32(data[0:4])
	meta.PrivateSegmentByteSize = binary.LittleEndian.Uint32(data[4:8])
	meta.KernargSegmentByteSize = uint64(binary.LittleEndian.Uint32(data[8:12]))
	meta.KernelCodeEntryByteOffset = binary.LittleEndian.Uint64(data[16:24])
	meta.ComputePgmRsrc3 = binary.LittleEndian.Uint32(data[40:44])
	meta.ComputePgmRsrc1 = binary.LittleEndian.Uint32(data[44:48])
	meta.ComputePgmRsrc2 = binary.LittleEndian.Uint32(data[48:52])

	// Parse kernel_code_properties (different bit layout from V2/V3 flags)
	props := binary.LittleEndian.Uint16(data[52:54])
	meta.EnableSgprPrivateSegmentBuffer = (props & (1 << 0)) != 0
	meta.EnableSgprDispatchPtr = (props & (1 << 2)) != 0
	meta.EnableSgprQueuePtr = (props & (1 << 3)) != 0
	meta.EnableSgprKernargSegmentPtr = (props & (1 << 4)) != 0
	meta.EnableSgprDispatchID = (props & (1 << 5)) != 0
	meta.EnableSgprFlatScratchInit = (props & (1 << 6)) != 0
	meta.EnableSgprPrivateSegmentSize = (props & (1 << 7)) != 0
	// Note: V5 doesn't have grid workgroup count enables in the same way

	return meta
}

// InstructionData returns the instruction binaries in the HsaCo
func (o *HsaCo) InstructionData() []byte {
	return o.Data
}

// HsaCoHeader is an alias for HsaCoMeta for backward compatibility
// Deprecated: Use HsaCoMeta instead
type HsaCoHeader = HsaCoMeta

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

// EnableSgprPrivateSegmentWaveByteOffset enable wavebyteoffset
func (h *HsaCoHeader) EnableSgprPrivateSegmentWaveByteOffset() bool {
	return extractBits(h.ComputePgmRsrc2, 0, 0) != 0
}

// UserSgprCount returns user sgpr
func (h *HsaCoHeader) UserSgprCount() uint32 {
	return extractBits(h.ComputePgmRsrc2, 1, 5)
}

// EnableSgprWorkGroupIDX enable idx
func (h *HsaCoHeader) EnableSgprWorkGroupIDX() bool {
	return extractBits(h.ComputePgmRsrc2, 7, 7) != 0
}

// EnableSgprWorkGroupIDY enable idy
func (h *HsaCoHeader) EnableSgprWorkGroupIDY() bool {
	return extractBits(h.ComputePgmRsrc2, 8, 8) != 0
}

// EnableSgprWorkGroupIDZ enable idz
func (h *HsaCoHeader) EnableSgprWorkGroupIDZ() bool {
	return extractBits(h.ComputePgmRsrc2, 9, 9) != 0
}

// EnableSgprWorkGroupInfo enable wg info
func (h *HsaCoHeader) EnableSgprWorkGroupInfo() bool {
	return extractBits(h.ComputePgmRsrc2, 10, 10) != 0
}

// EnableVgprWorkItemID checks if the setup of the work-item is enabled
func (h *HsaCoHeader) EnableVgprWorkItemID() uint32 {
	return extractBits(h.ComputePgmRsrc2, 11, 12)
}

// EnableExceptionAddressWatch enable exception address watch
func (h *HsaCoHeader) EnableExceptionAddressWatch() bool {
	return extractBits(h.ComputePgmRsrc2, 13, 13) != 0
}

// EnableExceptionMemoryViolation enable exception memory violation
func (h *HsaCoHeader) EnableExceptionMemoryViolation() bool {
	return extractBits(h.ComputePgmRsrc2, 14, 14) != 0
}

// GetEnableSgprPrivateSegmentBuffer returns if the private segment buffer
// information needs to be written into the SGPR
func (h *HsaCoMeta) GetEnableSgprPrivateSegmentBuffer() bool {
	return h.EnableSgprPrivateSegmentBuffer
}

// GetEnableSgprDispatchPtr returns if dispatch ptr is enabled
func (h *HsaCoMeta) GetEnableSgprDispatchPtr() bool {
	return h.EnableSgprDispatchPtr
}

// GetEnableSgprQueuePtr returns if queue ptr is enabled
func (h *HsaCoMeta) GetEnableSgprQueuePtr() bool {
	return h.EnableSgprQueuePtr
}

// GetEnableSgprKernargSegmentPtr returns if kernarg segment ptr is enabled
func (h *HsaCoMeta) GetEnableSgprKernargSegmentPtr() bool {
	return h.EnableSgprKernargSegmentPtr
}

// GetEnableSgprDispatchID returns if dispatch ID is enabled
func (h *HsaCoMeta) GetEnableSgprDispatchID() bool {
	return h.EnableSgprDispatchID
}

// GetEnableSgprFlatScratchInit returns if flat scratch init is enabled
func (h *HsaCoMeta) GetEnableSgprFlatScratchInit() bool {
	return h.EnableSgprFlatScratchInit
}

// GetEnableSgprPrivateSegmentSize returns if private segment size is enabled
func (h *HsaCoMeta) GetEnableSgprPrivateSegmentSize() bool {
	return h.EnableSgprPrivateSegmentSize
}

// GetEnableSgprGridWorkgroupCountX returns if grid workgroup count X is enabled
func (h *HsaCoMeta) GetEnableSgprGridWorkgroupCountX() bool {
	return h.EnableSgprGridWorkgroupCountX
}

// GetEnableSgprGridWorkgroupCountY returns if grid workgroup count Y is enabled
func (h *HsaCoMeta) GetEnableSgprGridWorkgroupCountY() bool {
	return h.EnableSgprGridWorkgroupCountY
}

// GetEnableSgprGridWorkgroupCountZ returns if grid workgroup count Z is enabled
func (h *HsaCoMeta) GetEnableSgprGridWorkgroupCountZ() bool {
	return h.EnableSgprGridWorkgroupCountZ
}

// Info prints the human readable information that is carried by the HsaCoMeta
func (h *HsaCoMeta) Info() string {
	s := "HSA Code Object:\n"
	s += fmt.Sprintf("\tVersion: %d.%d\n", h.CodeVersionMajor, h.CodeVersionMinor)
	s += fmt.Sprintf("\tMachine: %d.%d.%d\n", h.MachineVersionMajor, h.MachineVersionMinor, h.MachineVersionStepping)
	s += fmt.Sprintf("\tGranulated WI VGPR Count: %d\n", h.WIVgprCount)
	s += fmt.Sprintf("\tGranulated Wf SGPR Count: %d\n", h.WFSgprCount)
	s += fmt.Sprintf("\tGroup Segment Byte Size: %d\n", h.GroupSegmentByteSize)
	s += fmt.Sprintf("\tPrivate Segment Byte Size: %d\n", h.PrivateSegmentByteSize)
	s += fmt.Sprintf("\tKernarg Segment Byte Size: %d\n", h.KernargSegmentByteSize)
	s += fmt.Sprintf("\tRegisters:\n")
	s += fmt.Sprintf("\t\tEnable SGPR Private Segment Buffer: %t\n", h.EnableSgprPrivateSegmentBuffer)
	s += fmt.Sprintf("\t\tEnable SGPR Dispatch Ptr: %t\n", h.EnableSgprDispatchPtr)
	s += fmt.Sprintf("\t\tEnable SGPR Queue Ptr: %t\n", h.EnableSgprQueuePtr)
	s += fmt.Sprintf("\t\tEnable SGPR Kernarg Segment Ptr: %t\n", h.EnableSgprKernargSegmentPtr)
	s += fmt.Sprintf("\t\tEnable SGPR Dispatch ID: %t\n", h.EnableSgprDispatchID)
	s += fmt.Sprintf("\t\tEnable SGPR Flat Scratch Init: %t\n", h.EnableSgprFlatScratchInit)
	s += fmt.Sprintf("\t\tEnable SGPR Private Segment Size: %t\n", h.EnableSgprPrivateSegmentSize)
	s += fmt.Sprintf("\t\tEnable SGPR Work-Group Count (X, Y, Z): %t %t %t\n",
		h.EnableSgprGridWorkgroupCountX,
		h.EnableSgprGridWorkgroupCountY,
		h.EnableSgprGridWorkgroupCountZ)
	s += fmt.Sprintf("\t\tEnable SGPR Work-Group ID (X, Y, Z): %t %t %t\n",
		h.EnableSgprWorkGroupIDX(),
		h.EnableSgprWorkGroupIDY(),
		h.EnableSgprWorkGroupIDZ())
	s += fmt.Sprintf("\t\tEnable SGPR Work-Group Info: %t\n", h.EnableSgprWorkGroupInfo())
	s += fmt.Sprintf("\t\tEnable SGPR Private Segment Wave Byte Offset: %t\n", h.EnableSgprPrivateSegmentWaveByteOffset())

	s += fmt.Sprintf("\t\tEnable VGPR Work-Item ID X: %t\n", true)
	s += fmt.Sprintf("\t\tEnable VGPR Work-Item ID Y: %t\n", h.EnableVgprWorkItemID() > 0)
	s += fmt.Sprintf("\t\tEnable VGPR Work-Item ID Z: %t\n", h.EnableVgprWorkItemID() > 1)

	return s
}
