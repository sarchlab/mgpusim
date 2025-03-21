package insts

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/bitops"
)

// An HsaCo is the kernel code to be executed on an AMD GPU
type HsaCo struct {
	*HsaCoHeader
	Symbol *elf.Symbol
	Data   []byte
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

// NewHsaCoFromHeader creates an HsaCo with the provided data and header
func NewHsaCoFromHeader(data []byte, header *HsaCoHeader) *HsaCo {
	o := new(HsaCo)
	o.Data = data
	o.HsaCoHeader = header
	return o
}

// NewHsacoHeader creates a HsaCoHeader object
func NewHsacoHeader(
	kernelName string,
	data []byte,
	metadata map[string]interface{},
) *HsaCoHeader {
	header := new(HsaCoHeader)

	header.initHeaderFromCodeObjectV4(kernelName, data, metadata)

	return header
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

// EnableSgprPrivateSegmentBuffer checks if the private segment buffer
// information need to write into the SGPR
func (h *HsaCoHeader) EnableSgprPrivateSegmentBuffer() bool {
	return extractBits(h.Flags, 0, 0) != 0
}

// EnableSgprDispatchPtr enables dispatch ptr
func (h *HsaCoHeader) EnableSgprDispatchPtr() bool {
	return extractBits(h.Flags, 1, 1) != 0
}

// EnableSgprQueuePtr enables queue ptr
func (h *HsaCoHeader) EnableSgprQueuePtr() bool {
	return extractBits(h.Flags, 2, 2) != 0
}

// EnableSgprKernelArgSegmentPtr enables
func (h *HsaCoHeader) EnableSgprKernelArgSegmentPtr() bool {
	return extractBits(h.Flags, 3, 3) != 0
}

// EnableSgprDispatchID enables dispatch ID
func (h *HsaCoHeader) EnableSgprDispatchID() bool {
	return extractBits(h.Flags, 4, 4) != 0
}

// EnableSgprFlatScratchInit enables init
func (h *HsaCoHeader) EnableSgprFlatScratchInit() bool {
	return extractBits(h.Flags, 5, 5) != 0
}

// EnableSgprPrivateSegementSize enables size
func (h *HsaCoHeader) EnableSgprPrivateSegementSize() bool {
	return extractBits(h.Flags, 6, 6) != 0
}

// EnableSgprGridWorkGroupCountX enables wg countx
func (h *HsaCoHeader) EnableSgprGridWorkGroupCountX() bool {
	return extractBits(h.Flags, 7, 7) != 0
}

// EnableSgprGridWorkGroupCountY enables wg county
func (h *HsaCoHeader) EnableSgprGridWorkGroupCountY() bool {
	return extractBits(h.Flags, 8, 8) != 0
}

// EnableSgprGridWorkGroupCountZ enables wg countz
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
	s += fmt.Sprintf("\tWork-Group Group Segment Byte Size: %d\n", h.WGGroupSegmentByteSize)
	s += fmt.Sprintf("\tKernarg Segment Byte Size:%d\n", h.KernargSegmentByteSize)
	s += fmt.Sprintf("\tRegisters:\n")
	s += fmt.Sprintf("\t\tEnable SGPR Private SegmentBuffer: %t\n", h.EnableSgprPrivateSegmentBuffer())
	s += fmt.Sprintf("\t\tEnable SGPR Dispatch Ptr: %t\n", h.EnableSgprDispatchPtr())
	s += fmt.Sprintf("\t\tEnable SGPR Queue Ptr: %t\n", h.EnableSgprQueuePtr())
	s += fmt.Sprintf("\t\tEnable SGPR Kernarg Segment Ptr: %t\n", h.EnableSgprKernelArgSegmentPtr())
	s += fmt.Sprintf("\t\tEnable SGPR Dispatch ID: %t\n",
		h.EnableSgprDispatchID())
	s += fmt.Sprintf("\t\tEnable SGPR Flat Scratch Init: %t\n", h.EnableSgprFlatScratchInit())
	s += fmt.Sprintf("\t\tEnable SGPR Private Segment Size: %t\n", h.EnableSgprPrivateSegementSize())
	s += fmt.Sprintf("\t\tEnable SGPR Work-Group Count (X, Y, Z): %t %t %t\n",
		h.EnableSgprGridWorkGroupCountX(),
		h.EnableSgprGridWorkGroupCountY(),
		h.EnableSgprGridWorkGroupCountZ())
	s += fmt.Sprintf("\t\tEnable SGPR Work-Group ID (X, Y, Z): %t %t %t\n",
		h.EnableSgprWorkGroupIDX(),
		h.EnableSgprWorkGroupIDY(),
		h.EnableSgprWorkGroupIDZ())
	s += fmt.Sprintf("\t\tEnable SGPR Work-Group Info %t\n", h.EnableSgprWorkGroupInfo())
	s += fmt.Sprintf("\t\tEnable SGPR Private Segment Wave Byte Offset: %t\n", h.EnableSgprPrivateSegmentWaveByteOffset())

	s += fmt.Sprintf("\t\tEnable VGPR Work-Item ID X: %t\n", true)
	s += fmt.Sprintf("\t\tEnable VGPR Work-Item ID Y: %t\n", h.EnableVgprWorkItemID() > 0)
	s += fmt.Sprintf("\t\tEnable VGPR Work-Item ID Z: %t\n", h.EnableVgprWorkItemID() > 1)

	return s
}

func (h *HsaCoHeader) initHeaderFromCodeObjectV4(
	kernelName string,
	data []byte,
	metadata map[string]interface{},
) {
	h.WGGroupSegmentByteSize = bitops.BytesToU32(data[0:4])
	h.WIPrivateSegmentByteSize = bitops.BytesToU32(data[4:8])
	h.KernargSegmentByteSize = uint64(bitops.BytesToU32(data[8:12]))

	if bitops.BytesToU32(data[12:16]) != 0 {
		panic(fmt.Sprintf("Unsupported: Reserved, must be 0. (%v)", bitops.BytesToU32(data[12:16])))
	}

	h.KernelCodeEntryByteOffset = bitops.BytesToU64(data[16:24])

	if bitops.BytesToU64(data[24:32]) != 0 || bitops.BytesToU64(data[32:40]) != 0 || bitops.BytesToU32(data[40:44]) != 0 {
		panic("Unsupported: Reserved, must be 0.")
	}

	if bitops.BytesToU32(data[44:48]) != 0 {
		panic("Unsupported: COMPUTE_PGM_RSRC3 must be 0 for GFX6~GFX9.")
	}

	h.ComputePgmRsrc1 = bitops.BytesToU32(data[48:52])
	h.ComputePgmRsrc2 = bitops.BytesToU32(data[52:56])

	h.Flags = bitops.ExtractBitsFromU32(bitops.BytesToU32(data[56:60]), 0, 6)

	if bitops.ExtractBitsFromU32(bitops.BytesToU32(data[56:60]), 7, 9) != 0 {
		panic("Unsupported: Reserved, must be 0.")
	}

	// Seems have a bug here. The behavior is different from the docs.
	// https://llvm.org/docs/AMDGPUUsage.html#amdgpu-amdhsa-kernel-descriptor
	if bitops.ExtractBit(bitops.BytesToU32(data[56:60]), 10) != 0 {
		log.Print("Unsupported: ENABLE_WAVEFRONT_SIZE32 Reserved, must be 0 for GFX6~GFX9.")
	}

	if bitops.ExtractBit(bitops.BytesToU32(data[56:60]), 11) != 0 {
		log.Println("Unsupported: Indicates if the generated machine code is using a dynamically sized stack. ",
			"This is only set in code object v5 and later.")
	}

	if bitops.ExtractBitsFromU32(bitops.BytesToU32(data[56:60]), 12, 15) != 0 {
		panic("Unsupported: Reserved, must be 0.")
	}

	if bitops.ExtractBitsFromU32(bitops.BytesToU32(data[56:60]), 16, 22) != 0 {
		panic("Unsupported: KERNARG_PRELOAD_SPEC_LENGTH Reserved, must be 0 for GFX6~GFX9.")
	}

	if bitops.ExtractBitsFromU32(bitops.BytesToU32(data[56:60]), 23, 31) != 0 {
		panic("Unsupported: KERNARG_PRELOAD_SPEC_OFFSET Reserved, must be 0 for GFX6~GFX9.")
	}

	if bitops.BytesToU32(data[60:64]) != 0 {
		panic("Unsupported: Reserved, must be 0.")
	}

	findKernel := false
	for _, v := range metadata["amdhsa.kernels"].([]interface{}) {
		if v.(map[string]interface{})[".name"].(string) == kernelName {
			findKernel = true

			h.WIVgprCount = uint16(v.(map[string]interface{})[".vgpr_count"].(int8))
			h.WFSgprCount = uint16(v.(map[string]interface{})[".sgpr_count"].(int8))

			break
		}
	}

	if !findKernel {
		panic(fmt.Sprintf("Kernel %s not found in metadata", kernelName))
	}
}
