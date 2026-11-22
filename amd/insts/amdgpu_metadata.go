package insts

import (
	"debug/elf"
	"encoding/binary"
	"strings"

	"github.com/vmihailenco/msgpack/v5"
)

// NT_AMDGPU_METADATA contains the AMDHSA metadata map encoded as MessagePack.
const amdGPUMetadataNote = 32

type amdGPUKernelMetadata struct {
	Name string `msgpack:".name"`
	SGPR uint16 `msgpack:".sgpr_count"`
	VGPR uint16 `msgpack:".vgpr_count"`
}

type amdGPUCodeObjectMetadata struct {
	Target  string                 `msgpack:"amdhsa.target"`
	Kernels []amdGPUKernelMetadata `msgpack:"amdhsa.kernels"`
}

func overrideRegisterCountsFromNotes(meta *KernelCodeObjectMeta, name string, file *elf.File, machID uint32) {
	const efAMDGPUMachGFX942 = 0x4c
	if file.Machine != elf.EM_AMDGPU || file.Type != elf.ET_DYN || (machID != 0 && machID != efAMDGPUMachGFX942) {
		return
	}

	for _, section := range file.Sections {
		if section.Type != elf.SHT_NOTE {
			continue
		}

		data, err := section.Data()
		if err != nil {
			continue
		}

		note := amdGPUCodeObjectMetadataInNotes(data, file.ByteOrder)
		if note == nil {
			continue
		}
		// The opened ELF API cannot recover e_flags. The metadata target
		// identifies gfx942 even if the original ELF header is unavailable.
		target, _, _ := strings.Cut(note.Target, ":")
		if machID == 0 && !strings.HasSuffix(target, "-gfx942") {
			continue
		}
		for _, kernel := range note.Kernels {
			if kernel.Name == name {
				applyAMDGPUMetadataCounts(meta, kernel)
				return
			}
		}
	}
}

func amdGPUCodeObjectMetadataInNotes(data []byte, order binary.ByteOrder) *amdGPUCodeObjectMetadata {
	for len(data) >= 12 {
		// An ELF note is three uint32 fields followed by the name and
		// descriptor, each padded to a four-byte boundary.
		nameSize := uint64(order.Uint32(data))
		descSize := uint64(order.Uint32(data[4:]))
		noteType := order.Uint32(data[8:])
		data = data[12:]
		descStart := (nameSize + 3) &^ 3
		paddedDescSize := (descSize + 3) &^ 3
		nextNote := descStart + paddedDescSize
		if nextNote > uint64(len(data)) {
			break
		}
		if noteType == amdGPUMetadataNote && string(data[:nameSize]) == "AMDGPU\x00" {
			descriptor := data[descStart : descStart+descSize]
			var metadata amdGPUCodeObjectMetadata
			if err := msgpack.Unmarshal(descriptor, &metadata); err == nil {
				return &metadata
			}
		}
		data = data[nextNote:]
	}

	return nil
}

func applyAMDGPUMetadataCounts(meta *KernelCodeObjectMeta, kernel amdGPUKernelMetadata) {
	// The note's SGPR total already includes reserved registers such as VCC,
	// unlike the assembler's numbered_sgpr symbol. Only round for allocation.
	sgprs := (uint32(kernel.SGPR) + 7) &^ 7
	vgprs := (uint32(kernel.VGPR) + 3) &^ 3
	if sgprs <= 128 {
		meta.WFSgprCount = max(meta.WFSgprCount, uint16(sgprs))
	}
	if vgprs <= 512 {
		meta.WIVgprCount = max(meta.WIVgprCount, uint16(vgprs))
	}
}
