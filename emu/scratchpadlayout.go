package emu

// SOP2Layout represents the scratchpad layout for SOP2 instructions
type SOP2Layout struct {
	SRC0 uint64
	SRC1 uint64
	DST  uint64
	SCC  byte
}
