package emu

import "unsafe"

// Scratchpad is a piece of pure memory that is use for the alu to store input
// and output data
type Scratchpad []byte

// AsSOP2 returns the ScratchPad as a struct representing the SOP2 scratchpad
// layout
func (sp Scratchpad) AsSOP2() *SOP2Layout {
	return (*SOP2Layout)(unsafe.Pointer(&sp[0]))
}

// AsVOP1 returns the ScratchPad as a struct representing the VOP1 scratchpad
// layout
func (sp Scratchpad) AsVop1() *VOP1Layout {
	return (*VOP1Layout)(unsafe.Pointer(&sp[0]))
}

// SOP2Layout represents the scratchpad layout for SOP2 instructions
type SOP2Layout struct {
	SRC0 uint64
	SRC1 uint64
	DST  uint64
	SCC  byte
}

// VOP1Layout represents the scratchpad layout for VOP1 instructions
type VOP1Layout struct {
	SRC0 [64]uint64
	DST  [64]uint64
}
