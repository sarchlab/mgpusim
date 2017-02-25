package disasm

import (
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
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

	ssrc0Value := extractBits(bytes, 0, 7)
	inst.SSRC0, _ = getOperand(uint16(ssrc0Value))
	if inst.SSRC0.OperandType == LiteralConstant {
		inst.ByteSize += 4
	}

	ssrc1Value := extractBits(bytes, 8, 15)
	inst.SSRC1, _ = getOperand(uint16(ssrc1Value))
	if inst.SSRC1.OperandType == LiteralConstant {
		inst.ByteSize += 4
	}

	sdstValue := extractBits(bytes, 16, 22)
	inst.SDST, _ = getOperand(uint16(sdstValue))
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
		fmt.Fprint(os.Stderr, err)
		fmt.Fprintf(os.Stderr, " %032b\n",
			binary.LittleEndian.Uint32(buf))

		return nil, err
	}

	inst := new(Instruction)
	inst.Format = format
	inst.InstType = instType
	inst.ByteSize = format.ByteSizeExLiteral

	switch format.FormatType {
	case sop2:
		d.decodeSop2(inst, buf)
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

			instructionData := co.InstructionData()
			for len(instructionData) > 0 {
				inst, err := d.Decode(instructionData)
				if err != nil {
					instructionData = instructionData[4:]
				} else {
					fmt.Println(inst)
					instructionData = instructionData[inst.ByteSize:]
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
	d.formatTable[vop3] = &Format{vop3, "vop3", 0xD000, 0xFC00, 4, 16, 25}
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

	// SOP instructions
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
}
