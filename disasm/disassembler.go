package disasm

import (
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"
)

const (
	sop2 = iota
	sopk
	sop1
	sopc
	sopp
	smem
	vop2
	vop1
	vop3
	vopc
	vintrp
	ds
	mubuf
	mtbuf
	mimg
	exp
	flat
	formatTypeCount
)

type formatType int

// Format defines the possible microcode format of instructions
type Format struct {
	FormatType        formatType
	FormatName        string
	Encoding          uint16
	Mask              uint16
	ByteSizeExLiteral int
	OpcodeLow         uint8
	OpcodeHigh        uint8
}

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

// A InstType represents an instruction type. For example s_barrier instruction
// is a intruction type
type InstType struct {
	InstName string
	Opcode   Opcode
	Format   *Format
}

type decodeTable struct {
	insts map[Opcode]*InstType
}

func newDecodeTable() *decodeTable {
	return &decodeTable{make(map[Opcode]*InstType)}
}

// Disassembler is the unit that can decode .hsaco file
type Disassembler struct {
	formatTable map[formatType]*Format
	formatList  []*Format

	// Maps from the format to table
	decodeTables map[formatType]*decodeTable
}

func (d *Disassembler) addInstType(info *InstType) {
	if d.decodeTables[info.Format.FormatType] == nil {
		d.decodeTables[info.Format.FormatType] = newDecodeTable()
	}
	d.decodeTables[info.Format.FormatType].insts[info.Opcode] = info
}

