package insts

import (
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
)

func extractBits(number uint32, lo uint8, hi uint8) uint32 {
	var mask uint64
	var extracted uint64
	mask = ((1 << (hi - lo + 1)) - 1) << lo
	extracted = (uint64(number) & mask) >> lo
	return uint32(extracted)
}

func extractBit(number uint32, bitPosition uint8) uint32 {
	return number & uint32(bitPosition)
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

func (d *Disassembler) matchFormat(firstFourBytes uint32) (*Format, error) {
	for _, f := range d.formatList {
		if f.FormatType == VOP3b { // Skip VOP3b this time.
			continue
		}

		if (firstFourBytes^f.Encoding)&f.Mask == 0 {
			// Consider VOP3b
			if f.FormatType == VOP3a {
				opcode := f.retrieveOpcode(firstFourBytes)
				if d.isVOP3bOpcode(opcode) {
					return FormatTable[VOP3b], nil
				}
			}

			return f, nil
		}
	}

	bytesString := fmt.Sprintf("%04x", firstFourBytes)
	return nil, errors.New("cannot find the instruction format, first two " +
		"bytes are " + bytesString)
}

func (d *Disassembler) lookUp(
	format *Format,
	opcode Opcode,
) (*InstType, error) {
	if d.decodeTables[format.FormatType] != nil &&
		d.decodeTables[format.FormatType].insts[opcode] != nil {
		return d.decodeTables[format.FormatType].insts[opcode], nil
	}

	return nil, fmt.Errorf("instruction format %s, opcode %d not found",
		format.FormatName, opcode)
}

func (d *Disassembler) decodeSOP2(inst *Inst, buf []byte) error {
	bytes := binary.LittleEndian.Uint32(buf)

	src0Value := extractBits(bytes, 0, 7)
	inst.Src0, _ = getOperand(uint16(src0Value))
	if inst.Src0.OperandType == LiteralConstant {
		inst.ByteSize += 4
		if len(buf) < 8 {
			return errors.New("no enough bytes")
		}
		inst.Src0.LiteralConstant = BytesToUint32(buf[4:8])
	}

	src1Value := extractBits(bytes, 8, 15)
	inst.Src1, _ = getOperand(uint16(src1Value))
	if inst.Src1.OperandType == LiteralConstant {
		inst.ByteSize += 4
		if len(buf) < 8 {
			return errors.New("no enough bytes")
		}
		inst.Src1.LiteralConstant = BytesToUint32(buf[4:8])
	}

	sdstValue := extractBits(bytes, 16, 22)
	inst.Dst, _ = getOperand(uint16(sdstValue))

	if strings.Contains(inst.InstName, "64") {
		inst.Src0.RegCount = 2
		inst.Src1.RegCount = 2
		inst.Dst.RegCount = 2
	}
	return nil
}

func (d *Disassembler) decodeVOP1(inst *Inst, buf []byte) error {
	bytes := binary.LittleEndian.Uint32(buf)

	src0Value := extractBits(bytes, 0, 8)

	inst.Src0, _ = getOperand(uint16(src0Value))
	if inst.Src0.OperandType == LiteralConstant {
		inst.ByteSize += 4
		if len(buf) < 8 {
			return errors.New("no enough bytes")
		}
		inst.Src0.LiteralConstant = BytesToUint32(buf[4:8])
	}
	if inst.SRC0Width == 64 {
		inst.Src0.RegCount = 2
	}

	dstValue := extractBits(bytes, 17, 24)
	switch inst.Opcode {
	case 2: // v_readfirstlane_b32
		inst.Dst, _ = getOperand(uint16(dstValue))
	default:
		inst.Dst, _ = getOperand(uint16(dstValue + 256))
	}
	if inst.DSTWidth == 64 {
		inst.Dst.RegCount = 2
	}

	switch inst.Opcode {
	case 4: // v_cvt_f64_i32_e32
		inst.Dst.RegCount = 2
	case 15: // v_cvt_f32_f64_e32
		inst.Src0.RegCount = 2
	case 16: // v_cvt_f64_f32_e32
		inst.Dst.RegCount = 2
	}

	return nil
}

//nolint:gocyclo,funlen
func (d *Disassembler) decodeVOP2(inst *Inst, buf []byte) error {
	bytes := binary.LittleEndian.Uint32(buf)

	operandBits := uint16(extractBits(bytes, 0, 8))
	if operandBits == 249 {
		if len(buf) < 8 {
			return errors.New("no enough bytes")
		}
		inst.IsSdwa = true
		sdwaBytes := binary.LittleEndian.Uint32(buf[4:8])
		src0Bits := int(extractBits(sdwaBytes, 0, 7))
		inst.Src0 = NewVRegOperand(src0Bits, src0Bits, 0)

		dstSel := int(extractBits(sdwaBytes, 8, 10))
		dstUnused := int(extractBits(sdwaBytes, 11, 12))
		clamp := int(extractBits(sdwaBytes, 13, 13))
		src0Sel := int(extractBits(sdwaBytes, 16, 18))
		src0Sext := int(extractBits(sdwaBytes, 19, 19))
		src0Neg := int(extractBits(sdwaBytes, 20, 20))
		src0Abs := int(extractBits(sdwaBytes, 21, 21))
		src1Sel := int(extractBits(sdwaBytes, 24, 26))
		src1Sext := int(extractBits(sdwaBytes, 27, 27))
		src1Neg := int(extractBits(sdwaBytes, 28, 28))
		src1Abs := int(extractBits(sdwaBytes, 29, 29))

		switch dstSel {
		case 0:
			inst.DstSel = 0xff
		case 1:
			inst.DstSel = 0xff00
		case 2:
			inst.DstSel = 0xff0000
		case 3:
			inst.DstSel = 0xff000000
		case 4:
			inst.DstSel = 0xffff
		case 5:
			inst.DstSel = 0xFFFF0000
		case 6:
			inst.DstSel = 0xFFFFFFFF
		}

		switch dstUnused {
		case 0:
		case 1:
			log.Panicf("DST_UNUSED SEXT is not implemented")
		case 2:
			log.Panicf("DST_UNUSED PRESERVE is not implemented")
		}

		switch clamp {
		case 0:
		case 1:
			log.Panicf("CLAMP is not implemented")
		}

		switch src0Sel {
		case 0:
			inst.Src0Sel = 0xff
		case 1:
			inst.Src0Sel = 0xff00
		case 2:
			inst.Src0Sel = 0xff0000
		case 3:
			inst.Src0Sel = 0xff000000
		case 4:
			inst.Src0Sel = 0xffff
		case 5:
			inst.Src0Sel = 0xFFFF0000
		case 6:
			inst.Src0Sel = 0xFFFFFFFF
		}

		switch src0Sext {
		case 0:
		case 1:
			log.Panicf("SRC0_SEXT is not implemented")
		}

		switch src0Neg {
		case 0:
		case 1:
			log.Panicf("SRC0_NEG when true is not implemented")
		}

		switch src0Abs {
		case 0:
		case 1:
			log.Panicf("SRC0_ABS is not implemented")
		}

		switch src1Sel {
		case 0:
			inst.Src1Sel = 0xff
		case 1:
			inst.Src1Sel = 0xff00
		case 2:
			inst.Src1Sel = 0xff0000
		case 3:
			inst.Src1Sel = 0xff000000
		case 4:
			inst.Src1Sel = 0xffff
		case 5:
			inst.Src1Sel = 0xFFFF0000
		case 6:
			inst.Src1Sel = 0xFFFFFFFF
		}

		switch src1Sext {
		case 0:
		case 1:
			log.Panicf("SRC1_SEXT is not implemented")
		}

		switch src1Neg {
		case 0:
		case 1:
			log.Panicf("SRC1_NEG when true is not implemented")
		}

		switch src1Abs {
		case 0:
		case 1:
			log.Panicf("SRC1_ABS is not implemented")
		}

		inst.ByteSize += 4
	} else {
		inst.Src0, _ = getOperand(operandBits)
	}

	if inst.Src0.OperandType == LiteralConstant {
		inst.ByteSize += 4
		if len(buf) < 8 {
			return errors.New("no enough bytes")
		}
		inst.Src0.LiteralConstant = BytesToUint32(buf[4:8])
	}

	bits := int(extractBits(bytes, 9, 16))
	inst.Src1 = NewVRegOperand(bits, bits, 0)

	bits = int(extractBits(bytes, 17, 24))
	inst.Dst = NewVRegOperand(bits, bits, 0)

	switch inst.Opcode {
	case 24, 37: // v_madak
		inst.Imm = true
		inst.ByteSize += 4
		inst.Src2 = &Operand{0, LiteralConstant, nil, 0, 0, 0, 0}
		if len(buf) < 8 {
			return errors.New("no enough bytes")
		}

		inst.Src2.LiteralConstant = BytesToUint32(buf[4:8])
	}

	return nil
}

func (d *Disassembler) decodeFLAT(inst *Inst, buf []byte) error {
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
		inst.Data.RegCount = 3
		inst.Dst.RegCount = 3
	case 23, 31:
		inst.Data.RegCount = 4
		inst.Dst.RegCount = 4
	}
	return nil
}

