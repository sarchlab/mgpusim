package kernels

import (
	"bytes"
	"debug/elf"
	"log"

	"gitlab.com/akita/mgpusim/v2/insts"
)

// LoadProgram loads program
func LoadProgram(filePath, kernelName string) *insts.HsaCo {
	executable, err := elf.Open(filePath)
	if err != nil {
		log.Fatal(err)
	}

	symbols, err := executable.Symbols()
	if err != nil {
		log.Fatal(err)
	}

	textSection := executable.Section(".text")
	if textSection == nil {
		log.Fatal(".text section is not found")
	}

	textSectionData, err := textSection.Data()
	if err != nil {
		log.Fatal(err)
	}

	// An empty kernel name is for the case where the symbol is not generated.
	// Use the whole text section in this case.
	if kernelName == "" {
		hsaco := insts.NewHsaCoFromData(textSectionData)
		return hsaco
	}

	for _, symbol := range symbols {
		if symbol.Name == kernelName {
			offset := symbol.Value - textSection.Offset
			hsacoData := textSectionData[offset : offset+symbol.Size]
			hsaco := insts.NewHsaCoFromData(hsacoData)

			//fmt.Println(hsaco.Info())

			return hsaco
		}
	}

	return nil
}

// LoadProgramFromMemory loads program
func LoadProgramFromMemory(data []byte, kernelName string) *insts.HsaCo {
	reader := bytes.NewReader(data)
	executable, err := elf.NewFile(reader)
	if err != nil {
		log.Fatal(err)
	}

	symbols, err := executable.Symbols()
	if err != nil {
		log.Fatal(err)
	}

	textSection := executable.Section(".text")
	if textSection == nil {
		log.Fatal(".text section is not found")
	}

	textSectionData, err := textSection.Data()
	if err != nil {
		log.Fatal(err)
	}

	// An empty kernel name is for the case where the symbol is not generated.
	// Use the whole text section in this case.
	if kernelName == "" {
		hsaco := insts.NewHsaCoFromData(textSectionData)
		return hsaco
	}

	for _, symbol := range symbols {
		if symbol.Name == kernelName {
			offset := symbol.Value - textSection.Offset
			hsacoData := textSectionData[offset : offset+symbol.Size]
			hsaco := insts.NewHsaCoFromData(hsacoData)

			//fmt.Println(hsaco.Info())

			return hsaco
		}
	}

	return nil
}
