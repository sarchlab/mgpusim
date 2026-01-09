// Package deprecation provides deprecation warnings for legacy sample executables.
package deprecation

import (
	"fmt"
	"os"
	"path/filepath"
)

// PrintWarning prints a deprecation warning suggesting the new CLI.
func PrintWarning() {
	name := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, `
================================================================================
DEPRECATION WARNING: This standalone sample executable is deprecated.

Please use the unified CLI instead:
    mgpusim_amd run --benchmark=%s [options]

Example:
    mgpusim_amd run --benchmark=%s --sim.timing --sim.verify

The new CLI provides:
  - Interactive wizard mode (mgpusim_amd wizard)
  - YAML config file support (--config=simulation.yaml)
  - Clear flag grouping (--sim.*, --hw.*, --report.*)

This executable will be removed in a future version.
================================================================================

`, name, name)
}
