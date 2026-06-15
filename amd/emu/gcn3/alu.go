// Package gcn3 provides the GCN3 (gfx803) ALU implementation.
package gcn3

import "github.com/sarchlab/mgpusim/v4/amd/emu"

// ALU is the GCN3 (gfx803) ALU implementation.
// This is a type alias to the existing implementation in the emu package.
type ALU = emu.ALUImpl

// NewALU creates a new GCN3 ALU instance.
func NewALU(storageAccessor emu.StorageAccessor) *ALU {
	return emu.NewALU(storageAccessor)
}
