package main

import (
	"debug/elf"
	"fmt"
	"os"

	"gitlab.com/yaotsu/gcn3/insts"
)

func main() {
	filename := os.Args[1]
	elfFile, err := elf.Open(filename)
	if err != nil {
		_ = fmt.Errorf("failed to open file %v", filename)
	}
	defer elfFile.Close()

	disasm := insts.NewDisassembler()

	disasm.Disassemble(elfFile, os.Stdout)
}
