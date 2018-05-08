package kernels

import (
	"debug/elf"
	"log"

	"gitlab.com/yaotsu/gcn3/insts"
)

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
