package disasm

import (
	"debug/elf"
	"fmt"
	"io"
)

// Disassembler is the unit that can decode .hsaco file
type Disassembler struct {
}

// Disassemble take a binary file as an input and put the assembly code in a
// write
func (d *Disassembler) Disassemble(file *elf.File, w io.Writer) {
	sections := file.Sections

	for _, sec := range sections {
		if sec.Name == ".text" {
			data, _ := sec.Data()
			co := NewHsaCo(data)
			fmt.Println(
				len(co.Data),
				co.CodeVersionMajor(),
				co.KernelCodeEntryByteOffset(),
				co.RuntimeLoaderKernelSymbol())

			/*
				id := co.InstructionData()
				i := 0
				for _, b := range id {
					fmt.Printf("%08b ", b)
					i++
					if i == 8 {
						i = 0
						fmt.Printf("\n")
					}
				}
			*/
		}
	}
}