// NewDisassembler creates a new disassembler
func NewDisassembler() *Disassembler {
	d := new(Disassembler)

	d.initializeFormatTable()
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

func (d *Disassembler) loopUp(format *Format, opcode Opcode) (*InstType, error) {
	if d.decodeTables[format.FormatType] != nil &&
		d.decodeTables[format.FormatType].insts[opcode] != nil {
		return d.decodeTables[format.FormatType].insts[opcode], nil
	}

	return nil, fmt.Errorf("Instruction format %s, opcode %d not found",
		format.FormatName, opcode)
}

func (d *Disassembler) decodeSop2(inst *Instruction, buf []byte) {
	bytes := binary.LittleEndian.Uint32(buf)

	src0Value := extractBits(bytes, 0, 7)
	inst.Src0, _ = getOperand(uint16(src0Value))
	if inst.Src0.OperandType == LiteralConstant {
		inst.ByteSize += 4
	}

	src1Value := extractBits(bytes, 8, 15)
	inst.Src1, _ = getOperand(uint16(src1Value))
	if inst.Src1.OperandType == LiteralConstant {
		inst.ByteSize += 4
	}

	sdstValue := extractBits(bytes, 16, 22)
	inst.Dst, _ = getOperand(uint16(sdstValue))
}

func (d *Disassembler) decodeVop1(inst *Instruction, buf []byte) {
	bytes := binary.LittleEndian.Uint32(buf)

	src0Value := extractBits(bytes, 0, 8)
	inst.Src0, _ = getOperand(uint16(src0Value))
	if inst.Src0.OperandType == LiteralConstant {
		inst.ByteSize += 4
	}

	dstValue := extractBits(bytes, 17, 24)
	inst.Dst, _ = getOperand(uint16(dstValue + 256))
}

func (d *Disassembler) decodeVop2(inst *Instruction, buf []byte) {
	bytes := binary.LittleEndian.Uint32(buf)
	inst.Src0, _ = getOperand(uint16(extractBits(bytes, 0, 8)))
	if inst.Src0.OperandType == LiteralConstant {
		inst.ByteSize += 4
	}
	inst.Src1 = NewVRegOperand(int(extractBits(bytes, 9, 16)), 0)
	inst.Dst = NewVRegOperand(int(extractBits(bytes, 17, 24)), 0)
}

func (d *Disassembler) decodeFlat(inst *Instruction, buf []byte) {
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

	inst.Addr = NewVRegOperand(int(extractBits(bytesHi, 0, 7)), 2)

	inst.Dst = NewVRegOperand(int(extractBits(bytesHi, 24, 31)), 0)
	inst.Data = NewVRegOperand(int(extractBits(bytesHi, 8, 15)), 0)
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

func (d *Disassembler) decodeSmem(inst *Instruction, buf []byte) {
	bytesLo := binary.LittleEndian.Uint32(buf)
	bytesHi := binary.LittleEndian.Uint32(buf[4:])

	if extractBits(bytesLo, 16, 16) != 0 {
		inst.GlobalLevelCoherent = true
	}

	if extractBits(bytesLo, 17, 17) != 0 {
		inst.Imm = true
	}

	sbaseValue := extractBits(bytesLo, 0, 5)
	inst.Base = NewSRegOperand(int(sbaseValue<<1), 2)

	sdataValue := extractBits(bytesLo, 6, 12)
	inst.Data, _ = getOperand(uint16(sdataValue))
	if inst.Data.OperandType == LiteralConstant {
		inst.ByteSize += 4
	}
	switch inst.Opcode {
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
		inst.Offset = NewIntOperand(int64(extractBits(bytesHi, 0, 19)))
	} else {
		inst.Offset = NewSRegOperand(int(extractBits(bytesHi, 0, 19)), 1)
	}
}

func (d *Disassembler) decodeSopp(inst *Instruction, buf []byte) {
	bytes := binary.LittleEndian.Uint32(buf)

	inst.SImm16 = NewIntOperand(int64(extractBits(bytes, 0, 15)))
}

func (d *Disassembler) decodeVopc(inst *Instruction, buf []byte) {
	bytes := binary.LittleEndian.Uint32(buf)
	inst.Src0, _ = getOperand(uint16(extractBits(bytes, 0, 8)))
	if inst.Src0.OperandType == LiteralConstant {
		inst.ByteSize += 4
	}

	inst.Src1 = NewVRegOperand(int(extractBits(bytes, 9, 16)), 0)
}

func (d *Disassembler) decodeSopc(inst *Instruction, buf []byte) {
	bytes := binary.LittleEndian.Uint32(buf)
	inst.Src0, _ = getOperand(uint16(extractBits(bytes, 0, 7)))
	if inst.Src0.OperandType == LiteralConstant {
		inst.ByteSize += 4
	}

	inst.Src1, _ = getOperand(uint16(extractBits(bytes, 8, 15)))
	if inst.Src1.OperandType == LiteralConstant {
		inst.ByteSize += 4
	}
}

func (d *Disassembler) decodeVop3(inst *Instruction, buf []byte) {
	bytesLo := binary.LittleEndian.Uint32(buf)
	bytesHi := binary.LittleEndian.Uint32(buf[4:])

	// TODO: Consider the VOP3b format
	if inst.Opcode <= 255 { // The comparison instructions
		inst.Dst, _ = getOperand(uint16(extractBits(bytesLo, 0, 7)))
	} else {
		inst.Dst = NewVRegOperand(int(extractBits(bytesLo, 0, 7)), 0)
	}
	inst.Abs = int(extractBits(bytesLo, 8, 10))
	if extractBits(bytesLo, 15, 15) != 0 {
		inst.Clamp = true
	}

	inst.Src0, _ = getOperand(uint16(extractBits(bytesHi, 0, 8)))
	inst.Src1, _ = getOperand(uint16(extractBits(bytesHi, 9, 17)))
	inst.Src2, _ = getOperand(uint16(extractBits(bytesHi, 18, 26)))

	inst.Omod = int(extractBits(bytesHi, 27, 28))
	inst.Neg = int(extractBits(bytesHi, 29, 31))
}

// Decode parses the head of the buffer and returns the next instruction
func (d *Disassembler) Decode(buf []byte) (*Instruction, error) {
	format, err := d.matchFormat(binary.LittleEndian.Uint16(buf[2:]))
	if err != nil {
		return nil, err
	}

	opcode := format.retrieveOpcode(binary.LittleEndian.Uint32(buf))
	instType, err := d.loopUp(format, opcode)
	if err != nil {
		return nil, err
	}

	inst := new(Instruction)
	inst.Format = format
	inst.InstType = instType
	inst.ByteSize = format.ByteSizeExLiteral

	switch format.FormatType {
	case sop2:
		d.decodeSop2(inst, buf)
	case smem:
		d.decodeSmem(inst, buf)
	case vop2:
		d.decodeVop2(inst, buf)
	case vop1:
		d.decodeVop1(inst, buf)
	case flat:
		d.decodeFlat(inst, buf)
	case sopp:
		d.decodeSopp(inst, buf)
	case vopc:
		d.decodeVopc(inst, buf)
	case sopc:
		d.decodeSopc(inst, buf)
	case vop3:
		d.decodeVop3(inst, buf)
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
			co := NewHsaCo(data)

			buf := co.InstructionData()
			for len(buf) > 0 {
				inst, err := d.Decode(buf)
				if err != nil {
					buf = buf[4:]
				} else {
					fmt.Printf("%s\n", inst)
					buf = buf[inst.ByteSize:]
				}
			}
		}
	}
}

func (d *Disassembler) initializeFormatTable() {
	d.formatTable = make(map[formatType]*Format)
	d.formatTable[sop1] = &Format{sop1, "sop1", 0xBE80, 0xFF80, 4, 8, 15}
	d.formatTable[sopc] = &Format{sopc, "sopc", 0xBF00, 0xFF80, 4, 16, 22}
	d.formatTable[sopp] = &Format{sopp, "sopp", 0xBF80, 0xFF80, 4, 16, 22}
	d.formatTable[vop1] = &Format{vop1, "vop1", 0x7E00, 0xFE00, 4, 9, 16}
	d.formatTable[vopc] = &Format{vopc, "vopc", 0x7C00, 0xFE00, 4, 17, 24}
	d.formatTable[smem] = &Format{smem, "smem", 0xC000, 0xFC00, 8, 18, 25}
	d.formatTable[vop3] = &Format{vop3, "vop3", 0xD000, 0xFC00, 8, 16, 25}
	d.formatTable[vintrp] = &Format{vintrp, "vintrp", 0xC800, 0xFC00, 4, 16, 17}
	d.formatTable[ds] = &Format{ds, "ds", 0xD800, 0xFC00, 8, 17, 24}
	d.formatTable[mubuf] = &Format{mubuf, "mubuf", 0xE000, 0xFC00, 8, 18, 24}
	d.formatTable[mtbuf] = &Format{mtbuf, "mtbuf", 0xE800, 0xFC00, 8, 15, 18}
	d.formatTable[mimg] = &Format{mimg, "mimg", 0xF000, 0xFC00, 8, 18, 24}
	d.formatTable[exp] = &Format{exp, "exp", 0xC400, 0xFC00, 8, 0, 0}
	d.formatTable[flat] = &Format{flat, "flat", 0xDC00, 0xFC00, 8, 18, 24}
	d.formatTable[sopk] = &Format{sopk, "sopk", 0xB000, 0xF000, 4, 23, 27}
	d.formatTable[sop2] = &Format{sop2, "sop2", 0x8000, 0xA000, 4, 23, 29}
	d.formatTable[vop2] = &Format{vop2, "vop2", 0x0000, 0x8000, 4, 25, 30}

	d.formatList = make([]*Format, 0, 17)
	for _, value := range d.formatTable {
		d.formatList = append(d.formatList, value)
	}
	sort.Slice(d.formatList,
		func(i, j int) bool {
			return d.formatList[i].Mask > d.formatList[j].Mask
		})
}

func (d *Disassembler) initializeDecodeTable() {
	d.decodeTables = make(map[formatType]*decodeTable)

	// SOP2 instructions
	d.addInstType(&InstType{"s_add_u32", 0, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_sub_u32", 1, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_add_i32", 2, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_sub_i32", 3, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_addc_u32", 4, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_subb_u32", 5, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_min_i32", 6, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_min_u32", 7, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_max_i32", 8, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_max_u32", 9, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_cselect_b32", 10, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_cselect_b64", 11, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_and_b32", 12, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_and_b64", 13, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_or_b32", 14, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_or_b64", 15, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_xor_b32", 16, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_xor_b64", 17, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_andn2_b32", 18, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_andn2_b64", 19, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_orn2_b32", 20, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_orn2_b64", 21, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_nand_b32", 22, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_nand_b64", 23, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_nor_b32", 24, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_nor_b64", 25, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_xnor_b32", 26, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_xnor_b64", 27, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_lshl_b32", 28, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_lshl_b64", 29, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_lshr_b32", 30, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_lshr_b64", 31, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_ashr_i32", 32, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_ashr_i64", 33, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_bfm_b32", 34, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_bfm_b64", 35, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_mul_i32", 36, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_bfe_u32", 37, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_bfe_i32", 38, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_bfe_u64", 39, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_bfe_i64", 40, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_cbrahcn_g_fork", 41, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_absdiss_i32", 42, d.formatTable[sop2]})
	d.addInstType(&InstType{"s_rfe_restore_b64", 43, d.formatTable[sop2]})

	// VOP2 instructions
	d.addInstType(&InstType{"v_cndmask_b32", 0, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_add_f32", 1, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_sub_f32", 2, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_subrev_f32", 3, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_mul_legacy_f32", 4, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_mul_f32", 5, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_mul_i32_i24", 6, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_mul_hi_i32_i24", 7, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_mul_u32_u24", 8, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_mul_hi_u32_u24", 9, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_min_f32", 10, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_max_f32", 11, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_min_i32", 12, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_max_i32", 13, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_min_u32", 14, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_max_u32", 15, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_lshrrev_b32", 16, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_ashrrev_i32", 17, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_lshlrev_b32", 18, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_and_b32", 19, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_or_b32", 20, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_xor_b32", 21, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_mac_f32", 22, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_madmk_f32", 23, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_madak_f32", 24, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_add_u32", 25, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_sub_u32", 26, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_subrev_u32", 27, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_addc_u32", 28, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_subb_u32", 29, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_subbrev_u32", 30, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_add_f16", 31, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_sub_f16", 32, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_subrev_f16", 33, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_mul_f16", 34, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_mac_f16", 35, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_madmk_f16", 36, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_madak_f16", 37, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_add_u16", 38, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_sub_u16", 39, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_subrev_u16", 40, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_mul_lo_u16", 41, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_lshlrev_b16", 42, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_lshrrev_b16", 43, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_ashrrev_i16", 44, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_max_f16", 45, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_min_f16", 46, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_max_u16", 47, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_max_i16", 48, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_min_u16", 49, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_min_i16", 50, d.formatTable[vop2]})
	d.addInstType(&InstType{"v_ldexp_f16", 51, d.formatTable[vop2]})

	// VOP1 instructions
	d.addInstType(&InstType{"v_nop", 0, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_mov_b32_e32", 1, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_readfirstlane_b32", 2, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_i32_f64", 3, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_f64_i32", 4, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_f32_i32", 5, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_f32_u32", 6, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_u32_f32", 7, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_i32_f32", 8, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_f16_f32", 10, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_f32_f16", 11, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_rpi_i32_f32", 12, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_flr_i32_f32", 13, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_off_f32_i4", 14, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_f32_f64", 15, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_f64_f32", 16, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_f32_ubyte0", 17, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_f32_ubyte1", 18, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_f32_ubyte2", 19, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_f32_ubyte3", 20, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_u32_f64", 21, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_f64_u32", 22, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_trunc_f64", 23, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_ceil_f64", 24, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_rndne_f64", 25, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_floor_f64", 26, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_fract_f32", 27, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_trunc_f32", 28, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_ceil_f32", 29, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_rndne_f32", 30, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_floor_f32", 31, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_exp_f32", 32, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_log_f32", 33, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_rcp_f32", 34, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_rcp_iflag_f32", 35, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_rsq_f32", 36, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_rcp_f64", 37, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_rsq_f64", 38, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_sqrt_f32", 39, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_sqrt_f64", 40, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_sin_f32", 41, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cos_f32", 42, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_not_b32", 43, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_bfrev_b32", 44, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_ffbh_u32", 45, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_ffbl_b32", 46, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_ffbh_i32", 47, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_frexp_exp_i32_f64", 48, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_frexp_mant_f64", 49, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_fract_f64", 50, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_frexp_exp_i32_f32", 51, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_frexp_mant_f32", 52, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_clrexcp", 53, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_movreld_b32", 54, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_movrels_b32", 55, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_movrelsd_b32", 56, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_f16_u16", 57, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_f16_i16", 58, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_u16_f16", 59, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cvt_i16_f16", 60, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_rcp_f16", 61, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_sqrt_f16", 62, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_rsq_f16", 63, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_log_f16", 64, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_exp_f16", 65, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_frexp_mant_f16", 66, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_frexp_exp_i16_f16", 67, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_floor_f16", 68, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_ceil_f16", 69, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_trunc_f16", 70, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_rndne_f16", 71, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_fract_f16", 72, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_sin_f16", 73, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_cos_f16", 74, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_exp_legacy_f32", 75, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_log_legacy_f32", 76, d.formatTable[vop1]})

	// Flat Instructions
	// I am not sure why, but seems the numbers in the official disassembler
	// does not match the documentation. Here follows the official disassembler.
	d.addInstType(&InstType{"flat_load_ubyte", 16, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_load_sbyte", 17, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_load_ushort", 18, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_load_sshort", 19, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_load_dword", 20, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_load_dwordx2", 21, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_load_dwordx4", 22, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_load_dwordx3", 23, d.formatTable[flat]})
	// Unitl here
	d.addInstType(&InstType{"flat_store_byte", 24, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_store_short", 26, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_store_dword", 28, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_store_dwordx2", 29, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_store_dwordx4", 30, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_store_dwordx3", 31, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_swap", 48, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_cmpswap", 49, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_add", 50, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_sub", 51, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_smin", 53, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_umin", 54, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_smax", 55, d.formatTable[flat]})
	d.addInstType(&InstType{"flag_atomic_umax", 56, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_and", 57, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_or", 58, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_xor", 59, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_inc", 60, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_dec", 61, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_swap_x2", 80, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_cmpswap_x2", 81, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_add_x2", 82, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_sub_x2", 83, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_smin_x2", 85, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_umin_x2", 86, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_smax_x2", 87, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_umax_x2", 88, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_and_x2", 89, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_or_x2", 90, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_xor_x2", 91, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_inc_x2", 92, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_atomic_dec_x2", 93, d.formatTable[flat]})

	// SMEM instructions
	d.addInstType(&InstType{"s_load_dword", 0, d.formatTable[smem]})
	d.addInstType(&InstType{"s_load_dwordx2", 1, d.formatTable[smem]})
	d.addInstType(&InstType{"s_load_dwordx4", 2, d.formatTable[smem]})
	d.addInstType(&InstType{"s_load_dwordx8", 3, d.formatTable[smem]})
	d.addInstType(&InstType{"s_load_dwordx16", 4, d.formatTable[smem]})
	d.addInstType(&InstType{"s_buffer_load_dword", 8, d.formatTable[smem]})
	d.addInstType(&InstType{"s_buffer_load_dwordx2", 9, d.formatTable[smem]})
	d.addInstType(&InstType{"s_buffer_load_dwordx4", 10, d.formatTable[smem]})
	d.addInstType(&InstType{"s_buffer_load_dwordx8", 11, d.formatTable[smem]})
	d.addInstType(&InstType{"s_buffer_load_dwordx16", 12, d.formatTable[smem]})
	d.addInstType(&InstType{"s_store_dword", 16, d.formatTable[smem]})
	d.addInstType(&InstType{"s_store_dwordx2", 17, d.formatTable[smem]})
	d.addInstType(&InstType{"s_store_dwordx4", 18, d.formatTable[smem]})
	d.addInstType(&InstType{"s_buffer_store_dword", 24, d.formatTable[smem]})
	d.addInstType(&InstType{"s_buffer_store_dwordx2", 25, d.formatTable[smem]})
	d.addInstType(&InstType{"s_buffer_store_dwordx4", 26, d.formatTable[smem]})
	d.addInstType(&InstType{"s_dcache_inv", 32, d.formatTable[smem]})
	d.addInstType(&InstType{"s_dcache_wb", 33, d.formatTable[smem]})
	d.addInstType(&InstType{"s_dcache_inv_vol", 34, d.formatTable[smem]})
	d.addInstType(&InstType{"s_dcache_wb_vol", 35, d.formatTable[smem]})
	d.addInstType(&InstType{"s_memtime", 36, d.formatTable[smem]})
	d.addInstType(&InstType{"s_memrealtime", 37, d.formatTable[smem]})
	d.addInstType(&InstType{"s_atc_probe", 38, d.formatTable[smem]})
	d.addInstType(&InstType{"s_atc_probe_buffer", 39, d.formatTable[smem]})

	// SOPP instructions
	d.addInstType(&InstType{"s_nop", 0, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_endpgm", 1, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_branch", 2, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_cbranch_scc0", 4, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_cbranch_scc1", 5, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_cbranch_vccz", 6, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_cbranch_vccnz", 7, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_cbranch_execz", 8, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_cbranch_execnz", 9, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_barrier", 10, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_setkill", 11, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_waitcnt", 12, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_sethalt", 13, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_sleep", 14, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_setprio", 15, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_sendmsg", 16, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_sendmsghalt", 17, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_trap", 18, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_icache_inv", 19, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_incperflevel", 20, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_decperflevel", 21, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_ttracedata", 22, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_cbranch_cdbgsys", 23, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_cbranch_cdbguser", 24, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_cbranch_cdbgsys_or_user", 25, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_cbranch_cdbgsys_and_user", 26, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_endpgm_saved", 27, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_set_gpr_idx_off", 28, d.formatTable[sopp]})
	d.addInstType(&InstType{"s_set_gpr_idx_mode", 29, d.formatTable[sopp]})

	// SOPC instructions
	d.addInstType(&InstType{"s_cmp_eq_i32", 0, d.formatTable[sopc]})
	d.addInstType(&InstType{"s_cmp_lg_i32", 1, d.formatTable[sopc]})
	d.addInstType(&InstType{"s_cmp_gt_i32", 2, d.formatTable[sopc]})
	d.addInstType(&InstType{"s_cmp_ge_i32", 3, d.formatTable[sopc]})
	d.addInstType(&InstType{"s_cmp_lt_i32", 4, d.formatTable[sopc]})
	d.addInstType(&InstType{"s_cmp_le_i32", 5, d.formatTable[sopc]})
	d.addInstType(&InstType{"s_cmp_eq_u32", 6, d.formatTable[sopc]})
	d.addInstType(&InstType{"s_cmp_lg_u32", 7, d.formatTable[sopc]})
	d.addInstType(&InstType{"s_cmp_gt_u32", 8, d.formatTable[sopc]})
	d.addInstType(&InstType{"s_cmp_ge_u32", 9, d.formatTable[sopc]})
	d.addInstType(&InstType{"s_cmp_lt_u32", 10, d.formatTable[sopc]})
	d.addInstType(&InstType{"s_cmp_le_u32", 11, d.formatTable[sopc]})
	d.addInstType(&InstType{"s_bitcmp0_b32", 12, d.formatTable[sopc]})
	d.addInstType(&InstType{"s_bitcmp1_b32", 13, d.formatTable[sopc]})
	d.addInstType(&InstType{"s_bitcmp0_b64", 14, d.formatTable[sopc]})
	d.addInstType(&InstType{"s_bitcmp1_b64", 15, d.formatTable[sopc]})
	d.addInstType(&InstType{"s_setvskip", 16, d.formatTable[sopc]})
	d.addInstType(&InstType{"s_set_gpr_idx_on", 17, d.formatTable[sopc]})
	d.addInstType(&InstType{"s_cmp_eq_u64", 18, d.formatTable[sopc]})
	d.addInstType(&InstType{"s_cmp_ne_u64", 19, d.formatTable[sopc]})

	// SOPK instructions
	d.addInstType(&InstType{"s_movk_i32", 0, d.formatTable[sopk]})
	d.addInstType(&InstType{"s_cmovk_i32", 1, d.formatTable[sopk]})
	d.addInstType(&InstType{"s_cmpk_eq_i32", 2, d.formatTable[sopk]})
	d.addInstType(&InstType{"s_cmpk_lg_i32", 3, d.formatTable[sopk]})
	d.addInstType(&InstType{"s_cmpk_gt_i32", 4, d.formatTable[sopk]})
	d.addInstType(&InstType{"s_cmpk_ge_i32", 5, d.formatTable[sopk]})
	d.addInstType(&InstType{"s_cmpk_lt_i32", 6, d.formatTable[sopk]})
	d.addInstType(&InstType{"s_cmpk_le_i32", 7, d.formatTable[sopk]})
	d.addInstType(&InstType{"s_cmpk_eq_u32", 8, d.formatTable[sopk]})
	d.addInstType(&InstType{"s_cmpk_lg_u32", 9, d.formatTable[sopk]})
	d.addInstType(&InstType{"s_cmpk_gt_u32", 10, d.formatTable[sopk]})
	d.addInstType(&InstType{"s_cmpk_ge_u32", 11, d.formatTable[sopk]})
	d.addInstType(&InstType{"s_cmpk_lt_u32", 12, d.formatTable[sopk]})
	d.addInstType(&InstType{"s_cmpk_le_u32", 13, d.formatTable[sopk]})
	d.addInstType(&InstType{"s_addk_i32", 14, d.formatTable[sopk]})
	d.addInstType(&InstType{"s_mulk_i32", 15, d.formatTable[sopk]})
	d.addInstType(&InstType{"s_cbranch_i_fork", 16, d.formatTable[sopk]})
	d.addInstType(&InstType{"s_getreg_b32", 17, d.formatTable[sopk]})
	d.addInstType(&InstType{"s_setreg_b32", 18, d.formatTable[sopk]})
	d.addInstType(&InstType{"s_setreg_imm32_b32", 20, d.formatTable[sopk]})

	// VOPC instructions
	d.addInstType(&InstType{"v_cmp_class_f32", 0x10, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_class_f32", 0x11, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_class_f64", 0x12, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_class_f64", 0x13, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_class_f16", 0x14, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_class_f16", 0x15, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_f_f16", 0x20, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_lt_f16", 0x21, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_eq_f16", 0x22, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_le_f16", 0x23, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_gt_f16", 0x24, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_lg_f16", 0x25, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_ge_f16", 0x26, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_o_f16", 0x27, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_u_f16", 0x28, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_nge_f16", 0x29, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_nlg_f16", 0x2a, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_ngt_f16", 0x2b, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_nle_f16", 0x2c, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_neq_f16", 0x2d, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_nlt_f16", 0x2e, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_tru_f16", 0x2f, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_f_f16", 0x30, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_lt_f16", 0x31, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_eq_f16", 0x32, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_le_f16", 0x33, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_gt_f16", 0x34, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_lg_f16", 0x35, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_ge_f16", 0x36, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_o_f16", 0x37, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_u_f16", 0x38, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_nge_f16", 0x39, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_nlg_f16", 0x3a, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_ngt_f16", 0x3b, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_nle_f16", 0x3c, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_neq_f16", 0x3d, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_nlt_f16", 0x3e, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_tru_f16", 0x3f, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_f_f32", 0x40, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_lt_f32", 0x41, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_eq_f32", 0x42, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_le_f32", 0x43, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_gt_f32", 0x44, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_lg_f32", 0x45, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_ge_f32", 0x46, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_o_f32", 0x47, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_u_f32", 0x48, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_nge_f32", 0x49, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_nlg_f32", 0x4a, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_ngt_f32", 0x4b, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_nle_f32", 0x4c, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_neq_f32", 0x4d, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_nlt_f32", 0x4e, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_tru_f32", 0x4f, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_f_f32", 0x50, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_lt_f32", 0x51, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_eq_f32", 0x52, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_le_f32", 0x53, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_gt_f32", 0x54, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_lg_f32", 0x55, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_ge_f32", 0x56, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_o_f32", 0x57, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_u_f32", 0x58, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_nge_f32", 0x59, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_nlg_f32", 0x5a, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_ngt_f32", 0x5b, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_nle_f32", 0x5c, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_neq_f32", 0x5d, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_nlt_f32", 0x5e, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_tru_f32", 0x5f, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_f_f64", 0x60, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_lt_f64", 0x61, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_eq_f64", 0x62, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_le_f64", 0x63, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_gt_f64", 0x64, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_lg_f64", 0x65, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_ge_f64", 0x66, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_o_f64", 0x67, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_u_f64", 0x68, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_nge_f64", 0x69, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_nlg_f64", 0x6a, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_ngt_f64", 0x6b, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_nle_f64", 0x6c, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_neq_f64", 0x6d, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_nlt_f64", 0x6e, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_tru_f64", 0x6f, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_f_f64", 0x70, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_lt_f64", 0x71, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_eq_f64", 0x72, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_le_f64", 0x73, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_gt_f64", 0x74, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_lg_f64", 0x75, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_ge_f64", 0x76, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_o_f64", 0x77, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_u_f64", 0x78, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_nge_f64", 0x79, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_nlg_f64", 0x7a, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_ngt_f64", 0x7b, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_nle_f64", 0x7c, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_neq_f64", 0x7d, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_nlt_f64", 0x7e, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_tru_f64", 0x7f, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_f_i16", 0xa0, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_lt_i16", 0xa1, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_eq_i16", 0xa2, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_le_i16", 0xa3, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_gt_i16", 0xa4, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_lg_i16", 0xa5, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_ge_i16", 0xa6, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_tru_i16", 0xa7, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_f_u16", 0xa8, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_lt_u16", 0xa9, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_eq_u16", 0xaa, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_le_u16", 0xab, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_gt_u16", 0xac, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_lg_u16", 0xad, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_ge_u16", 0xae, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_tru_u16", 0xaf, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_f_i16", 0xb0, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_lt_i16", 0xb1, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_eq_i16", 0xb2, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_le_i16", 0xb3, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_gt_i16", 0xb4, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_lg_i16", 0xb5, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_ge_i16", 0xb6, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_tru_i16", 0xb7, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_f_u16", 0xb8, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_lt_u16", 0xb9, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_eq_u16", 0xba, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_le_u16", 0xbb, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_gt_u16", 0xbc, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_lg_u16", 0xbd, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_ge_u16", 0xbe, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_tru_u16", 0xbf, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_f_i32", 0xc0, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_lt_i32", 0xc1, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_eq_i32", 0xc2, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_le_i32", 0xc3, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_gt_i32", 0xc4, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_lg_i32", 0xc5, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_ge_i32", 0xc6, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_tru_i32", 0xc7, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_f_u32", 0xc8, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_lt_u32", 0xc9, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_eq_u32", 0xca, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_le_u32", 0xcb, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_gt_u32", 0xcc, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_lg_u32", 0xcd, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_ge_u32", 0xce, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_tru_u32", 0xcf, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_f_i32", 0xd0, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_lt_i32", 0xd1, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_eq_i32", 0xd2, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_le_i32", 0xd3, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_gt_i32", 0xd4, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_lg_i32", 0xd5, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_ge_i32", 0xd6, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_tru_i32", 0xd7, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_f_u32", 0xd8, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_lt_u32", 0xd9, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_eq_u32", 0xda, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_le_u32", 0xdb, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_gt_u32", 0xdc, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_lg_u32", 0xdd, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_ge_u32", 0xde, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_tru_u32", 0xdf, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_f_i64", 0xe0, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_lt_i64", 0xe1, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_eq_i64", 0xe2, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_le_i64", 0xe3, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_gt_i64", 0xe4, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_lg_i64", 0xe5, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_ge_i64", 0xe6, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_tru_i64", 0xe7, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_f_u64", 0xe8, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_lt_u64", 0xe9, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_eq_u64", 0xea, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_le_u64", 0xeb, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_gt_u64", 0xec, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_lg_u64", 0xed, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_ge_u64", 0xee, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmp_tru_u64", 0xef, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_f_i64", 0xf0, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_lt_i64", 0xf1, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_eq_i64", 0xf2, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_le_i64", 0xf3, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_gt_i64", 0xf4, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_lg_i64", 0xf5, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_ge_i64", 0xf6, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_tru_i64", 0xf7, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_f_u64", 0xf8, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_lt_u64", 0xf9, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_eq_u64", 0xfa, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_le_u64", 0xfb, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_gt_u64", 0xfc, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_lg_u64", 0xfd, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_ge_u64", 0xfe, d.formatTable[vopc]})
	d.addInstType(&InstType{"v_cmpx_tru_u64", 0xff, d.formatTable[vopc]})

	// VOP3 Instructions
	for _, instType := range d.decodeTables[vopc].insts {
		d.addInstType(&InstType{instType.InstName, instType.Opcode,
			d.formatTable[vop3]})
	}
	for _, instType := range d.decodeTables[vop2].insts {
		d.addInstType(&InstType{instType.InstName,
			instType.Opcode + Opcode(256),
			d.formatTable[vop3]})
	}
	for _, instType := range d.decodeTables[vop1].insts {
		d.addInstType(&InstType{instType.InstName,
			instType.Opcode + Opcode(320),
			d.formatTable[vop3]})
	}
	d.addInstType(&InstType{"v_mad_legacy_f32", 448, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_mad_f32", 449, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_mad_i32_i24", 450, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_mad_u32_u24", 451, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_cubeid_f32", 452, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_cubesc_f32", 453, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_cubetc_f32", 454, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_cubema_f32", 455, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_bfe_u32", 456, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_bfe_i32", 457, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_bfi_b32", 458, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_fma_f32", 459, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_fma_f64", 460, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_lerp_u8", 461, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_alignbit_b32", 462, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_alignbyte_b32", 463, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_min3_f32", 464, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_min3_i32", 465, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_min3_u32", 466, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_max3_f32", 467, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_max3_i32", 468, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_max3_u32", 469, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_med3_f32", 470, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_med3_i32", 471, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_med3_u32", 472, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_sad_u8", 473, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_sad_hi_u8", 474, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_sad_u16", 475, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_sad_u32", 476, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_cvt_pk_u8_f32", 477, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_div_fixup_f32", 478, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_div_fixup_f64", 479, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_div_scale_f32", 480, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_div_scale_f64", 481, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_div_fmas_f32", 482, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_div_fmas_f64", 483, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_msad_u8", 484, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_qsad_pk_u16_u8", 485, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_mqsad_pk_u16_u8", 486, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_mqsad_u32_u8", 487, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_mad_u64_u32", 488, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_mad_i64_i32", 489, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_mad_f16", 490, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_mad_u16", 491, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_mad_i16", 492, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_perm_b32", 493, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_fma_f16", 494, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_div_fixup_16", 495, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_cvt_pkaccum_u8_f32", 496, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_interp_p1_f32", 624, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_interp_p2_f32", 625, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_interp_mov_f32", 626, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_interp_p1ll_f16", 628, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_interp_p1lv_f16", 629, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_interp_p2_f16", 630, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_add_f64", 640, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_mul_f64", 641, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_min_f64", 642, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_max_f64", 643, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_ldexp_f64", 644, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_mul_lo_u32", 645, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_mul_hi_u32", 646, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_mul_hi_i32", 647, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_ldexp_f32", 648, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_readlane_b32", 649, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_writelane_b32", 650, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_bcnt_u32_b32", 651, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_mbcnt_lo_u32_b32", 652, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_mbcnt_hi_u32_b32", 653, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_lshlrev_b64", 655, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_lshrrev_b64", 656, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_ashrrev_i64", 657, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_trig_preop_f64", 658, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_bfm_b32", 659, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_cvt_pknorm_i16_f32", 660, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_cvt_pknorm_u16_f32", 661, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_cvt_pkrtz_f16_f32", 662, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_cvt_pk_u16_u32", 663, d.formatTable[vop3]})
	d.addInstType(&InstType{"v_cvt_pk_i16_i32", 664, d.formatTable[vop3]})

	// SOP1 Instructions
	d.addInstType(&InstType{"s_mov_b32", 0, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_mov_b64", 1, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_cmov_b32", 2, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_cmov_b64", 3, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_not_b32", 4, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_not_b64", 5, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_wqm_b32", 6, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_wqm_b64 ", 7, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_brev_b32", 8, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_brev_b64", 9, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_bcnt0_i32_b32", 10, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_bcnt0_i32_b64", 11, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_bcnt1_i32_b32", 12, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_bcnt1_i32_b64", 13, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_ff0_i32_b32", 14, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_ff0_i32_b64", 15, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_ff1_i32_b32", 16, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_ff1_i32_b64", 17, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_flbit_i32_b32", 18, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_flbit_i32_b64", 19, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_flbit_i32", 20, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_flbit_i32_i64", 21, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_sext_i32_i8", 22, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_sext_i32_i16", 23, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_bitset0_b32", 24, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_bitset0_b64", 25, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_bitset1_b32", 26, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_bitset1_b64", 27, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_getpc_b64", 28, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_setpc_b64", 29, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_swappc_b64", 30, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_rfe_b64", 31, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_and_saveexec_b64", 32, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_or_saveexec_b64", 33, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_xor_saveexec_b64", 34, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_andn2_saveexec_b64", 35, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_orn2_saveexec_b64", 36, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_nand_saveexec_b64", 37, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_nor_saveexec_b64", 38, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_xnor_saveexec_b64", 39, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_quadmask_b32", 40, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_quadmask_b64", 41, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_movrels_b32", 42, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_movrels_b64", 43, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_movreld_b32", 44, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_movreld_b64", 45, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_cbranch_join", 46, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_abs_i32", 48, d.formatTable[sop1]})
	d.addInstType(&InstType{"s_set_gpr_idx_idx", 49, d.formatTable[sop1]})

}
