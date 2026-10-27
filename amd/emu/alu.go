package emu

// ALU defines the interface for architecture-specific ALU implementations.
type ALU interface {
	// Run borrows state for this call only; implementations must not retain it.
	Run(state InstEmuState)
	SetLDS(lds []byte)
	LDS() []byte
	ArchName() string // Returns "GCN3" or "CDNA3"
}
