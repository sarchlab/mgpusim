package insts

import (
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

func extractBits(number uint32, lo uint8, hi uint8) uint32 {
	var mask uint32
	var extracted uint32
	mask = ((1 << (hi - lo + 1)) - 1) << lo
	extracted = (number & mask) >> lo
	return extracted
}

func (f *Format) retrieveOpcode(firstFourBytes uint32) Opcode {
	var opcode uint32
	opcode = extractBits(firstFourBytes, f.OpcodeLow, f.OpcodeHigh)
	return Opcode(opcode)
}

// Opcode is the opcode of a GCN3 Instruction
type Opcode uint16

type decodeTable struct {
	insts map[Opcode]*InstType
}

func newDecodeTable() *decodeTable {
	return &decodeTable{make(map[Opcode]*InstType)}
}

// Disassembler is the unit that can decode .hsaco file
type Disassembler struct {
	formatList []*Format

	// Maps from the format to table
	decodeTables map[FormatType]*decodeTable
	nextInstID   int
}

func (d *Disassembler) addInstType(info *InstType) {
	if d.decodeTables[info.Format.FormatType] == nil {
		d.decodeTables[info.Format.FormatType] = newDecodeTable()
	}
	d.decodeTables[info.Format.FormatType].insts[info.Opcode] = info
	info.ID = d.nextInstID
	d.nextInstID++
}

// NewDisassembler creates a new disassembler
func NewDisassembler() *Disassembler {
	d := new(Disassembler)

	d.nextInstID = 0

	d.initFormatList()
	d.initializeDecodeTable()

	return d
}

func (d *Disassembler) matchFormat(firstTwoBytes uint16) (*Format, error) {
	for _, f := range d.formatList {
		if (firstTwoBytes^f.Encoding)&f.Mask == 0 {
			return f, nil
		}
	}
	bytesString := fmt.Sprintf("%04x", firstTwoBytes)
	return nil, errors.New("cannot find the instruction format, first two " +
		"bytes are " + bytesString)
}

func (d *Disassembler) lookUp(format *Format, opcode Opcode) (*InstType, error) {
	if d.decodeTables[format.FormatType] != nil &&
		d.decodeTables[format.FormatType].insts[opcode] != nil {
		return d.decodeTables[format.FormatType].insts[opcode], nil
	}

	return nil, fmt.Errorf("Instruction format %s, opcode %d not found",
		format.FormatName, opcode)
}

func (d *Disassembler) decodeSop2(inst *Inst, buf []byte) {
	bytes := binary.LittleEndian.Uint32(buf)

	src0Value := extractBits(bytes, 0, 7)
	inst.Src0, _ = getOperand(uint16(src0Value))
	if inst.Src0.OperandType == LiteralConstant {
		inst.ByteSize += 4
		inst.Src0.LiteralConstant = BytesToUint32(buf[4:8])
	}

	src1Value := extractBits(bytes, 8, 15)
	inst.Src1, _ = getOperand(uint16(src1Value))
	if inst.Src1.OperandType == LiteralConstant {
		inst.ByteSize += 4
		inst.Src1.LiteralConstant = BytesToUint32(buf[4:8])
	}

	sdstValue := extractBits(bytes, 16, 22)
	inst.Dst, _ = getOperand(uint16(sdstValue))

	if strings.Contains(inst.InstName, "64") {
		inst.Src0.RegCount = 2
		inst.Src1.RegCount = 2
		inst.Dst.RegCount = 2
	}
}

func (d *Disassembler) decodeVop1(inst *Inst, buf []byte) {
	bytes := binary.LittleEndian.Uint32(buf)

	src0Value := extractBits(bytes, 0, 8)
	inst.Src0, _ = getOperand(uint16(src0Value))
	if inst.Src0.OperandType == LiteralConstant {
		inst.ByteSize += 4
		inst.Src0.LiteralConstant = BytesToUint32(buf[4:8])
	}

	dstValue := extractBits(bytes, 17, 24)
	if inst.Opcode == 2 {
		// v_readfirstlane_b32
		inst.Dst, _ = getOperand(uint16(dstValue))
	} else {
		inst.Dst, _ = getOperand(uint16(dstValue + 256))
	}
}

func (d *Disassembler) decodeVop2(inst *Inst, buf []byte) {
	bytes := binary.LittleEndian.Uint32(buf)
	fmt.Printf("%s\n", inst.InstName)
	src0Bits := extractBits(bytes, 0, 8)
	inst.Src0, _ = getOperand(uint16(src0Bits))
	if inst.Src0.OperandType == LiteralConstant {
		inst.ByteSize += 4
		inst.Src0.LiteralConstant = BytesToUint32(buf[4:8])
	}

	bits := int(extractBits(bytes, 9, 16))
	inst.Src1 = NewVRegOperand(bits, bits, 0)
	bits = int(extractBits(bytes, 17, 24))
	inst.Dst = NewVRegOperand(bits, bits, 0)
}

func (d *Disassembler) decodeFlat(inst *Inst, buf []byte) {
	bytesLo := binary.LittleEndian.Uint32(buf)
	bytesHi := binary.LittleEndian.Uint32(buf[4:])

	if extractBits(bytesLo, 17, 17) != 0 {
		inst.SystemLevelCoherent = true
	}

	if extractBits(bytesLo, 16, 16) != 0 {
		inst.GlobalLevelCoherent = true
	}

	if extractBits(bytesHi, 23, 23) != 0 {
		inst.TextureFailEnable = true
	}

	bits := int(extractBits(bytesHi, 0, 7))
	inst.Addr = NewVRegOperand(bits, bits, 2)
	bits = int(extractBits(bytesHi, 24, 31))
	inst.Dst = NewVRegOperand(bits, bits, 0)
	bits = int(extractBits(bytesHi, 8, 15))
	inst.Data = NewVRegOperand(bits, bits, 0)

	switch inst.Opcode {
	case 21, 29, 80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 90, 91, 92, 93:
		inst.Data.RegCount = 2
		inst.Dst.RegCount = 2
	case 22, 30:
		inst.Data.RegCount = 4
		inst.Dst.RegCount = 4
	case 23, 31:
		inst.Data.RegCount = 3
		inst.Dst.RegCount = 3
	}
}

func (d *Disassembler) decodeSmem(inst *Inst, buf []byte) {
	bytesLo := binary.LittleEndian.Uint32(buf)
	bytesHi := binary.LittleEndian.Uint32(buf[4:])

	if extractBits(bytesLo, 16, 16) != 0 {
		inst.GlobalLevelCoherent = true
	}

	if extractBits(bytesLo, 17, 17) != 0 {
		inst.Imm = true
	}

	sbaseValue := extractBits(bytesLo, 0, 5)
	bits := int(sbaseValue << 1)
	inst.Base = NewSRegOperand(bits, bits, 2)

	sdataValue := extractBits(bytesLo, 6, 12)
	inst.Data, _ = getOperand(uint16(sdataValue))
	if inst.Data.OperandType == LiteralConstant {
		inst.ByteSize += 4
	}
	switch inst.Opcode {
	case 0:
		inst.Data.RegCount = 1
	case 1, 9, 17, 25:
		inst.Data.RegCount = 2
	case 2, 10, 18, 26:
		inst.Data.RegCount = 4
	case 3, 11, 19, 27:
		inst.Data.RegCount = 8
	case 4, 12, 20, 28:
		inst.Data.RegCount = 16
	}

	if inst.Imm {
		bits64 := int64(extractBits(bytesHi, 0, 19))
		inst.Offset = NewIntOperand(0, bits64)
	} else {
		bits := int(extractBits(bytesHi, 0, 19))
		inst.Offset = NewSRegOperand(bits, bits, 1)
	}
}

func (d *Disassembler) decodeSopp(inst *Inst, buf []byte) {
	bytes := binary.LittleEndian.Uint32(buf)

	inst.SImm16 = NewIntOperand(0, int64(extractBits(bytes, 0, 15)))
}

func (d *Disassembler) decodeVopc(inst *Inst, buf []byte) {
	bytes := binary.LittleEndian.Uint32(buf)
	inst.Src0, _ = getOperand(uint16(extractBits(bytes, 0, 8)))
	if inst.Src0.OperandType == LiteralConstant {
		inst.ByteSize += 4
		inst.Src0.LiteralConstant = BytesToUint32(buf[4:8])
	}

	bits := int(extractBits(bytes, 9, 16))
	inst.Src1 = NewVRegOperand(bits, bits, 0)
}

func (d *Disassembler) decodeSopc(inst *Inst, buf []byte) {
	bytes := binary.LittleEndian.Uint32(buf)
	inst.Src0, _ = getOperand(uint16(extractBits(bytes, 0, 7)))
	if inst.Src0.OperandType == LiteralConstant {
		inst.ByteSize += 4
		inst.Src0.LiteralConstant = BytesToUint32(buf[4:8])
	}

	inst.Src1, _ = getOperand(uint16(extractBits(bytes, 8, 15)))
	if inst.Src1.OperandType == LiteralConstant {
		inst.ByteSize += 4
		inst.Src1.LiteralConstant = BytesToUint32(buf[4:8])
	}
}

func (d *Disassembler) decodeVop3(inst *Inst, buf []byte) {
	bytesLo := binary.LittleEndian.Uint32(buf)
	bytesHi := binary.LittleEndian.Uint32(buf[4:])
	is64Bit := false
	if strings.Contains(inst.InstName, "64") {
		is64Bit = true
	}

	// TODO: Consider the VOP3b format
	if inst.Opcode <= 255 { // The comparison instructions
		inst.Dst, _ = getOperand(uint16(extractBits(bytesLo, 0, 7)))
		inst.Dst.RegCount = 2
	} else {
		bits := int(extractBits(bytesLo, 0, 7))
		inst.Dst = NewVRegOperand(bits, bits, 0)
	}
	if is64Bit {
		inst.Dst.RegCount = 2
	}

	inst.Abs = int(extractBits(bytesLo, 8, 10))
	if extractBits(bytesLo, 15, 15) != 0 {
		inst.Clamp = true
	}

	inst.Src0, _ = getOperand(uint16(extractBits(bytesHi, 0, 8)))
	if is64Bit {
		inst.Src0.RegCount = 2
	}
	inst.Src1, _ = getOperand(uint16(extractBits(bytesHi, 9, 17)))
	if is64Bit {
		inst.Src1.RegCount = 2
	}

	if (inst.Opcode <= 447 && inst.Opcode != 256) ||
		(inst.Opcode >= 464 && inst.Opcode <= 469) ||
		(inst.Opcode >= 640 && inst.Opcode <= 664) {
		// Do not use inst.Src2
	} else {
		// For V_CNDMASK_B32 in VOP3a only
		if inst.Opcode == 256 {
			is64Bit = true
		}
		inst.Src2, _ = getOperand(uint16(extractBits(bytesHi, 18, 26)))
		if is64Bit {
			inst.Src2.RegCount = 2
		}

	}

	inst.Omod = int(extractBits(bytesHi, 27, 28))
	inst.Neg = int(extractBits(bytesHi, 29, 31))
}

func (d *Disassembler) decodeSop1(inst *Inst, buf []byte) {
	bytes := binary.LittleEndian.Uint32(buf)
	inst.Src0, _ = getOperand(uint16(extractBits(bytes, 0, 7)))
	inst.Dst, _ = getOperand(uint16(extractBits(bytes, 16, 22)))

	if strings.Contains(inst.InstName, "64") {
		inst.Src0.RegCount = 2
		inst.Dst.RegCount = 2
	}
}
func (d *Disassembler) decodeSopk(inst *Inst, buf []byte) {
	bytes := binary.LittleEndian.Uint32(buf)
	inst.SImm16 = NewIntOperand(0, int64(extractBits(bytes, 0, 15)))
	inst.Dst, _ = getOperand(uint16(extractBits(bytes, 16, 22)))
}

// Decode parses the head of the buffer and returns the next instruction
func (d *Disassembler) Decode(buf []byte) (*Inst, error) {
	format, err := d.matchFormat(binary.LittleEndian.Uint16(buf[2:]))
	if err != nil {
		return nil, err
	}

	opcode := format.retrieveOpcode(binary.LittleEndian.Uint32(buf))
	instType, err := d.lookUp(format, opcode)
	if err != nil {
		return nil, err
	}

	inst := new(Inst)
	inst.Format = format
	inst.InstType = instType
	inst.ByteSize = format.ByteSizeExLiteral

	switch format.FormatType {
	case Sop2:
		d.decodeSop2(inst, buf)
	case Smem:
		d.decodeSmem(inst, buf)
	case Vop2:
		d.decodeVop2(inst, buf)
	case Vop1:
		d.decodeVop1(inst, buf)
	case Flat:
		d.decodeFlat(inst, buf)
	case Sopp:
		d.decodeSopp(inst, buf)
	case Vopc:
		d.decodeVopc(inst, buf)
	case Sopc:
		d.decodeSopc(inst, buf)
	case Vop3:
		d.decodeVop3(inst, buf)
	case Sop1:
		d.decodeSop1(inst, buf)
	case Sopk:
		d.decodeSopk(inst, buf)
	default:
		break
	}

	return inst, nil
}

// Disassemble take a binary file as an input and put the assembly code in a
// write
func (d *Disassembler) Disassemble(file *elf.File, w io.Writer) {
	sections := file.Sections

	for _, sec := range sections {
		if sec.Name == ".text" {
			data, _ := sec.Data()
			co := NewHsaCoFromData(data)

			buf := co.InstructionData()
			pc := 0x100
			for len(buf) > 0 {
				inst, err := d.Decode(buf)
				if err != nil {
					buf = buf[4:]
					pc += 4
				} else {
					// fmt.Fprintf(w, "%s %08b\n", inst, buf[0:inst.ByteSize])
					fmt.Fprintf(w, "0x%016x\t%s\n", pc, inst)
					buf = buf[inst.ByteSize:]
					pc += inst.ByteSize
				}
			}
		}
	}
}

func (d *Disassembler) initFormatList() {
	d.formatList = make([]*Format, 0, 17)
	for _, value := range FormatTable {
		d.formatList = append(d.formatList, value)
	}
	sort.Slice(d.formatList,
		func(i, j int) bool {
			return d.formatList[i].Mask > d.formatList[j].Mask
		})
}

func (d *Disassembler) initializeDecodeTable() {
	d.decodeTables = make(map[FormatType]*decodeTable)

	// SOP2 instructions
	d.addInstType(&InstType{"s_add_u32", 0, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_sub_u32", 1, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_add_i32", 2, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_sub_i32", 3, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_addc_u32", 4, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_subb_u32", 5, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_min_i32", 6, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_min_u32", 7, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_max_i32", 8, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_max_u32", 9, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cselect_b32", 10, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cselect_b64", 11, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_and_b32", 12, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_and_b64", 13, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_or_b32", 14, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_or_b64", 15, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_xor_b32", 16, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_xor_b64", 17, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_andn2_b32", 18, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_andn2_b64", 19, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_orn2_b32", 20, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_orn2_b64", 21, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_nand_b32", 22, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_nand_b64", 23, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_nor_b32", 24, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_nor_b64", 25, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_xnor_b32", 26, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_xnor_b64", 27, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_lshl_b32", 28, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_lshl_b64", 29, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_lshr_b32", 30, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_lshr_b64", 31, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_ashr_i32", 32, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_ashr_i64", 33, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_bfm_b32", 34, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_bfm_b64", 35, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_mul_i32", 36, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_bfe_u32", 37, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_bfe_i32", 38, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_bfe_u64", 39, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_bfe_i64", 40, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cbrahcn_g_fork", 41, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_absdiss_i32", 42, FormatTable[Sop2], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_rfe_restore_b64", 43, FormatTable[Sop2], 0, ExeUnitScalar})

	// VOP2 instructions
	d.addInstType(&InstType{"v_cndmask_b32", 0, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_add_f32", 1, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_sub_f32", 2, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_subrev_f32", 3, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mul_legacy_f32", 4, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mul_f32", 5, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mul_i32_i24", 6, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mul_hi_i32_i24", 7, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mul_u32_u24", 8, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mul_hi_u32_u24", 9, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_min_f32", 10, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_max_f32", 11, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_min_i32", 12, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_max_i32", 13, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_min_u32", 14, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_max_u32", 15, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_lshrrev_b32", 16, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_ashrrev_i32", 17, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_lshlrev_b32", 18, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_and_b32", 19, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_or_b32", 20, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_xor_b32", 21, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mac_f32_e32", 22, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_madmk_f32", 23, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_madak_f32", 24, FormatTable[Vop2], 0, ExeUnitVALU})
	// On documentation, this is v_add_u32
	d.addInstType(&InstType{"v_add_i32_e32", 25, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_sub_u32", 26, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_subrev_u32", 27, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_addc_u32_e32", 28, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_subb_u32", 29, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_subbrev_u32", 30, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_add_f16", 31, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_sub_f16", 32, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_subrev_f16", 33, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mul_f16", 34, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mac_f16", 35, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_madmk_f16", 36, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_madak_f16", 37, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_add_u16", 38, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_sub_u16", 39, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_subrev_u16", 40, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mul_lo_u16", 41, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_lshlrev_b16", 42, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_lshrrev_b16", 43, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_ashrrev_i16", 44, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_max_f16", 45, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_min_f16", 46, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_max_u16", 47, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_max_i16", 48, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_min_u16", 49, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_min_i16", 50, FormatTable[Vop2], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_ldexp_f16", 51, FormatTable[Vop2], 0, ExeUnitVALU})

	// VOP1 instructions
	d.addInstType(&InstType{"v_nop", 0, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mov_b32_e32", 1, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_readfirstlane_b32", 2, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_i32_f64", 3, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_f64_i32", 4, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_f32_i32", 5, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_f32_u32", 6, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_u32_f32", 7, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_i32_f32", 8, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_f16_f32", 10, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_f32_f16", 11, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_rpi_i32_f32", 12, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_flr_i32_f32", 13, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_off_f32_i4", 14, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_f32_f64", 15, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_f64_f32", 16, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_f32_ubyte0", 17, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_f32_ubyte1", 18, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_f32_ubyte2", 19, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_f32_ubyte3", 20, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_u32_f64", 21, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_f64_u32", 22, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_trunc_f64", 23, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_ceil_f64", 24, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_rndne_f64", 25, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_floor_f64", 26, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_fract_f32", 27, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_trunc_f32", 28, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_ceil_f32", 29, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_rndne_f32", 30, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_floor_f32", 31, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_exp_f32", 32, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_log_f32", 33, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_rcp_f32", 34, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_rcp_iflag_f32", 35, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_rsq_f32", 36, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_rcp_f64", 37, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_rsq_f64", 38, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_sqrt_f32", 39, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_sqrt_f64", 40, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_sin_f32", 41, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cos_f32", 42, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_not_b32", 43, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_bfrev_b32", 44, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_ffbh_u32", 45, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_ffbl_b32", 46, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_ffbh_i32", 47, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_frexp_exp_i32_f64", 48, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_frexp_mant_f64", 49, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_fract_f64", 50, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_frexp_exp_i32_f32", 51, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_frexp_mant_f32", 52, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_clrexcp", 53, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_movreld_b32", 54, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_movrels_b32", 55, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_movrelsd_b32", 56, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_f16_u16", 57, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_f16_i16", 58, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_u16_f16", 59, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_i16_f16", 60, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_rcp_f16", 61, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_sqrt_f16", 62, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_rsq_f16", 63, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_log_f16", 64, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_exp_f16", 65, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_frexp_mant_f16", 66, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_frexp_exp_i16_f16", 67, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_floor_f16", 68, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_ceil_f16", 69, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_trunc_f16", 70, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_rndne_f16", 71, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_fract_f16", 72, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_sin_f16", 73, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cos_f16", 74, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_exp_legacy_f32", 75, FormatTable[Vop1], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_log_legacy_f32", 76, FormatTable[Vop1], 0, ExeUnitVALU})

	// Flat Instructions
	// I am not sure why, but seems the numbers in the official disassembler
	// does not match the documentation. Here follows the official disassembler.
	d.addInstType(&InstType{"flat_load_ubyte", 16, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_load_sbyte", 17, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_load_ushort", 18, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_load_sshort", 19, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_load_dword", 20, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_load_dwordx2", 21, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_load_dwordx4", 22, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_load_dwordx3", 23, FormatTable[Flat], 0, ExeUnitVALU})
	// Unitl here
	d.addInstType(&InstType{"flat_store_byte", 24, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_store_short", 26, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_store_dword", 28, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_store_dwordx2", 29, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_store_dwordx4", 30, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_store_dwordx3", 31, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_swap", 48, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_cmpswap", 49, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_add", 50, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_sub", 51, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_smin", 53, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_umin", 54, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_smax", 55, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flag_atomic_umax", 56, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_and", 57, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_or", 58, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_xor", 59, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_inc", 60, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_dec", 61, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_swap_x2", 80, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_cmpswap_x2", 81, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_add_x2", 82, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_sub_x2", 83, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_smin_x2", 85, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_umin_x2", 86, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_smax_x2", 87, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_umax_x2", 88, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_and_x2", 89, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_or_x2", 90, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_xor_x2", 91, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_inc_x2", 92, FormatTable[Flat], 0, ExeUnitVALU})
	d.addInstType(&InstType{"flat_atomic_dec_x2", 93, FormatTable[Flat], 0, ExeUnitVALU})

	// SMEM instructions
	d.addInstType(&InstType{"s_load_dword", 0, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_load_dwordx2", 1, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_load_dwordx4", 2, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_load_dwordx8", 3, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_load_dwordx16", 4, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_buffer_load_dword", 8, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_buffer_load_dwordx2", 9, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_buffer_load_dwordx4", 10, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_buffer_load_dwordx8", 11, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_buffer_load_dwordx16", 12, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_store_dword", 16, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_store_dwordx2", 17, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_store_dwordx4", 18, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_buffer_store_dword", 24, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_buffer_store_dwordx2", 25, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_buffer_store_dwordx4", 26, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_dcache_inv", 32, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_dcache_wb", 33, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_dcache_inv_vol", 34, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_dcache_wb_vol", 35, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_memtime", 36, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_memrealtime", 37, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_atc_probe", 38, FormatTable[Smem], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_atc_probe_buffer", 39, FormatTable[Smem], 0, ExeUnitScalar})

	// SOPP instructions
	d.addInstType(&InstType{"s_nop", 0, FormatTable[Sopp], 0, ExeUnitSpecial})
	d.addInstType(&InstType{"s_endpgm", 1, FormatTable[Sopp], 0, ExeUnitSpecial})
	d.addInstType(&InstType{"s_branch", 2, FormatTable[Sopp], 0, ExeUnitBranch})
	d.addInstType(&InstType{"s_cbranch_scc0", 4, FormatTable[Sopp], 0, ExeUnitBranch})
	d.addInstType(&InstType{"s_cbranch_scc1", 5, FormatTable[Sopp], 0, ExeUnitBranch})
	d.addInstType(&InstType{"s_cbranch_vccz", 6, FormatTable[Sopp], 0, ExeUnitBranch})
	d.addInstType(&InstType{"s_cbranch_vccnz", 7, FormatTable[Sopp], 0, ExeUnitBranch})
	d.addInstType(&InstType{"s_cbranch_execz", 8, FormatTable[Sopp], 0, ExeUnitBranch})
	d.addInstType(&InstType{"s_cbranch_execnz", 9, FormatTable[Sopp], 0, ExeUnitBranch})
	d.addInstType(&InstType{"s_barrier", 10, FormatTable[Sopp], 0, ExeUnitSpecial})
	d.addInstType(&InstType{"s_setkill", 11, FormatTable[Sopp], 0, ExeUnitSpecial})
	d.addInstType(&InstType{"s_waitcnt", 12, FormatTable[Sopp], 0, ExeUnitSpecial})
	d.addInstType(&InstType{"s_sethalt", 13, FormatTable[Sopp], 0, ExeUnitSpecial})
	d.addInstType(&InstType{"s_sleep", 14, FormatTable[Sopp], 0, ExeUnitSpecial})
	d.addInstType(&InstType{"s_setprio", 15, FormatTable[Sopp], 0, ExeUnitSpecial})
	d.addInstType(&InstType{"s_sendmsg", 16, FormatTable[Sopp], 0, ExeUnitBranch})
	d.addInstType(&InstType{"s_sendmsghalt", 17, FormatTable[Sopp], 0, ExeUnitBranch})
	d.addInstType(&InstType{"s_trap", 18, FormatTable[Sopp], 0, ExeUnitSpecial})
	d.addInstType(&InstType{"s_icache_inv", 19, FormatTable[Sopp], 0, ExeUnitSpecial})
	d.addInstType(&InstType{"s_incperflevel", 20, FormatTable[Sopp], 0, ExeUnitSpecial})
	d.addInstType(&InstType{"s_decperflevel", 21, FormatTable[Sopp], 0, ExeUnitSpecial})
	d.addInstType(&InstType{"s_ttracedata", 22, FormatTable[Sopp], 0, ExeUnitSpecial})
	d.addInstType(&InstType{"s_cbranch_cdbgsys", 23, FormatTable[Sopp], 0, ExeUnitBranch})
	d.addInstType(&InstType{"s_cbranch_cdbguser", 24, FormatTable[Sopp], 0, ExeUnitBranch})
	d.addInstType(&InstType{"s_cbranch_cdbgsys_or_user", 25, FormatTable[Sopp], 0, ExeUnitBranch})
	d.addInstType(&InstType{"s_cbranch_cdbgsys_and_user", 26, FormatTable[Sopp], 0, ExeUnitBranch})
	d.addInstType(&InstType{"s_endpgm_saved", 27, FormatTable[Sopp], 0, ExeUnitSpecial})
	d.addInstType(&InstType{"s_set_gpr_idx_off", 28, FormatTable[Sopp], 0, ExeUnitSpecial})
	d.addInstType(&InstType{"s_set_gpr_idx_mode", 29, FormatTable[Sopp], 0, ExeUnitSpecial})

	// SOPC instructions
	d.addInstType(&InstType{"s_cmp_eq_i32", 0, FormatTable[Sopc], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmp_lg_i32", 1, FormatTable[Sopc], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmp_gt_i32", 2, FormatTable[Sopc], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmp_ge_i32", 3, FormatTable[Sopc], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmp_lt_i32", 4, FormatTable[Sopc], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmp_le_i32", 5, FormatTable[Sopc], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmp_eq_u32", 6, FormatTable[Sopc], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmp_lg_u32", 7, FormatTable[Sopc], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmp_gt_u32", 8, FormatTable[Sopc], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmp_ge_u32", 9, FormatTable[Sopc], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmp_lt_u32", 10, FormatTable[Sopc], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmp_le_u32", 11, FormatTable[Sopc], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_bitcmp0_b32", 12, FormatTable[Sopc], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_bitcmp1_b32", 13, FormatTable[Sopc], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_bitcmp0_b64", 14, FormatTable[Sopc], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_bitcmp1_b64", 15, FormatTable[Sopc], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_setvskip", 16, FormatTable[Sopc], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_set_gpr_idx_on", 17, FormatTable[Sopc], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmp_eq_u64", 18, FormatTable[Sopc], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmp_ne_u64", 19, FormatTable[Sopc], 0, ExeUnitScalar})

	// SOPK instructions
	d.addInstType(&InstType{"s_movk_i32", 0, FormatTable[Sopk], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmovk_i32", 1, FormatTable[Sopk], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmpk_eq_i32", 2, FormatTable[Sopk], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmpk_lg_i32", 3, FormatTable[Sopk], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmpk_gt_i32", 4, FormatTable[Sopk], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmpk_ge_i32", 5, FormatTable[Sopk], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmpk_lt_i32", 6, FormatTable[Sopk], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmpk_le_i32", 7, FormatTable[Sopk], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmpk_eq_u32", 8, FormatTable[Sopk], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmpk_lg_u32", 9, FormatTable[Sopk], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmpk_gt_u32", 10, FormatTable[Sopk], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmpk_ge_u32", 11, FormatTable[Sopk], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmpk_lt_u32", 12, FormatTable[Sopk], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmpk_le_u32", 13, FormatTable[Sopk], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_addk_i32", 14, FormatTable[Sopk], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_mulk_i32", 15, FormatTable[Sopk], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cbranch_i_fork", 16, FormatTable[Sopk], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_getreg_b32", 17, FormatTable[Sopk], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_setreg_b32", 18, FormatTable[Sopk], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_setreg_imm32_b32", 20, FormatTable[Sopk], 0, ExeUnitScalar})

	// VOPC instruction
	d.addInstType(&InstType{"v_cmp_class_f32", 0x10, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_class_f32", 0x11, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_class_f64", 0x12, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_class_f64", 0x13, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_class_f16", 0x14, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_class_f16", 0x15, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_f_f16", 0x20, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_lt_f16", 0x21, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_eq_f16", 0x22, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_le_f16", 0x23, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_gt_f16", 0x24, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_lg_f16", 0x25, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_ge_f16", 0x26, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_o_f16", 0x27, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_u_f16", 0x28, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_nge_f16", 0x29, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_nlg_f16", 0x2a, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_ngt_f16", 0x2b, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_nle_f16", 0x2c, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_neq_f16", 0x2d, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_nlt_f16", 0x2e, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_tru_f16", 0x2f, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_f_f16", 0x30, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_lt_f16", 0x31, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_eq_f16", 0x32, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_le_f16", 0x33, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_gt_f16", 0x34, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_lg_f16", 0x35, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_ge_f16", 0x36, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_o_f16", 0x37, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_u_f16", 0x38, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_nge_f16", 0x39, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_nlg_f16", 0x3a, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_ngt_f16", 0x3b, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_nle_f16", 0x3c, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_neq_f16", 0x3d, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_nlt_f16", 0x3e, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_tru_f16", 0x3f, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_f_f32", 0x40, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_lt_f32", 0x41, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_eq_f32", 0x42, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_le_f32", 0x43, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_gt_f32", 0x44, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_lg_f32", 0x45, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_ge_f32", 0x46, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_o_f32", 0x47, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_u_f32", 0x48, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_nge_f32", 0x49, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_nlg_f32", 0x4a, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_ngt_f32", 0x4b, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_nle_f32", 0x4c, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_neq_f32", 0x4d, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_nlt_f32", 0x4e, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_tru_f32", 0x4f, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_f_f32", 0x50, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_lt_f32", 0x51, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_eq_f32", 0x52, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_le_f32", 0x53, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_gt_f32", 0x54, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_lg_f32", 0x55, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_ge_f32", 0x56, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_o_f32", 0x57, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_u_f32", 0x58, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_nge_f32", 0x59, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_nlg_f32", 0x5a, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_ngt_f32", 0x5b, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_nle_f32", 0x5c, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_neq_f32", 0x5d, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_nlt_f32", 0x5e, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_tru_f32", 0x5f, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_f_f64", 0x60, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_lt_f64", 0x61, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_eq_f64", 0x62, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_le_f64", 0x63, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_gt_f64", 0x64, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_lg_f64", 0x65, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_ge_f64", 0x66, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_o_f64", 0x67, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_u_f64", 0x68, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_nge_f64", 0x69, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_nlg_f64", 0x6a, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_ngt_f64", 0x6b, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_nle_f64", 0x6c, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_neq_f64", 0x6d, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_nlt_f64", 0x6e, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_tru_f64", 0x6f, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_f_f64", 0x70, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_lt_f64", 0x71, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_eq_f64", 0x72, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_le_f64", 0x73, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_gt_f64", 0x74, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_lg_f64", 0x75, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_ge_f64", 0x76, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_o_f64", 0x77, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_u_f64", 0x78, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_nge_f64", 0x79, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_nlg_f64", 0x7a, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_ngt_f64", 0x7b, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_nle_f64", 0x7c, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_neq_f64", 0x7d, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_nlt_f64", 0x7e, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_tru_f64", 0x7f, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_f_i16", 0xa0, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_lt_i16", 0xa1, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_eq_i16", 0xa2, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_le_i16", 0xa3, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_gt_i16", 0xa4, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_lg_i16", 0xa5, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_ge_i16", 0xa6, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_tru_i16", 0xa7, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_f_u16", 0xa8, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_lt_u16", 0xa9, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_eq_u16", 0xaa, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_le_u16", 0xab, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_gt_u16", 0xac, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_lg_u16", 0xad, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_ge_u16", 0xae, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_tru_u16", 0xaf, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_f_i16", 0xb0, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_lt_i16", 0xb1, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_eq_i16", 0xb2, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_le_i16", 0xb3, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_gt_i16", 0xb4, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_lg_i16", 0xb5, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_ge_i16", 0xb6, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_tru_i16", 0xb7, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_f_u16", 0xb8, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_lt_u16", 0xb9, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_eq_u16", 0xba, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_le_u16", 0xbb, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_gt_u16", 0xbc, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_lg_u16", 0xbd, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_ge_u16", 0xbe, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_tru_u16", 0xbf, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_f_i32", 0xc0, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_lt_i32", 0xc1, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_eq_i32", 0xc2, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_le_i32", 0xc3, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_gt_i32", 0xc4, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_lg_i32", 0xc5, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_ge_i32", 0xc6, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_tru_i32", 0xc7, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_f_u32", 0xc8, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_lt_u32", 0xc9, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_eq_u32", 0xca, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_le_u32", 0xcb, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_gt_u32", 0xcc, FormatTable[Vopc], 0, ExeUnitVALU})
	// It is lg in the documentation
	d.addInstType(&InstType{"v_cmp_ne_u32", 0xcd, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_ge_u32", 0xce, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_tru_u32", 0xcf, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_f_i32", 0xd0, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_lt_i32", 0xd1, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_eq_i32", 0xd2, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_le_i32", 0xd3, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_gt_i32", 0xd4, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_lg_i32", 0xd5, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_ge_i32", 0xd6, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_tru_i32", 0xd7, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_f_u32", 0xd8, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_lt_u32", 0xd9, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_eq_u32", 0xda, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_le_u32", 0xdb, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_gt_u32", 0xdc, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_lg_u32", 0xdd, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_ge_u32", 0xde, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_tru_u32", 0xdf, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_f_i64", 0xe0, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_lt_i64", 0xe1, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_eq_i64", 0xe2, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_le_i64", 0xe3, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_gt_i64", 0xe4, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_lg_i64", 0xe5, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_ge_i64", 0xe6, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_tru_i64", 0xe7, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_f_u64", 0xe8, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_lt_u64", 0xe9, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_eq_u64", 0xea, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_le_u64", 0xeb, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_gt_u64", 0xec, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_lg_u64", 0xed, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_ge_u64", 0xee, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmp_tru_u64", 0xef, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_f_i64", 0xf0, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_lt_i64", 0xf1, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_eq_i64", 0xf2, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_le_i64", 0xf3, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_gt_i64", 0xf4, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_lg_i64", 0xf5, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_ge_i64", 0xf6, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_tru_i64", 0xf7, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_f_u64", 0xf8, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_lt_u64", 0xf9, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_eq_u64", 0xfa, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_le_u64", 0xfb, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_gt_u64", 0xfc, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_lg_u64", 0xfd, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_ge_u64", 0xfe, FormatTable[Vopc], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cmpx_tru_u64", 0xff, FormatTable[Vopc], 0, ExeUnitVALU})

	// VOP3 Instructions
	for _, instType := range d.decodeTables[Vopc].insts {
		d.addInstType(&InstType{instType.InstName, instType.Opcode,
			FormatTable[Vop3], 0, ExeUnitVALU})
	}
	for _, instType := range d.decodeTables[Vop2].insts {
		d.addInstType(&InstType{instType.InstName,
			instType.Opcode + Opcode(256),
			FormatTable[Vop3], 0, ExeUnitVALU})
	}
	for _, instType := range d.decodeTables[Vop1].insts {
		d.addInstType(&InstType{instType.InstName,
			instType.Opcode + Opcode(320),
			FormatTable[Vop3], 0, ExeUnitVALU})
	}
	d.addInstType(&InstType{"v_mad_legacy_f32", 448, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mad_f32", 449, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mad_i32_i24", 450, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mad_u32_u24", 451, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cubeid_f32", 452, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cubesc_f32", 453, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cubetc_f32", 454, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cubema_f32", 455, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_bfe_u32", 456, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_bfe_i32", 457, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_bfi_b32", 458, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_fma_f32", 459, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_fma_f64", 460, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_lerp_u8", 461, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_alignbit_b32", 462, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_alignbyte_b32", 463, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_min3_f32", 464, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_min3_i32", 465, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_min3_u32", 466, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_max3_f32", 467, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_max3_i32", 468, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_max3_u32", 469, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_med3_f32", 470, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_med3_i32", 471, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_med3_u32", 472, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_sad_u8", 473, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_sad_hi_u8", 474, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_sad_u16", 475, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_sad_u32", 476, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_pk_u8_f32", 477, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_div_fixup_f32", 478, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_div_fixup_f64", 479, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_div_scale_f32", 480, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_div_scale_f64", 481, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_div_fmas_f32", 482, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_div_fmas_f64", 483, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_msad_u8", 484, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_qsad_pk_u16_u8", 485, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mqsad_pk_u16_u8", 486, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mqsad_u32_u8", 487, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mad_u64_u32", 488, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mad_i64_i32", 489, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mad_f16", 490, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mad_u16", 491, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mad_i16", 492, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_perm_b32", 493, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_fma_f16", 494, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_div_fixup_16", 495, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_pkaccum_u8_f32", 496, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_interp_p1_f32", 624, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_interp_p2_f32", 625, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_interp_mov_f32", 626, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_interp_p1ll_f16", 628, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_interp_p1lv_f16", 629, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_interp_p2_f16", 630, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_add_f64", 640, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mul_f64", 641, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_min_f64", 642, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_max_f64", 643, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_ldexp_f64", 644, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mul_lo_u32", 645, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mul_hi_u32", 646, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mul_hi_i32", 647, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_ldexp_f32", 648, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_readlane_b32", 649, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_writelane_b32", 650, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_bcnt_u32_b32", 651, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mbcnt_lo_u32_b32", 652, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_mbcnt_hi_u32_b32", 653, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_lshlrev_b64", 655, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_lshrrev_b64", 656, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_ashrrev_i64", 657, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_trig_preop_f64", 658, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_bfm_b32", 659, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_pknorm_i16_f32", 660, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_pknorm_u16_f32", 661, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_pkrtz_f16_f32", 662, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_pk_u16_u32", 663, FormatTable[Vop3], 0, ExeUnitVALU})
	d.addInstType(&InstType{"v_cvt_pk_i16_i32", 664, FormatTable[Vop3], 0, ExeUnitVALU})

	// SOP1 Instructions
	d.addInstType(&InstType{"s_mov_b32", 0, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_mov_b64", 1, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmov_b32", 2, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cmov_b64", 3, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_not_b32", 4, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_not_b64", 5, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_wqm_b32", 6, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_wqm_b64 ", 7, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_brev_b32", 8, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_brev_b64", 9, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_bcnt0_i32_b32", 10, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_bcnt0_i32_b64", 11, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_bcnt1_i32_b32", 12, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_bcnt1_i32_b64", 13, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_ff0_i32_b32", 14, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_ff0_i32_b64", 15, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_ff1_i32_b32", 16, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_ff1_i32_b64", 17, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_flbit_i32_b32", 18, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_flbit_i32_b64", 19, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_flbit_i32", 20, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_flbit_i32_i64", 21, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_sext_i32_i8", 22, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_sext_i32_i16", 23, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_bitset0_b32", 24, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_bitset0_b64", 25, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_bitset1_b32", 26, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_bitset1_b64", 27, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_getpc_b64", 28, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_setpc_b64", 29, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_swappc_b64", 30, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_rfe_b64", 31, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_and_saveexec_b64", 32, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_or_saveexec_b64", 33, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_xor_saveexec_b64", 34, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_andn2_saveexec_b64", 35, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_orn2_saveexec_b64", 36, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_nand_saveexec_b64", 37, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_nor_saveexec_b64", 38, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_xnor_saveexec_b64", 39, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_quadmask_b32", 40, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_quadmask_b64", 41, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_movrels_b32", 42, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_movrels_b64", 43, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_movreld_b32", 44, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_movreld_b64", 45, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_cbranch_join", 46, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_abs_i32", 48, FormatTable[Sop1], 0, ExeUnitScalar})
	d.addInstType(&InstType{"s_set_gpr_idx_idx", 49, FormatTable[Sop1], 0, ExeUnitScalar})

}
