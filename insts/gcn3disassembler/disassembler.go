package main

import (
	"debug/elf"
	"fmt"
	"os"
	"path/filepath"

	"gitlab.com/yaotsu/gcn3/insts"
)

func main() {
	path := os.Args[1]
	elfFile, err := elf.Open(path)
	if err != nil {
		_ = fmt.Errorf("failed to open file %v", path)
	}
	defer elfFile.Close()

	_, filename := filepath.Split(path)
	fmt.Fprintf(os.Stdout, "\n%s:\tfile format ELF64-amdgpu\n", filename)
	fmt.Fprintf(os.Stdout, "\nDisassembly of section .text:")

	disasm := insts.NewDisassembler()

	disasm.Disassemble(elfFile, os.Stdout)
}