//nolint:gocyclo,funlen
func (d *Disassembler) decodeSMEM(inst *Inst, buf []byte) error {
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
		if len(buf) < 8 {
			return errors.New("no enough bytes")
		}
		inst.Data.LiteralConstant = BytesToUint32(buf[4:8])
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
	return nil
}

func (d *Disassembler) decodeSOPP(inst *Inst, buf []byte) error {
	bytes := binary.LittleEndian.Uint32(buf)

	inst.SImm16 = NewIntOperand(0, int64(extractBits(bytes, 0, 15)))

	if inst.Opcode == 12 { // WAIT_CNT
		inst.VMCNT = int(extractBits(uint32(inst.SImm16.IntValue), 0, 3))
		inst.LKGMCNT = int(extractBits(uint32(inst.SImm16.IntValue), 8, 12))
	}

	return nil
}

func (d *Disassembler) decodeVOPC(inst *Inst, buf []byte) error {
	bytes := binary.LittleEndian.Uint32(buf)
	inst.Src0, _ = getOperand(uint16(extractBits(bytes, 0, 8)))
	if inst.Src0.OperandType == LiteralConstant {
		inst.ByteSize += 4
		if len(buf) < 8 {
			return errors.New("no enough bytes")
		}
		inst.Src0.LiteralConstant = BytesToUint32(buf[4:8])
	}

	bits := int(extractBits(bytes, 9, 16))
	inst.Src1 = NewVRegOperand(bits, bits, 0)
	return nil
}

