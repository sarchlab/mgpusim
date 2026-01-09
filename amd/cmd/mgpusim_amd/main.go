// Package main is the entry point for the MGPUSim AMD CLI.
package main

import (
	"os"

	"github.com/sarchlab/mgpusim/v4/amd/cli/commands"
)

func main() {
	if err := commands.Execute(); err != nil {
		os.Exit(1)
	}
}
