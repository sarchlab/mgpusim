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
	case vop1:
		d.decodeVop1(inst, buf)
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

	// VOP1 instructions
	d.addInstType(&InstType{"v_nop", 0, d.formatTable[vop1]})
	d.addInstType(&InstType{"v_mov_b32", 1, d.formatTable[vop1]})
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
	d.addInstType(&InstType{"flat_load_ubyte", 8, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_load_sbyte", 9, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_load_ushort", 10, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_load_sshort", 11, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_load_dword", 12, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_load_dwordx2", 13, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_load_dwordx4", 14, d.formatTable[flat]})
	d.addInstType(&InstType{"flat_load_dwordx3", 15, d.formatTable[flat]})
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
}
