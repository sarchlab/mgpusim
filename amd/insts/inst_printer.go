package insts

import (
	"debug/elf"
	"fmt"
	"strings"
)

// InstPrinter formats instructions as strings with optional symbol resolution.
type InstPrinter struct {
	file *elf.File
}

// NewInstPrinter creates an InstPrinter with an optional ELF file for symbol resolution.
func NewInstPrinter(file *elf.File) *InstPrinter {
	return &InstPrinter{file: file}
}

// Print returns the disassembly string for an instruction.
//
//nolint:gocyclo
func (p *InstPrinter) Print(i *Inst) string {
	switch i.FormatType {
	case SOP2:
		return p.sop2String(i)
	case SMEM:
		return p.smemString(i)
	case VOP1:
		return p.vop1String(i)
	case VOP2:
		return p.vop2String(i)
	case FLAT:
		return p.flatString(i)
	case SOPP:
		return p.soppString(i)
	case VOPC:
		return p.vopcString(i)
	case SOPC:
		return p.sopcString(i)
	case VOP3a:
		return p.vop3aString(i)
	case VOP3b:
		return p.vop3bString(i)
	case SOP1:
		return p.sop1String(i)
	case SOPK:
		return p.sopkString(i)
	case DS:
		return p.dsString(i)
	default:
		return i.InstName
	}
}

func (p *InstPrinter) sop2String(i *Inst) string {
	return i.InstName + " " +
		i.Dst.String() + ", " +
		i.Src0.String() + ", " +
		i.Src1.String()
}

func (p *InstPrinter) vop1String(i *Inst) string {
	return i.InstName + " " +
		i.Dst.String() + ", " +
		i.Src0.String()
}

func (p *InstPrinter) flatString(i *Inst) string {
	var s string
	instName := i.InstName

	// Check if SADDR is 0x7F (OFF) - indicates global addressing
	isGlobal := i.SAddr != nil && i.SAddr.IntValue == 0x7F
	if isGlobal {
		instName = strings.Replace(instName, "flat_", "global_", 1)
	}

	if i.Opcode >= 16 && i.Opcode <= 23 {
		s = instName + " " + i.Dst.String() + ", " + i.Addr.String()
		if isGlobal {
			s += ", off"
		}
	} else if i.Opcode >= 24 && i.Opcode <= 31 {
		s = instName + " " + i.Addr.String() + ", " + i.Data.String()
		if isGlobal {
			s += ", off"
		}
	}
	return s
}

func (p *InstPrinter) smemString(i *Inst) string {
	s := fmt.Sprintf("%s %s, %s, %#x",
		i.InstName, i.Data.String(), i.Base.String(), uint16(i.Offset.IntValue))
	return s
}

func (p *InstPrinter) soppString(i *Inst) string {
	operandStr := ""
	if i.Opcode == 12 { // S_WAITCNT
		operandStr = p.waitcntOperandString(i)
	} else if i.Opcode >= 2 && i.Opcode <= 9 { // Branch
		// For branches, just print the immediate value
		// Symbol annotation is handled separately by BranchTargetAnnotation()
		operandStr = " " + i.SImm16.String()
	} else if i.Opcode == 1 || i.Opcode == 10 {
		// Does not print anything
	} else {
		operandStr = " " + i.SImm16.String()
	}
	s := i.InstName + operandStr
	return s
}

// BranchTargetAnnotation returns the symbol annotation for branch instructions.
// Returns empty string if not a branch or no symbol found.
func (p *InstPrinter) BranchTargetAnnotation(i *Inst) string {
	if i.FormatType != SOPP {
		return ""
	}

	// Only branch instructions (opcodes 2-9)
	if i.Opcode < 2 || i.Opcode > 9 {
		return ""
	}

	if p.file == nil {
		return ""
	}

	imm := int16(uint16(i.SImm16.IntValue))
	target := i.PC + uint64(imm*4) + 4
	symbols, _ := p.file.Symbols()

	// First try exact symbol match
	for _, symbol := range symbols {
		if symbol.Value == target {
			return fmt.Sprintf("<%s>", symbol.Name)
		}
	}

	// Try to find containing symbol for relative offset
	for _, symbol := range symbols {
		if symbol.Size > 0 && target >= symbol.Value && target < symbol.Value+symbol.Size {
			offset := target - symbol.Value
			return fmt.Sprintf("<%s+0x%x>", symbol.Name, offset)
		}
	}

	return ""
}

