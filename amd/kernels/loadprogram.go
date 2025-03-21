package kernels

import (
	"bytes"
	"debug/elf"
	"log"
	"regexp"

	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

func parseNote(
	file *elf.File,
) CodeObjectVersion {
	noteSection := file.Section(".note")
	if noteSection == nil {
		panic(".note section is not found")
	}

	note, err := noteSection.Data()
	if err != nil {
		panic("cannot read note")
	}

	// determine the Code Object Version
	// if the note contains "amdhsa.version", then it is version 3 or later
	// otherwise, it is version 2
	amdhsaRegEx := regexp.MustCompile(`amdhsa.version`)
	amdhsaStr := string(amdhsaRegEx.Find(note))
	if amdhsaStr == "" {
		return CodeObjectVersionV2
	}

	return parseFromCodeObjectVersionV3ToV5(note)
}

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
			hsaco.Symbol = &symbol

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

	codeObjectVersion := parseNote(executable)

	switch codeObjectVersion {
	case UndefinedCodeObjectVersion:
		log.Panic("unsupported Code Object Version")
	case CodeObjectVersionV2:
		return addCodeObjectV2(data, kernelName, executable)
	case CodeObjectVersionV3:
		log.Panicf("unimplemented Code Object Version: %d", codeObjectVersion)
	case CodeObjectVersionV4:
		return addCodeObjectV4(data, kernelName, executable)
	case CodeObjectVersionV5:
		log.Panicf("unimplemented Code Object Version: %d", codeObjectVersion)
	default:
		log.Panicf("unsupported Code Object Version: %d", codeObjectVersion)
	}

	return nil
}