func (d *Disassembler) decodeSOPC(inst *Inst, buf []byte) error {
	bytes := binary.LittleEndian.Uint32(buf)
	inst.Src0, _ = getOperand(uint16(extractBits(bytes, 0, 7)))
	if inst.Src0.OperandType == LiteralConstant {
		inst.ByteSize += 4
		if len(buf) < 8 {
			return errors.New("no enough bytes")
		}
		inst.Src0.LiteralConstant = BytesToUint32(buf[4:8])
	}

	inst.Src1, _ = getOperand(uint16(extractBits(bytes, 8, 15)))
	if inst.Src1.OperandType == LiteralConstant {
		inst.ByteSize += 4
		if len(buf) < 8 {
			return errors.New("no enough bytes")
		}
		inst.Src1.LiteralConstant = BytesToUint32(buf[4:8])
	}
	return nil
}

func (d *Disassembler) isVOP3bOpcode(opcode Opcode) bool {
	//if opcode < 255 {
	//	return true
	//}

	switch opcode {
	case 281, 282, 283, 284, 285, 286, 480, 481:
		return true
	}

	return false
}

func (d *Disassembler) decodeVOP3b(inst *Inst, buf []byte) error {
	bytesLo := binary.LittleEndian.Uint32(buf)
	bytesHi := binary.LittleEndian.Uint32(buf[4:])

	if inst.Opcode > 255 {
		dstBits := int(extractBits(bytesLo, 0, 7))
		inst.Dst = NewVRegOperand(dstBits, dstBits, 1)
		if inst.DSTWidth == 64 {
			inst.Dst.RegCount = 2
		}
	}

	inst.SDst, _ = getOperand(uint16(extractBits(bytesLo, 8, 14)))
	if inst.SDSTWidth == 64 {
		inst.SDst.RegCount = 2
	}

	if extractBits(bytesLo, 15, 15) != 0 {
		inst.Clamp = true
	}

	inst.Src0, _ = getOperand(uint16(extractBits(bytesHi, 0, 8)))
	if inst.SRC0Width == 64 {
		inst.Src0.RegCount = 2
	}

	inst.Src1, _ = getOperand(uint16(extractBits(bytesHi, 9, 17)))
	if inst.SRC1Width == 64 {
		inst.Src1.RegCount = 2
	}

	if inst.Opcode > 255 && inst.SRC2Width > 0 {
		inst.Src2, _ = getOperand(uint16(extractBits(bytesHi, 18, 26)))
		if inst.SRC2Width == 64 {
			inst.Src2.RegCount = 2
		}
	}

	inst.Omod = int(extractBits(bytesHi, 27, 28))
	inst.Neg = int(extractBits(bytesHi, 29, 31))
	return nil
}

