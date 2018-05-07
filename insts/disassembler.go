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
	var mask uint32
	var extracted uint32
	mask = ((1 << (hi - lo + 1)) - 1) << lo
	extracted = (number & mask) >> lo
	return extracted
}

func extractBit(number uint32, bit_position uint8) uint32 {
	return number & uint32(bit_position)
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

func (d *Disassembler) lookUp(format *Format, opcode Opcode) (*InstType, error) {
	if d.decodeTables[format.FormatType] != nil &&
		d.decodeTables[format.FormatType].insts[opcode] != nil {
		return d.decodeTables[format.FormatType].insts[opcode], nil
	}

	return nil, fmt.Errorf("instruction format %s, opcode %d not found",
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

func (d *Disassembler) decodeVOP1(inst *Inst, buf []byte) {
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

func (d *Disassembler) decodeVOP2(inst *Inst, buf []byte) {
	bytes := binary.LittleEndian.Uint32(buf)

	operand_bits := uint16(extractBits(bytes, 0, 8))
	if operand_bits == 249 {
		inst.IsSdwa = true
		sdwa_bytes := binary.LittleEndian.Uint32(buf[4:8])
		src0_bits := int(extractBits(sdwa_bytes, 0, 7))
		inst.Src0 = NewVRegOperand(src0_bits, src0_bits, 0)

		dst_sel := int(extractBits(sdwa_bytes, 8, 10))
		dst_unused := int(extractBits(sdwa_bytes, 11, 12))
		clamp := int(extractBits(sdwa_bytes, 13, 13))
		src0_sel := int(extractBits(sdwa_bytes, 16, 18))
		src0_sext := int(extractBits(sdwa_bytes, 19, 19))
		src0_neg := int(extractBits(sdwa_bytes, 20, 20))
		src0_abs := int(extractBits(sdwa_bytes, 21, 21))
		src1_sel := int(extractBits(sdwa_bytes, 24, 26))
		src1_sext := int(extractBits(sdwa_bytes, 27, 27))
		src1_neg := int(extractBits(sdwa_bytes, 28, 28))
		src1_abs := int(extractBits(sdwa_bytes, 29, 29))

		switch dst_sel {
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

		switch dst_unused {
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

		switch src0_sel {
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

		switch src0_sext {
		case 0:
		case 1:
			log.Panicf("SRC0_SEXT is not implemented")
		}

		switch src0_neg {
		case 0:
		case 1:
			log.Panicf("SRC0_NEG when true is not implemented")
		}

		switch src0_abs {
		case 0:
		case 1:
			log.Panicf("SRC0_ABS is not implemented")
		}

		switch src1_sel {
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

		switch src1_sext {
		case 0:
		case 1:
			log.Panicf("SRC1_SEXT is not implemented")
		}

		switch src1_neg {
		case 0:
		case 1:
			log.Panicf("SRC1_NEG when true is not implemented")
		}

		switch src1_abs {
		case 0:
		case 1:
			log.Panicf("SRC1_ABS is not implemented")
		}

		inst.ByteSize += 4

	} else {
		inst.Src0, _ = getOperand(operand_bits)
	}

	if inst.Src0.OperandType == LiteralConstant {
		inst.ByteSize += 4
		inst.Src0.LiteralConstant = BytesToUint32(buf[4:8])
	}

	bits := int(extractBits(bytes, 9, 16))
	inst.Src1 = NewVRegOperand(bits, bits, 0)
	bits = int(extractBits(bytes, 17, 24))
	inst.Dst = NewVRegOperand(bits, bits, 0)
}

func (d *Disassembler) decodeFLAT(inst *Inst, buf []byte) {
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
}

func (d *Disassembler) decodeSMEM(inst *Inst, buf []byte) {
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

func (d *Disassembler) decodeSOPP(inst *Inst, buf []byte) {
	bytes := binary.LittleEndian.Uint32(buf)

	inst.SImm16 = NewIntOperand(0, int64(extractBits(bytes, 0, 15)))
}

func (d *Disassembler) decodeVOPC(inst *Inst, buf []byte) {
	bytes := binary.LittleEndian.Uint32(buf)
	inst.Src0, _ = getOperand(uint16(extractBits(bytes, 0, 8)))
	if inst.Src0.OperandType == LiteralConstant {
		inst.ByteSize += 4
		inst.Src0.LiteralConstant = BytesToUint32(buf[4:8])
	}

	bits := int(extractBits(bytes, 9, 16))
	inst.Src1 = NewVRegOperand(bits, bits, 0)
}

func (d *Disassembler) decodeSOPC(inst *Inst, buf []byte) {
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

func (d *Disassembler) decodeVOP3b(inst *Inst, buf []byte) {
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

	if inst.Opcode > 255 {
		inst.Src2, _ = getOperand(uint16(extractBits(bytesHi, 18, 26)))
		if inst.SRC2Width == 64 {
			inst.Src2.RegCount = 2
		}
	}

	inst.Omod = int(extractBits(bytesHi, 27, 28))
	inst.Neg = int(extractBits(bytesHi, 29, 31))
}

func (d *Disassembler) decodeVOP3a(inst *Inst, buf []byte) {
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
}

func (d *Disassembler) decodeSOP1(inst *Inst, buf []byte) {
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
		inst.Src0.LiteralConstant = BytesToUint32(buf[4:8])
	}
}

func (d *Disassembler) decodeSopk(inst *Inst, buf []byte) {
	bytes := binary.LittleEndian.Uint32(buf)
	inst.SImm16 = NewIntOperand(0, int64(extractBits(bytes, 0, 15)))
	inst.Dst, _ = getOperand(uint16(extractBits(bytes, 16, 22)))
}

func (d *Disassembler) decodeDS(inst *Inst, buf []byte) {
	bytesLo := binary.LittleEndian.Uint32(buf)
	bytesHi := binary.LittleEndian.Uint32(buf[4:])

	inst.Offset0 = uint8(extractBits(bytesLo, 0, 7))
	inst.Offset1 = uint8(extractBits(bytesLo, 8, 16))

	gdsBit := extractBit(bytesLo, 16)
	if gdsBit != 0 {
		inst.GDS = true
	}

	addrBits := int(extractBits(bytesHi, 0, 8))
	inst.Addr = NewVRegOperand(addrBits, addrBits, 1)

	if inst.SRC0Width > 0 {
		data0Bits := int(extractBits(bytesHi, 8, 16))
		inst.Data = NewVRegOperand(data0Bits, data0Bits, 1)
		if inst.SRC0Width == 64 {
			inst.Data.RegCount = 2
		}
	}

	if inst.SRC1Width > 0 {
		data1Bits := int(extractBits(bytesHi, 16, 24))
		inst.Data1 = NewVRegOperand(data1Bits, data1Bits, 1)
		if inst.SRC1Width == 64 {
			inst.Data1.RegCount = 2
		}
	}

	dstBits := int(extractBits(bytesHi, 24, 32))
	inst.Dst = NewVRegOperand(dstBits, dstBits, 1)
	if inst.DSTWidth == 64 {
		inst.Dst.RegCount = 2
	} else if inst.DSTWidth == 128 {
		inst.Dst.RegCount = 4
	}
}

// Decode parses the head of the buffer and returns the next instruction
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

	switch format.FormatType {
	case SOP2:
		d.decodeSop2(inst, buf)
	case SMEM:
		d.decodeSMEM(inst, buf)
	case VOP2:
		d.decodeVOP2(inst, buf)
	case VOP1:
		d.decodeVOP1(inst, buf)
	case FLAT:
		d.decodeFLAT(inst, buf)
	case SOPP:
		d.decodeSOPP(inst, buf)
	case VOPC:
		d.decodeVOPC(inst, buf)
	case SOPC:
		d.decodeSOPC(inst, buf)
	case VOP3a:
		d.decodeVOP3a(inst, buf)
	case VOP3b:
		d.decodeVOP3b(inst, buf)
	case SOP1:
		d.decodeSOP1(inst, buf)
	case SOPK:
		d.decodeSopk(inst, buf)
	case DS:
		d.decodeDS(inst, buf)
	default:
		log.Panicf("unabkle to decode instruction type %s", inst.FormatName)
		break
	}

	return inst, nil
}

// Disassemble take a binary file as an input and put the assembly code in a
// writer
func (d *Disassembler) Disassemble(file *elf.File, w io.Writer) {

	sec := file.Section(".text")
	data, _ := sec.Data()
	co := NewHsaCoFromData(data)

	buf := co.InstructionData()
	pc := uint64(0x100)
	d.tryPrintSymbol(file, sec.Offset, w)
	for len(buf) > 0 {
		d.tryPrintSymbol(file, sec.Offset+pc, w)
		inst, err := d.Decode(buf)
		if err != nil {
			fmt.Printf("Instuction not decodable\n")
			buf = buf[4:]
			pc += 4
		} else {
			// fmt.Fprintf(w, "%s %08b\n", inst, buf[0:inst.ByteSize])
			//fmt.Fprintf(w, "0x%016x\t%s\n", pc, inst)
			instStr := inst.String()
			fmt.Fprintf(w, "\t%s", instStr)
			for i := len(instStr); i < 59; i++ {
				fmt.Fprint(w, " ")
			}

			fmt.Fprintf(w, "// %012X: ", sec.Offset+pc)
			fmt.Fprintf(w, "%08X ", binary.LittleEndian.Uint32(buf[0:4]))
			if inst.ByteSize == 8 {
				fmt.Fprintf(w, "%08X ", binary.LittleEndian.Uint32(buf[4:8]))
			}
			fmt.Fprintf(w, "\n")
			buf = buf[inst.ByteSize:]
			pc += uint64(inst.ByteSize)
		}
	}
}

func (d *Disassembler) tryPrintSymbol(file *elf.File, offset uint64, w io.Writer) {
	symbols, _ := file.Symbols()
	for _, symbol := range symbols {
		if symbol.Value == offset {
			fmt.Fprintf(w, "%s:\n", symbol.Name)
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
