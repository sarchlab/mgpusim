package kernels

import (
	"bytes"
	"debug/elf"
	"log"

	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// LoadProgram loads program
func LoadProgram(filePath, kernelName string) *insts.HsaCo {
	executable, err := elf.Open(filePath)
	if err != nil {
		log.Fatal(err)
	}

	return loadProgramFromELF(executable, kernelName)
}

// LoadProgramFromMemory loads program
func LoadProgramFromMemory(data []byte, kernelName string) *insts.HsaCo {
	reader := bytes.NewReader(data)
	executable, err := elf.NewFile(reader)
	if err != nil {
		log.Fatal(err)
	}

	return loadProgramFromELF(executable, kernelName)
}

func loadProgramFromELF(executable *elf.File, kernelName string) *insts.HsaCo {
	hsaco := insts.NewHsaCoFromELF(executable)
	if hsaco == nil {
		log.Fatal("Failed to load HSACO from ELF")
	}

	// If no kernel name specified, return the first/only kernel
	if kernelName == "" {
		return hsaco
	}

	// Find the symbol for the specified kernel
	symbols, err := executable.Symbols()
	if err != nil {
		log.Fatal(err)
	}

	for _, symbol := range symbols {
		if symbol.Name == kernelName {
			symbolCopy := symbol
			hsaco.Symbol = &symbolCopy
			return hsaco
		}
	}

	return nil
}
