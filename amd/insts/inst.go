package insts

import (
	"debug/elf"
)

// ExeUnit defines which execution unit should execute the instruction
type ExeUnit int

// Defines all possible execution units
const (
	ExeUnitVALU ExeUnit = iota
	ExeUnitScalar
	ExeUnitVMem
	ExeUnitBranch
	ExeUnitLDS
	ExeUnitGDS
	ExeUnitSpecial
)

// A InstType represents an instruction type. For example s_barrier instruction
// is a instruction type
type InstType struct {
	InstName  string
	Opcode    Opcode
	Format    *Format
	ID        int
	ExeUnit   ExeUnit
	DSTWidth  int
	SRC0Width int
	SRC1Width int
	SRC2Width int
	SDSTWidth int
}

// An Inst is a GCN3 instruction
type Inst struct {
	*Format
	*InstType
	ByteSize int
	PC       uint64

	Src0 *Operand
	Src1 *Operand
	Src2 *Operand
	Dst  *Operand
	SDst *Operand // For VOP3b

	Addr   *Operand
	Data   *Operand
	Data1  *Operand
	Base   *Operand
	Offset *Operand
	SImm16 *Operand
	SAddr  *Operand // FLAT scalar address (0x7F = OFF for global addressing)

	Abs                 int
	Omod                int
	Neg                 int
	Offset0             uint32
	Offset1             uint32
	SystemLevelCoherent bool
	GlobalLevelCoherent bool
	TextureFailEnable   bool
	Imm                 bool
	Clamp               bool
	GDS                 bool
	VMCNT               int
	LKGMCNT             int

	//Fields for SDWA extensions
	IsSdwa    bool
	DstSel    SDWASelect
	DstUnused SDWAUnused
	Src0Sel   SDWASelect
	Src0Sext  bool
	Src0Neg   bool
	Src0Abs   bool
	Src1Sel   SDWASelect
	Src1Sext  bool
	Src1Neg   bool
	Src1Abs   bool
	Src2Neg   bool
	Src2Abs   bool
}

// NewInst creates a zero-filled instruction
func NewInst() *Inst {
	i := new(Inst)
	i.Format = new(Format)
	i.InstType = new(InstType)
	return i
}

// String returns the disassembly of an instruction.
// This is a convenience wrapper that uses InstPrinter.
func (i Inst) String(file *elf.File) string {
	printer := NewInstPrinter(file)
	return printer.Print(&i)
}