func (d *Disassembler) decodeVOP3a(inst *Inst, buf []byte) error {
	bytesLo := binary.LittleEndian.Uint32(buf)
	bytesHi := binary.LittleEndian.Uint32(buf[4:])

	bits := int(extractBits(bytesLo, 0, 7))
	if inst.Opcode <= 255 {
		inst.Dst, _ = getOperand(uint16(bits))
	} else {
		inst.Dst = NewVRegOperand(bits, bits, 0)
	}
	if inst.DSTWidth == 64 {
		inst.Dst.RegCount = 2
	}

	inst.Abs = int(extractBits(bytesLo, 8, 10))
	d.parseAbs(inst, inst.Abs)

	if extractBits(bytesLo, 15, 15) != 0 {
		inst.Clamp = true
	}

	inst.Src0, _ = getOperand(uint16(extractBits(bytesHi, 0, 8)))
	if inst.SRC0Width == 64 {
		inst.Src0.RegCount = 2
	}
	inst.Src1, _ = getOperand(uint16(extractBits(bytesHi, 9, 17)))
	if inst.SRC1Width == 64 {
		inst.Src1.RegCount = 2
	}

	if inst.SRC2Width != 0 {
		inst.Src2, _ = getOperand(uint16(extractBits(bytesHi, 18, 26)))
		if inst.SRC2Width == 64 {
			inst.Src2.RegCount = 2
		}
	}

	inst.Omod = int(extractBits(bytesHi, 27, 28))
	inst.Neg = int(extractBits(bytesHi, 29, 31))
	d.parseNeg(inst, inst.Neg)

	return nil
}

func (d *Disassembler) parseNeg(inst *Inst, neg int) {
	if neg&0b001 > 0 {
		inst.Src0Neg = true
	}

	if neg&0b010 > 0 {
		inst.Src1Neg = true
	}

	if neg&0b100 > 0 {
		inst.Src2Neg = true
	}
}

func (d *Disassembler) parseAbs(inst *Inst, abs int) {
	if abs&0b001 > 0 {
		inst.Src0Abs = true
	}

	if abs&0b010 > 0 {
		inst.Src1Abs = true
	}

	if abs&0b100 > 0 {
		inst.Src2Abs = true
	}
}

func (d *Disassembler) decodeSOP1(inst *Inst, buf []byte) error {
	bytes := binary.LittleEndian.Uint32(buf)

	inst.Src0, _ = getOperand(uint16(extractBits(bytes, 0, 7)))
	if inst.SRC0Width == 64 {
		inst.Src0.RegCount = 2
	}

	inst.Dst, _ = getOperand(uint16(extractBits(bytes, 16, 22)))
	if inst.DSTWidth == 64 {
		inst.Dst.RegCount = 2
	}

	if inst.Src0.OperandType == LiteralConstant {
		inst.ByteSize += 4
		if len(buf) < 8 {
			return errors.New("no enough bytes")
		}
		inst.Src0.LiteralConstant = BytesToUint32(buf[4:8])
	}
	return nil
}

func (d *Disassembler) decodeSOPK(inst *Inst, buf []byte) error {
	bytes := binary.LittleEndian.Uint32(buf)
	inst.SImm16 = NewIntOperand(0, int64(extractBits(bytes, 0, 15)))
	inst.Dst, _ = getOperand(uint16(extractBits(bytes, 16, 22)))
	return nil
}

func (d *Disassembler) decodeDS(inst *Inst, buf []byte) error {
	bytesLo := binary.LittleEndian.Uint32(buf)
	bytesHi := binary.LittleEndian.Uint32(buf[4:])

	inst.Offset0 = extractBits(bytesLo, 0, 7)
	inst.Offset1 = extractBits(bytesLo, 8, 15)
	d.combineDSOffsets(inst)

	gdsBit := extractBit(bytesLo, 16)
	if gdsBit != 0 {
		inst.GDS = true
	}

	addrBits := int(extractBits(bytesHi, 0, 7))
	inst.Addr = NewVRegOperand(addrBits, addrBits, 1)

	if inst.SRC0Width > 0 {
		data0Bits := int(extractBits(bytesHi, 8, 15))
		inst.Data = NewVRegOperand(data0Bits, data0Bits, 1)
		d.setRegCountFromWidth(inst.Data, inst.SRC0Width)
	}

	if inst.SRC1Width > 0 {
		data1Bits := int(extractBits(bytesHi, 16, 23))
		inst.Data1 = NewVRegOperand(data1Bits, data1Bits, 1)
		d.setRegCountFromWidth(inst.Data1, inst.SRC1Width)
	}

	if inst.DSTWidth > 0 {
		dstBits := int(extractBits(bytesHi, 24, 31))
		inst.Dst = NewVRegOperand(dstBits, dstBits, 1)
		d.setRegCountFromWidth(inst.Dst, inst.DSTWidth)
	}

	return nil
}

