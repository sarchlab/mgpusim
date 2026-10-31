package main

import (
	"debug/elf"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

func main() {
	architecture := flag.String("arch", "gcn3", "GPU architecture: gcn3 or cdna3")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [-arch=gcn3|cdna3] file.hsaco\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	if *architecture != "gcn3" && *architecture != "cdna3" {
		fmt.Fprintf(os.Stderr, "unsupported architecture %q: expected gcn3 or cdna3\n", *architecture)
		os.Exit(2)
	}

	path := flag.Arg(0)
	elfFile, err := elf.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open %s: %v\n", path, err)
		os.Exit(1)
	}
	defer elfFile.Close()

	_, filename := filepath.Split(path)

	disasm := insts.NewDisassembler()
	disasm.IsCDNA3 = *architecture == "cdna3"

	disasm.Disassemble(elfFile, filename, os.Stdout)
}
