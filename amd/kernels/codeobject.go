package kernels

import (
	"debug/elf"
	"fmt"
	"log"
	"strings"

	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	// CodeObjectVersionV2 represents the version of the code object.
	UndefinedCodeObjectVersion = 0
	CodeObjectVersionV2        = 2
	CodeObjectVersionV3        = 3
	CodeObjectVersionV4        = 4
	CodeObjectVersionV5        = 5
)

type CodeObjectVersion int

// AMDHSAVersion represents the version of the AMD HSA code object.

type AMDHSAVersion struct {
	Major uint32
	Minor uint32
}

func parseFromCodeObjectVersionV3ToV5(
	note []byte,
) (version CodeObjectVersion) {
	kernelsIndex := strings.Index(string(note), "amdhsa.kernels")
	if kernelsIndex == -1 {
		panic("amdhsa.kernels not found in note section")
	}

	var metadata map[string]interface{}
	err := msgpack.Unmarshal(note[kernelsIndex-2:], &metadata)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal note section: %v", err))
	}

	amdHSAVersion := AMDHSAVersion{
		Major: uint32(metadata["amdhsa.version"].([]interface{})[0].(int8)),
		Minor: uint32(metadata["amdhsa.version"].([]interface{})[1].(int8)),
	}

	if amdHSAVersion.Major == 1 && amdHSAVersion.Minor == 1 {
		return CodeObjectVersionV4
	} else {
		log.Panicf("unsupported AMD HSA version: %d.%d", amdHSAVersion.Major, amdHSAVersion.Minor)
	}

	return UndefinedCodeObjectVersion
}

func addCodeObjectV2(
	data []byte,
	kernelName string,
	executable *elf.File,
) *insts.HsaCo {
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
			symbolCopy := symbol
			hsaco.Symbol = &symbolCopy

			// fmt.Println(hsaco.Info())

			return hsaco
		}
	}

	return nil
}

func addCodeObjectV4(
	data []byte,
	kernelName string,
	executable *elf.File,
) *insts.HsaCo {
	symbols, err := executable.Symbols()
	if err != nil {
		log.Fatal(err)
	}

	rodataSection := executable.Section(".rodata")
	if rodataSection == nil {
		panic("cannot find rodata sec")
	}

	rodataSecData, err := rodataSection.Data()
	if err != nil {
		panic(err)
	}

	textSection := executable.Section(".text")
	if textSection == nil {
		panic("cannot find text sec")
	}

	noteSection := executable.Section(".note")
	if noteSection == nil {
		panic("cannot find note sec")
	}

	noteSecData, err := noteSection.Data()
	if err != nil {
		panic(err)
	}

	kernelsIndex := strings.Index(string(noteSecData), "amdhsa.kernels")
	if kernelsIndex == -1 {
		panic("amdhsa.kernels not found in note section")
	}

	var metadata map[string]interface{}
	err = msgpack.Unmarshal(noteSecData[kernelsIndex-2:], &metadata)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal note section: %v", err))
	}

	// An empty kernel name is for the case where the symbol is not generated.
	// Use the whole text section in this case.
	if kernelName == "" {
		panic("kernel name must be specified in the case of CodeObjectVersionV4")
	}

	for _, descriptorSymbol := range symbols {
		if descriptorSymbol.Name == kernelName+".kd" {
			descriptorOffset := descriptorSymbol.Value - rodataSection.Offset
			kdData := rodataSecData[descriptorOffset : descriptorOffset+descriptorSymbol.Size]

			hsacoHeader := insts.NewHsacoHeader(kernelName, kdData, metadata)

			hsacoHeader.CodeVersionMajor = 1
			hsacoHeader.CodeVersionMinor = 1

			for _, kernelSymbol := range symbols {
				if kernelSymbol.Name == kernelName {
					offset := kernelSymbol.Value - textSection.Addr + textSection.Offset

					kernelData := data[offset : offset+kernelSymbol.Size]

					hsaco := insts.NewHsaCoFromHeader(kernelData, hsacoHeader)
					symbolCopy := kernelSymbol
					hsaco.Symbol = &symbolCopy

					// fmt.Println(hsaco.Info())

					// Unlike CodeObjectVersionV2, we directly move instruction data
					// to the beginning of the code object.
					hsaco.KernelCodeEntryByteOffset = 0

					return hsaco
				}
			}
		}
	}

	return nil
}
