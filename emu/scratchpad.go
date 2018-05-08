package emu

import "unsafe"

// Scratchpad is a piece of pure memory that is use for the alu to store input
// and output data
type Scratchpad []byte

// AsSOP1 returns the ScratchPad as a struct representing the SOP1 scratchpad
// layout
func (sp Scratchpad) AsSOP1() *SOP1Layout {
	return (*SOP1Layout)(unsafe.Pointer(&sp[0]))
}

// AsSOP2 returns the ScratchPad as a struct representing the SOP2 scratchpad
// layout
func (sp Scratchpad) AsSOP2() *SOP2Layout {
	return (*SOP2Layout)(unsafe.Pointer(&sp[0]))
}

// AsSOPP returns the ScratchPad as a struct representing the SOPP scratchpad
// layout
func (sp Scratchpad) AsSOPP() *SOPPLayout {
	return (*SOPPLayout)(unsafe.Pointer(&sp[0]))
}

func (sp Scratchpad) AsSOPK() *SOPKLayout {
	return (*SOPKLayout)(unsafe.Pointer(&sp[0]))
}

// AsSOPC returns the ScratchPad as a struct representing the SOPC scratchpad
// layout
func (sp Scratchpad) AsSOPC() *SOPCLayout {
	return (*SOPCLayout)(unsafe.Pointer(&sp[0]))
}

// AsVOP1 returns the ScratchPad as a struct representing the VOP1 scratchpad
// layout
func (sp Scratchpad) AsVOP1() *VOP1Layout {
	return (*VOP1Layout)(unsafe.Pointer(&sp[0]))
}

// AsVOP2 returns the ScratchPad as a struct representing the VOP1 scratchpad
// layout
func (sp Scratchpad) AsVOP2() *VOP2Layout {
	return (*VOP2Layout)(unsafe.Pointer(&sp[0]))
}

// AsVOP3A returns the ScratchPad as a struct representing the VOP3a scratchpad
// layout
func (sp Scratchpad) AsVOP3A() *VOP3ALayout {
	return (*VOP3ALayout)(unsafe.Pointer(&sp[0]))
}

// AsVOP3B returns the ScratchPad as a struct representing the VOP3a scratchpad
// layout
func (sp Scratchpad) AsVOP3B() *VOP3BLayout {
	return (*VOP3BLayout)(unsafe.Pointer(&sp[0]))
}

// AsVOPC returns the ScratchPad as a struct representing the VOPC scratchpad
// layout
func (sp Scratchpad) AsVOPC() *VOPCLayout {
	return (*VOPCLayout)(unsafe.Pointer(&sp[0]))
}

// AsFlat returns the ScratchPad as a struct representing the Flat scratchpad
// layout
func (sp Scratchpad) AsFlat() *FlatLayout {
	return (*FlatLayout)(unsafe.Pointer(&sp[0]))
}

// AsSMEM returns the ScratchPad as a struct representing the SMEM scratchpad
// layout
func (sp Scratchpad) AsSMEM() *SMEMLayout {
	return (*SMEMLayout)(unsafe.Pointer(&sp[0]))
}

// SOP1Layout represents the scratchpad layout for SOP1 instructions
type SOP1Layout struct {
	SRC0 uint64
	DST  uint64
	EXEC uint64
	SCC  byte
	PC   uint64
}

// SOP2Layout represents the scratchpad layout for SOP2 instructions
type SOP2Layout struct {
	SRC0 uint64
	SRC1 uint64
	DST  uint64
	SCC  byte
}

// SOPCLayout represents the scratchpad layout for SOPC instructions
type SOPCLayout struct {
	SRC0 uint64
	SRC1 uint64
	SCC  byte
}

// SMEMLayout reqpresents the scratchpad layout for SMEM instructions
type SMEMLayout struct {
	DATA   [4]uint32  // 0:16
	Offset uint64     // 16:24
	Base   uint64     // 24:32
	DST    [16]uint32 // 32:96
}

// SOPPLayout reqpresents the scratchpad layout for SOPP instructions
type SOPPLayout struct {
	EXEC uint64
	PC   uint64
	IMM  uint64
	SCC  byte
	VCC  uint64
}

//SOPKLayout represents the scratchpad layout for SOPK instructions
type SOPKLayout struct {
	DST uint64
	IMM uint64
	SCC byte
}

// VOP1Layout represents the scratchpad layout for VOP1 instructions
type VOP1Layout struct {
	EXEC uint64
	DST  [64]uint64
	VCC  uint64
	SRC0 [64]uint64
}

// VOP2Layout represents the scratchpad layout for VOP2 instructions
type VOP2Layout struct {
	EXEC uint64
	DST  [64]uint64
	VCC  uint64
	SRC0 [64]uint64
	SRC1 [64]uint64
}

// VOP3ALayout represents the scratchpad layout for VOP3a instructions
type VOP3ALayout struct {
	EXEC uint64
	DST  [64]uint64
	VCC  uint64
	SRC0 [64]uint64
	SRC1 [64]uint64
	SRC2 [64]uint64
}

// VOP3BLayout represents the scratchpad layout for VOP3a instructions
type VOP3BLayout struct {
	EXEC uint64
	DST  [64]uint64
	VCC  uint64
	SRC0 [64]uint64
	SRC1 [64]uint64
	SRC2 [64]uint64
	SDST uint64
}

// VOPCLayout represents the scratchpad layout for the VOPC instructions
type VOPCLayout struct {
	EXEC uint64
	VCC  uint64
	SRC0 [64]uint64
	SRC1 [64]uint64
}

// FlatLayout represents the scratchpad layout for Flat instructions
type FlatLayout struct {
	EXEC uint64
	ADDR [64]uint64
	DATA [256]uint32 // 256 to consider the X4 instructions
	DST  [256]uint32
}
