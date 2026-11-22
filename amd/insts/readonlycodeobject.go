package insts

import (
	"bytes"
	"debug/elf"
	"log"
)

// LoadGFX942KernelWithReadOnlyData loads linked code and its read-only data at
// the original relative addresses. This is needed for PC-relative references
// to a constant lookup table. Writable state and dynamic relocations require
// an ELF loader with additional support.
func LoadGFX942KernelWithReadOnlyData(
	data []byte, name string, constantsByName ...ReadOnlyConstant,
) *KernelCodeObject {
	executable, err := elf.NewFile(bytes.NewReader(data))
	if err != nil {
		log.Panicf("open gfx942 code object: %v", err)
	}
	defer executable.Close()
	const gfx942Machine = 0x4c
	if executable.Machine != elf.EM_AMDGPU || executable.Type != elf.ET_DYN ||
		elfAMDGPUMach(data) != gfx942Machine {
		log.Panic("expected a linked gfx942 code object")
	}
	if executable.Section(".rela.dyn") != nil || executable.Section(".rel.dyn") != nil {
		log.Panic("gfx942 kernel with read-only data requires a fully linked image")
	}
	code := loadKernelCodeObjectFromELF(executable, name, elfAMDGPUMach(data))
	if code == nil || code.Symbol == nil {
		log.Panicf("kernel %q is unavailable", name)
	}
	section := executable.Section(".rodata")
	if section == nil || section.Flags&elf.SHF_WRITE != 0 || section.Flags&elf.SHF_ALLOC == 0 {
		log.Panicf("kernel %q is missing its read-only data", name)
	}
	constants, err := section.Data()
	if err != nil {
		log.Panicf("read gfx942 constants: %v", err)
	}
	applyReadOnlyConstants(executable, section, constants, constantsByName)
	entry := code.Symbol.Value
	base := min(section.Addr, entry)
	end := max(section.Addr+uint64(len(constants)), entry+uint64(len(code.Data)))
	if end < base || end-base > uint64(len(data))*4 {
		log.Panicf("invalid linked range for gfx942 kernel %q", name)
	}
	image := make([]byte, end-base)
	copy(image[section.Addr-base:], constants)
	copy(image[entry-base:], code.Data)
	code.Data = image
	code.KernelCodeEntryByteOffset = entry - base
	return code
}

// ReadOnlyConstant initializes a named .rodata object before a code image is
// uploaded. Data can initialize all or a leading portion of the symbol.
type ReadOnlyConstant struct {
	Name string
	Data []byte
}

func applyReadOnlyConstants(file *elf.File, section *elf.Section, data []byte, patches []ReadOnlyConstant) {
	if len(patches) == 0 {
		return
	}
	symbols, err := file.Symbols()
	if err != nil {
		log.Panicf("read gfx942 data symbols: %v", err)
	}
	for _, patch := range patches {
		var target *elf.Symbol
		for i, symbol := range symbols {
			if symbol.Name == patch.Name && elf.ST_TYPE(symbol.Info) == elf.STT_OBJECT &&
				int(symbol.Section) < len(file.Sections) && file.Sections[symbol.Section] == section {
				target = &symbols[i]
				break
			}
		}
		if target == nil || target.Value < section.Addr || uint64(len(patch.Data)) > target.Size ||
			target.Value-section.Addr > uint64(len(data)) ||
			uint64(len(patch.Data)) > uint64(len(data))-(target.Value-section.Addr) {
			log.Panicf("invalid gfx942 read-only constant %q", patch.Name)
		}
		copy(data[target.Value-section.Addr:], patch.Data)
	}
}
