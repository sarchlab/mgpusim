package insts

import (
	"debug/elf"
	"encoding/binary"

	"github.com/vmihailenco/msgpack/v5"
)

// NT_AMDGPU_METADATA contains the AMDHSA metadata map encoded as MessagePack.
const amdGPUMetadataNote = 32

type amdGPUKernelMetadata struct {
	Name string `msgpack:".name"`
	SGPR uint16 `msgpack:".sgpr_count"`
	VGPR uint16 `msgpack:".vgpr_count"`
}

func overrideRegisterCountsFromNotes(meta *KernelCodeObjectMeta, name string, file *elf.File) {
	for _, section := range file.Sections {
		if section.Type != elf.SHT_NOTE {
			continue
		}

		data, err := section.Data()
		if err != nil {
			continue
		}

		for _, kernel := range amdGPUKernelsInNotes(data, file.ByteOrder) {
			if kernel.Name == name {
				applyAMDGPUMetadataCounts(meta, kernel)
			}
		}
	}
}

func amdGPUKernelsInNotes(data []byte, order binary.ByteOrder) []amdGPUKernelMetadata {
	var metadata struct {
		Kernels []amdGPUKernelMetadata `msgpack:"amdhsa.kernels"`
	}

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
			if err := msgpack.Unmarshal(descriptor, &metadata); err == nil {
				return metadata.Kernels
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
