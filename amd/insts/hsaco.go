package insts

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"log"
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

// An KernelCodeObject is the kernel code to be executed on an AMD GPU
type KernelCodeObject struct {
	*KernelCodeObjectMeta
	Symbol  *elf.Symbol
	Data    []byte // Instruction data only (no header)
	Version CodeObjectVersion
}

// KernelCodeObjectMeta contains the metadata of an HSACO kernel
// This struct is populated from either V2/V3 header or V5 kernel descriptor
type KernelCodeObjectMeta struct {
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

// newKernelCodeObjectFromEntireTextSection creates a KernelCodeObject from raw kernel data.
// The data should start with the 256-byte V2/V3 header followed by instructions.
// This is an internal helper used by the load functions.
func newKernelCodeObjectFromEntireTextSection(data []byte) *KernelCodeObject {
	o := new(KernelCodeObject)

	if len(data) >= 256 && isV2V3Header(data) {
		// V2/V3 format: 256-byte header followed by instructions
		o.KernelCodeObjectMeta = parseV2V3Header(data)
		o.Data = data[256:] // Instructions start after 256-byte header
		// Since we strip the 256-byte header from Data, the entry offset is now 0
		o.KernelCodeObjectMeta.KernelCodeEntryByteOffset = 0
		o.Version = CodeObjectV3
	} else {
		// Fallback: treat entire data as instructions
		o.Data = data
		o.KernelCodeObjectMeta = new(KernelCodeObjectMeta)
		o.Version = CodeObjectV5
	}

	return o
}

// LoadKernelCodeObjectFromFS loads a kernel from an HSACO file by path.
// If kernelName is empty, auto-detects single-kernel ELFs or panics for multi-kernel.
func LoadKernelCodeObjectFromFS(filePath, kernelName string) *KernelCodeObject {
	executable, err := elf.Open(filePath)
	if err != nil {
		log.Fatal(err)
	}
	defer executable.Close()

	return loadKernelCodeObjectFromELF(executable, kernelName)
}

// LoadKernelCodeObjectFromBytes loads a kernel from embedded HSACO bytes.
// If kernelName is empty, auto-detects single-kernel ELFs or panics for multi-kernel.
func LoadKernelCodeObjectFromBytes(data []byte, kernelName string) *KernelCodeObject {
	reader := bytes.NewReader(data)
	executable, err := elf.NewFile(reader)
	if err != nil {
		log.Fatal(err)
	}

	return loadKernelCodeObjectFromELF(executable, kernelName)
}

// LoadKernelCodeObjectFromELF loads a kernel from an already-opened ELF file.
// If kernelName is empty, auto-detects single-kernel ELFs or panics for multi-kernel.
func LoadKernelCodeObjectFromELF(elfFile *elf.File, kernelName string) *KernelCodeObject {
	return loadKernelCodeObjectFromELF(elfFile, kernelName)
}

// loadKernelCodeObjectFromELF extracts a kernel from an ELF file.
// If kernelName is empty:
//   - For single-kernel ELFs: uses the only kernel
//   - For multi-kernel ELFs: panics with helpful message listing available kernels
//
//nolint:gocognit,funlen
func loadKernelCodeObjectFromELF(executable *elf.File, kernelName string) *KernelCodeObject {
	textSection := executable.Section(".text")
	if textSection == nil {
		log.Fatal(".text section not found in ELF file")
	}

	textSectionData, err := textSection.Data()
	if err != nil {
		log.Fatal(err)
	}

	// Get .rodata section for V5 kernel descriptors
	var rodataSection *elf.Section
	var rodataSectionData []byte
	rodataSection = executable.Section(".rodata")
	if rodataSection != nil {
		rodataSectionData, _ = rodataSection.Data()
	}

	// Get symbols to find kernels
	symbols, err := executable.Symbols()
	if err != nil {
		// No symbol table - treat entire .text as single kernel
		return newKernelCodeObjectFromEntireTextSection(textSectionData)
	}

	// Find kernel symbols (functions in .text section)
	var kernelSymbols []elf.Symbol
	for _, sym := range symbols {
		if sym.Section == elf.SHN_UNDEF {
			continue
		}
		if int(sym.Section) >= len(executable.Sections) {
			continue
		}
		sec := executable.Sections[sym.Section]
		if sec.Name == ".text" && sym.Size > 0 {
			kernelSymbols = append(kernelSymbols, sym)
		}
	}

	// If no kernel name specified, handle auto-detect
	if kernelName == "" {
		if len(kernelSymbols) == 0 {
			// No symbols found - use entire .text section
			return newKernelCodeObjectFromEntireTextSection(textSectionData)
		} else if len(kernelSymbols) == 1 {
			// Single kernel - use it
			kernelName = kernelSymbols[0].Name
		} else {
			// Multiple kernels - error with helpful message
			names := make([]string, len(kernelSymbols))
			for i, sym := range kernelSymbols {
				names[i] = sym.Name
			}
			log.Fatalf("multiple kernels found in ELF file, specify kernel name. Available: %v", names)
		}
	}

	// Find the specified kernel
	for _, symbol := range kernelSymbols {
		if symbol.Name == kernelName {
			// Extract kernel data using symbol offset and size
			// symbol.Value is the virtual address; textSection.Addr is the section's virtual address
			offset := symbol.Value - textSection.Addr
			kernelData := textSectionData[offset : offset+symbol.Size]
			symbolCopy := symbol

			// FIX: Check for V5 kernel descriptor FIRST, before V2/V3 detection.
			//
			// Background: V5 (AMDHSA Code Object V4+) kernels store their
			// metadata in a 64-byte kernel descriptor in the .rodata section,
			// identified by a "<kernelName>.kd" symbol. The .text section
			// contains only raw instructions with no header.
			//
			// V2/V3 kernels embed a 256-byte header at the start of .text
			// data, identified by signature bytes (CodeVersionMajor=1,
			// CodeVersionMinor<=2, MachineKind=1).
			//
			// The old code called isV2V3Header() first, which could produce
			// false positives when V5 kernel instructions happened to start
			// with bytes matching the V2/V3 signature. This caused the first
			// 256 bytes of real instructions to be stripped, corrupting the
			// kernel (see: stencil2d page fault bug).
			//
			// By checking for V5 descriptors first, we avoid the false
			// positive entirely: if a .kd symbol exists, the kernel is
			// definitively V5 and we use the raw .text data as-is.
			if v5Meta := findV5KernelDescriptor(
				kernelName, symbols, executable,
				rodataSection, rodataSectionData,
			); v5Meta != nil {
				co := new(KernelCodeObject)
				co.Data = kernelData // V5: entire kernel data is instructions
				co.KernelCodeObjectMeta = v5Meta
				co.Version = CodeObjectV5
				co.Symbol = &symbolCopy
				return co
			}

			// No V5 descriptor found — fall back to V2/V3 detection
			co := newKernelCodeObjectFromEntireTextSection(kernelData)
			co.Symbol = &symbolCopy
			return co
		}
	}

	log.Fatalf("kernel '%s' not found in ELF file", kernelName)
	return nil
}

// findV5KernelDescriptor looks for a V5 kernel descriptor (.kd symbol) in .rodata.
// Returns the parsed metadata if found, or nil if this kernel has no V5 descriptor.
func findV5KernelDescriptor(
	kernelName string,
	symbols []elf.Symbol,
	executable *elf.File,
	rodataSection *elf.Section,
	rodataSectionData []byte,
) *KernelCodeObjectMeta {
	if rodataSection == nil || rodataSectionData == nil {
		return nil
	}

	kdSymbolName := kernelName + ".kd"
	for _, sym := range symbols {
		if sym.Name == kdSymbolName && sym.Size == 64 {
			if int(sym.Section) < len(executable.Sections) {
				sec := executable.Sections[sym.Section]
				if sec.Name == ".rodata" {
					kdOffset := sym.Value - rodataSection.Addr
					if kdOffset+64 <= uint64(len(rodataSectionData)) {
						kdData := rodataSectionData[kdOffset : kdOffset+64]
						return parseV5KernelDescriptor(kdData)
					}
				}
			}
			break
		}
	}

	return nil
}

// isV2V3Header checks if data looks like a V2/V3 kernel header.
//
// This performs multi-field validation to reduce false positives.
// A real V2/V3 header has specific structure beyond just the first 10 bytes:
//   - CodeVersionMajor (offset 0-3) = 1
//   - CodeVersionMinor (offset 4-7) = 0, 1, or 2
//   - MachineKind (offset 8-9) = 1 (AMDGPU)
//   - MachineVersionMajor (offset 10-11) = known GPU generation (7, 8, 9, etc.)
//   - KernelCodeEntryByteOffset (offset 16-23) = 256 (instructions follow header)
func isV2V3Header(data []byte) bool {
	if len(data) < 256 {
		return false
	}

	codeVersionMajor := binary.LittleEndian.Uint32(data[0:4])
	codeVersionMinor := binary.LittleEndian.Uint32(data[4:8])
	machineKind := binary.LittleEndian.Uint16(data[8:10])

	// Primary signature check
	if codeVersionMajor != 1 || codeVersionMinor > 2 || machineKind != 1 {
		return false
	}

	// Secondary validation: MachineVersionMajor should be a known AMD GPU
	// generation. Valid values: 7 (GCN3/Sea Islands), 8 (GCN4/Volcanic Islands),
	// 9 (GCN5/Vega). Values outside this range indicate this is not a real header.
	machineVersionMajor := binary.LittleEndian.Uint16(data[10:12])
	if machineVersionMajor < 7 || machineVersionMajor > 9 {
		return false
	}

	// Tertiary validation: KernelCodeEntryByteOffset must be 256 for V2/V3 headers.
	// This is the offset from the start of the code object to the first instruction,
	// which is always immediately after the 256-byte header.
	kernelCodeEntryByteOffset := binary.LittleEndian.Uint64(data[16:24])
	if kernelCodeEntryByteOffset != 256 {
		return false
	}

	return true
}

// parseV2V3Header parses the 256-byte V2/V3 kernel header
func parseV2V3Header(data []byte) *KernelCodeObjectMeta {
	meta := new(KernelCodeObjectMeta)

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
func parseV5KernelDescriptor(data []byte) *KernelCodeObjectMeta {
	meta := new(KernelCodeObjectMeta)

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

	// Derive WIVgprCount and WFSgprCount from ComputePgmRsrc1.
	// These are "granulated" counts in the hardware register:
	//   bits 0-5: granulated_workitem_vgpr_count → actual = (value + 1) * 4
	//   bits 6-9: granulated_wavefront_sgpr_count → actual = (value + 1) * 8
	// The resource allocator uses WIVgprCount/WFSgprCount for register
	// allocation; leaving them at 0 causes all wavefronts to share offset 0.
	granulatedVgpr := extractBits(meta.ComputePgmRsrc1, 0, 5)
	granulatedSgpr := extractBits(meta.ComputePgmRsrc1, 6, 9)
	meta.WIVgprCount = uint16((granulatedVgpr + 1) * 4)
	meta.WFSgprCount = uint16((granulatedSgpr + 1) * 8)

	// For V5 (AMDHSA Code Object V4+) kernel descriptors:
	// The kernel_code_properties field and compute_pgm_rsrc2 may not
	// accurately reflect all SGPR setup requirements, especially for
	// kernels compiled with extern "C". Instead of relying solely on
	// the property flags, we use a practical approach:
	//
	// 1. If kernarg_size > 0, the kernel needs a kernarg segment pointer
	// 2. Workgroup ID and work-item ID enables come from compute_pgm_rsrc2
	// 3. We disable unused/deprecated SGPR features (dispatch ptr, queue ptr, etc.)
	//
	// The SGPR layout for V5 kernels is always:
	//   s[0:1] = kernarg segment pointer (if kernel has kernargs)
	//   s2     = workgroup ID X (if enabled in rsrc2)
	//   s3     = workgroup ID Y (if enabled in rsrc2)
	//   s4     = workgroup ID Z (if enabled in rsrc2)

	meta.EnableSgprPrivateSegmentBuffer = false // Deprecated in V5

	// For V5 code objects, always enable kernarg ptr if there are kernel arguments
	meta.EnableSgprKernargSegmentPtr = meta.KernargSegmentByteSize > 0

	// Disable features we don't support / that may have incorrect flags
	meta.EnableSgprDispatchPtr = false
	meta.EnableSgprQueuePtr = false
	meta.EnableSgprDispatchID = false
	meta.EnableSgprFlatScratchInit = false
	meta.EnableSgprPrivateSegmentSize = false

	// Fix compute_pgm_rsrc2: Some extern "C" kernels have incorrect
	// enable_sgpr_workgroup_id and enable_vgpr_workitem_id bits.
	// For V5 code objects, we ensure these are always enabled since
	// all HIP kernels use workgroup and work-item IDs.
	// Bit 1-5:  user_sgpr_count → set to 2 (for kernarg ptr s[0:1])
	// Bit 7:    enable_sgpr_workgroup_id_x → force enable
	// Bit 8:    enable_sgpr_workgroup_id_y → force enable
	// Bit 9:    enable_sgpr_workgroup_id_z → leave as-is
	// Bit 11-12: enable_vgpr_workitem_id → force to at least 1 (X+Y)
	rsrc2 := meta.ComputePgmRsrc2
	// Clear bit 0 (enable_sgpr_private_segment_wave_byte_offset) — deprecated in V5
	rsrc2 &^= 1
	if meta.EnableSgprKernargSegmentPtr {
		// Set user_sgpr_count to 2 (kernarg ptr uses 2 SGPRs)
		rsrc2 = (rsrc2 &^ (0x1F << 1)) | (2 << 1)
	}
	rsrc2 |= (1 << 7) // enable_sgpr_workgroup_id_x
	rsrc2 |= (1 << 8) // enable_sgpr_workgroup_id_y
	// Set enable_vgpr_workitem_id to at least 1 (enable X and Y)
	if (rsrc2>>11)&3 == 0 {
		rsrc2 = (rsrc2 &^ (3 << 11)) | (1 << 11)
	}
	meta.ComputePgmRsrc2 = rsrc2

	return meta
}

// InstructionData returns the instruction binaries in the KernelCodeObject
func (o *KernelCodeObject) InstructionData() []byte {
	return o.Data
}

// WorkItemVgprCount returns the number of VGPRs used by each work-item
func (h *KernelCodeObjectMeta) WorkItemVgprCount() uint32 {
	return extractBits(h.ComputePgmRsrc1, 0, 5)
}

// WavefrontSgprCount returns the number of SGPRs used by each wavefront
func (h *KernelCodeObjectMeta) WavefrontSgprCount() uint32 {
	return extractBits(h.ComputePgmRsrc1, 6, 9)
}

// Priority returns the priority of the kernel
func (h *KernelCodeObjectMeta) Priority() uint32 {
	return extractBits(h.ComputePgmRsrc1, 10, 11)
}

// EnableSgprPrivateSegmentWaveByteOffset enable wavebyteoffset
func (h *KernelCodeObjectMeta) EnableSgprPrivateSegmentWaveByteOffset() bool {
	return extractBits(h.ComputePgmRsrc2, 0, 0) != 0
}

// UserSgprCount returns user sgpr
func (h *KernelCodeObjectMeta) UserSgprCount() uint32 {
	return extractBits(h.ComputePgmRsrc2, 1, 5)
}

// EnableSgprWorkGroupIDX enable idx
func (h *KernelCodeObjectMeta) EnableSgprWorkGroupIDX() bool {
	return extractBits(h.ComputePgmRsrc2, 7, 7) != 0
}

// EnableSgprWorkGroupIDY enable idy
func (h *KernelCodeObjectMeta) EnableSgprWorkGroupIDY() bool {
	return extractBits(h.ComputePgmRsrc2, 8, 8) != 0
}

// EnableSgprWorkGroupIDZ enable idz
func (h *KernelCodeObjectMeta) EnableSgprWorkGroupIDZ() bool {
	return extractBits(h.ComputePgmRsrc2, 9, 9) != 0
}

// EnableSgprWorkGroupInfo enable wg info
func (h *KernelCodeObjectMeta) EnableSgprWorkGroupInfo() bool {
	return extractBits(h.ComputePgmRsrc2, 10, 10) != 0
}

// EnableVgprWorkItemID checks if the setup of the work-item is enabled
func (h *KernelCodeObjectMeta) EnableVgprWorkItemID() uint32 {
	return extractBits(h.ComputePgmRsrc2, 11, 12)
}

// EnableExceptionAddressWatch enable exception address watch
func (h *KernelCodeObjectMeta) EnableExceptionAddressWatch() bool {
	return extractBits(h.ComputePgmRsrc2, 13, 13) != 0
}

// EnableExceptionMemoryViolation enable exception memory violation
func (h *KernelCodeObjectMeta) EnableExceptionMemoryViolation() bool {
	return extractBits(h.ComputePgmRsrc2, 14, 14) != 0
}

// GetEnableSgprPrivateSegmentBuffer returns if the private segment buffer
// information needs to be written into the SGPR
func (h *KernelCodeObjectMeta) GetEnableSgprPrivateSegmentBuffer() bool {
	return h.EnableSgprPrivateSegmentBuffer
}

// GetEnableSgprDispatchPtr returns if dispatch ptr is enabled
func (h *KernelCodeObjectMeta) GetEnableSgprDispatchPtr() bool {
	return h.EnableSgprDispatchPtr
}

// GetEnableSgprQueuePtr returns if queue ptr is enabled
func (h *KernelCodeObjectMeta) GetEnableSgprQueuePtr() bool {
	return h.EnableSgprQueuePtr
}

// GetEnableSgprKernargSegmentPtr returns if kernarg segment ptr is enabled
func (h *KernelCodeObjectMeta) GetEnableSgprKernargSegmentPtr() bool {
	return h.EnableSgprKernargSegmentPtr
}

// GetEnableSgprDispatchID returns if dispatch ID is enabled
func (h *KernelCodeObjectMeta) GetEnableSgprDispatchID() bool {
	return h.EnableSgprDispatchID
}

// GetEnableSgprFlatScratchInit returns if flat scratch init is enabled
func (h *KernelCodeObjectMeta) GetEnableSgprFlatScratchInit() bool {
	return h.EnableSgprFlatScratchInit
}

// GetEnableSgprPrivateSegmentSize returns if private segment size is enabled
func (h *KernelCodeObjectMeta) GetEnableSgprPrivateSegmentSize() bool {
	return h.EnableSgprPrivateSegmentSize
}

// GetEnableSgprGridWorkgroupCountX returns if grid workgroup count X is enabled
func (h *KernelCodeObjectMeta) GetEnableSgprGridWorkgroupCountX() bool {
	return h.EnableSgprGridWorkgroupCountX
}

// GetEnableSgprGridWorkgroupCountY returns if grid workgroup count Y is enabled
func (h *KernelCodeObjectMeta) GetEnableSgprGridWorkgroupCountY() bool {
	return h.EnableSgprGridWorkgroupCountY
}

// GetEnableSgprGridWorkgroupCountZ returns if grid workgroup count Z is enabled
func (h *KernelCodeObjectMeta) GetEnableSgprGridWorkgroupCountZ() bool {
	return h.EnableSgprGridWorkgroupCountZ
}

// Info prints the human readable information that is carried by the KernelCodeObjectMeta
func (h *KernelCodeObjectMeta) Info() string {
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
