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

	disasm := insts.NewDisassembler()

	disasm.Disassemble(elfFile, filename, os.Stdout)
}