func (d *Disassembler) setRegCountFromWidth(operand *Operand, width int) {
	switch width {
	case 64:
		operand.RegCount = 2
	case 96:
		operand.RegCount = 3
	case 128:
		operand.RegCount = 4
	default:
		operand.RegCount = 1
	}
}

func (d *Disassembler) combineDSOffsets(inst *Inst) {
	switch inst.Opcode {
	default:
		inst.Offset0 += inst.Offset1 << 8
	case 14, 15, 46, 47, 55, 56, 78, 79, 110, 111, 119, 120:
		// do nothing
	}
}

// Decode parses the head of the buffer and returns the next instruction
//
//nolint:gocyclo,funlen
func (d *Disassembler) Decode(buf []byte) (*Inst, error) {
	format, err := d.matchFormat(binary.LittleEndian.Uint32(buf[:4]))
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

	if inst.ByteSize > len(buf) {
		return nil, errors.New("no enough buffer")
	}

	switch format.FormatType {
	case SOP2:
		err = d.decodeSOP2(inst, buf)
	case SMEM:
		err = d.decodeSMEM(inst, buf)
	case VOP2:
		err = d.decodeVOP2(inst, buf)
	case VOP1:
		err = d.decodeVOP1(inst, buf)
	case FLAT:
		err = d.decodeFLAT(inst, buf)
	case SOPP:
		err = d.decodeSOPP(inst, buf)
	case VOPC:
		err = d.decodeVOPC(inst, buf)
	case SOPC:
		err = d.decodeSOPC(inst, buf)
	case VOP3a:
		err = d.decodeVOP3a(inst, buf)
	case VOP3b:
		err = d.decodeVOP3b(inst, buf)
	case SOP1:
		err = d.decodeSOP1(inst, buf)
	case SOPK:
		err = d.decodeSOPK(inst, buf)
	case DS:
		err = d.decodeDS(inst, buf)
	default:
		log.Panicf("unabkle to decode instruction type %s", inst.FormatName)
		break
	}

	if err != nil {
		return nil, err
	}

	return inst, nil
}

// Disassemble take a binary file as an input and put the assembly code in a
// writer
func (d *Disassembler) Disassemble(
	file *elf.File,
	filename string,
	w io.Writer,
) {
	fmt.Fprintf(w, "\n%s:\tfile format ELF64-amdgpu\n", filename)
	fmt.Fprintf(w, "\n\nDisassembly of section .text:\n")

	sec := file.Section(".text")
	data, _ := sec.Data()
	co := NewHsaCoFromData(data)

	buf := co.InstructionData()
	pc := uint64(0x100)
	d.tryPrintSymbol(file, sec.Offset, w)
	for len(buf) > 0 {
		d.tryPrintSymbol(file, sec.Offset+pc, w)

		if d.isNewKenrelStart(file, sec.Offset+pc) {
			buf = buf[0x100:]
			pc += 0x100
		}

		inst, err := d.Decode(buf)
		inst.PC = pc + sec.Offset
		if err != nil {
			fmt.Printf("Instruction not decodable\n")
			buf = buf[4:]
			pc += 4
		} else {
			instStr := inst.String(file)
			fmt.Fprintf(w, "\t%s", instStr)
			for i := len(instStr); i < 59; i++ {
				fmt.Fprint(w, " ")
			}

			fmt.Fprintf(w, "// %012X: ", sec.Offset+pc)
			fmt.Fprintf(w, "%08X", binary.LittleEndian.Uint32(buf[0:4]))
			if inst.ByteSize == 8 {
				fmt.Fprintf(w, " %08X", binary.LittleEndian.Uint32(buf[4:8]))
			}
			fmt.Fprintf(w, "\n")
			buf = buf[inst.ByteSize:]
			pc += uint64(inst.ByteSize)
		}
	}
}

func (d *Disassembler) tryPrintSymbol(
	file *elf.File,
	offset uint64,
	w io.Writer,
) {
	symbols, _ := file.Symbols()
	for _, symbol := range symbols {
		if symbol.Value == offset {
			if d.isKernelSymbol(symbol) {
				fmt.Fprintf(w, "\n%016x %s:\n", offset+0x100, symbol.Name)
			} else {
				fmt.Fprintf(w, "\n%016x %s:\n", offset, symbol.Name)
			}
		}
	}
}

func (d *Disassembler) isKernelSymbol(symbol elf.Symbol) bool {
	return symbol.Size > 0
}

func (d *Disassembler) isNewKenrelStart(file *elf.File, offset uint64) bool {
	symbols, _ := file.Symbols()
	for _, symbol := range symbols {
		if symbol.Value == offset && symbol.Size > 0 {
			return true
		}
	}
	return false
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