func (p *InstPrinter) waitcntOperandString(i *Inst) string {
	operandStr := ""
	if i.VMCNT != 15 {
		operandStr += fmt.Sprintf(" vmcnt(%d)", i.VMCNT)
	}

	if i.LKGMCNT != 15 {
		operandStr += fmt.Sprintf(" lgkmcnt(%d)", i.LKGMCNT)
	}
	return operandStr
}

func (p *InstPrinter) vop2String(i *Inst) string {
	s := fmt.Sprintf("%s %s", i.InstName, i.Dst.String())

	switch i.Opcode {
	case 25, 26, 27, 28, 29, 30:
		s += ", vcc"
	}

	s += fmt.Sprintf(", %s, %s", i.Src0.String(), i.Src1.String())

	switch i.Opcode {
	case 0, 28, 29:
		s += ", vcc"
	case 24, 37: // madak
		s += ", " + i.Src2.String()
	}

	if i.IsSdwa {
		s = strings.ReplaceAll(s, "_e32", "_sdwa")
		s += p.sdwaVOP2String(i)
	}

	return s
}

func (p *InstPrinter) sdwaVOP2String(i *Inst) string {
	s := ""

	s += " dst_sel:"
	s += sdwaSelectString(i.DstSel)
	s += " dst_unused:"
	s += sdwaUnusedString(i.DstUnused)
	s += " src0_sel:"
	s += sdwaSelectString(i.Src0Sel)
	s += " src1_sel:"
	s += sdwaSelectString(i.Src1Sel)

	return s
}

func (p *InstPrinter) vopcString(i *Inst) string {
	dst := "vcc"
	if strings.Contains(i.InstName, "cmpx") {
		dst = "exec"
	}

	return fmt.Sprintf("%s %s, %s, %s",
		i.InstName, dst, i.Src0.String(), i.Src1.String())
}

func (p *InstPrinter) sopcString(i *Inst) string {
	return fmt.Sprintf("%s %s, %s",
		i.InstName, i.Src0.String(), i.Src1.String())
}

func (p *InstPrinter) vop3aString(i *Inst) string {
	s := fmt.Sprintf("%s %s",
		i.InstName, i.Dst.String())

	s += ", " + p.vop3aInputOperandString(i.Src0,
		i.Src0Neg,
		i.Src0Abs)

	s += ", " + p.vop3aInputOperandString(i.Src1,
		i.Src1Neg,
		i.Src1Abs)

	if i.Src2 == nil {
		return s
	}

	s += ", " + p.vop3aInputOperandString(i.Src2,
		i.Src2Neg,
		i.Src2Abs)

	return s
}

func (p *InstPrinter) vop3aInputOperandString(operand *Operand, neg, abs bool) string {
	s := ""

	if neg {
		s += "-"
	}

	if abs {
		s += "|"
	}

	s += operand.String()

	if abs {
		s += "|"
	}

	return s
}

func (p *InstPrinter) vop3bString(i *Inst) string {
	s := i.InstName + " "

	if i.Dst != nil {
		s += i.Dst.String() + ", "
	}

	s += fmt.Sprintf("%s, %s, %s",
		i.SDst.String(),
		i.Src0.String(),
		i.Src1.String(),
	)

	if i.Opcode != 281 && i.Src2 != nil {
		s += ", " + i.Src2.String()
	}

	return s
}

func (p *InstPrinter) sop1String(i *Inst) string {
	return fmt.Sprintf("%s %s, %s", i.InstName, i.Dst.String(), i.Src0.String())
}

func (p *InstPrinter) sopkString(i *Inst) string {
	s := fmt.Sprintf("%s %s, 0x%x",
		i.InstName, i.Dst.String(), i.SImm16.IntValue)

	return s
}

func (p *InstPrinter) dsString(i *Inst) string {
	s := i.InstName + " "
	switch i.Opcode {
	case 54, 55, 56, 57, 58, 59, 60, 118, 119, 120, 254, 255:
		s += i.Dst.String() + ", "
	}

	s += i.Addr.String()

	if i.SRC0Width > 0 {
		s += ", " + i.Data.String()
	}

	if i.SRC1Width > 0 {
		s += ", " + i.Data1.String()
	}

	switch i.Opcode {
	case 13, 54, 254, 255:
		if i.Offset0 > 0 {
			s += fmt.Sprintf(" offset:%d", i.Offset0)
		}
	default:
		if i.Offset0 > 0 {
			s += fmt.Sprintf(" offset0:%d", i.Offset0)
		}

		if i.Offset1 > 0 {
			s += fmt.Sprintf(" offset1:%d", i.Offset1)
		}
	}

	return s
}
