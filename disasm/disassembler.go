package disasm

import (
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
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
	formatType formatType
	Name       string
	Encoding   uint16
	Mask       uint16
	OpcodeLow  uint8
	OpcodeHigh uint8
}

func (f *Format) retrieveOpcode(firstFourBytes uint32) Opcode {
	var mask uint32
	var opcode uint32
	mask = ((1 << (f.OpcodeHigh - f.OpcodeLow + 1)) - 1) << f.OpcodeLow
	opcode = (firstFourBytes & mask) >> f.OpcodeLow
	return Opcode(opcode)
}

// Opcode is the opcode of a GCN3 Instruction
type Opcode uint16

// A InstType represents an instruction type. For example s_barrier instruction
// is a intruction type
type InstType struct {
	Name   string
	Opcode Opcode
	Format *Format
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

	// Maps from the format to table
	decodeTables map[formatType]*decodeTable
}

func (d *Disassembler) addInstType(info *InstType) {
	if d.decodeTables[info.Format.formatType] == nil {
		d.decodeTables[info.Format.formatType] = newDecodeTable()
	}
	d.decodeTables[info.Format.formatType].insts[info.Opcode] = info
}

func (d *Disassembler) initializeFormatTable() {
	d.formatTable = make(map[formatType]*Format)
	d.formatTable[sop1] = &Format{sop1, "sop1", 0xBE80, 0xFF80, 8, 15}
	d.formatTable[sopc] = &Format{sopc, "sopc", 0xBF00, 0xFF80, 16, 22}
	d.formatTable[sopp] = &Format{sopp, "sopp", 0xBF80, 0xFF80, 16, 22}
	d.formatTable[vop1] = &Format{vop1, "vop1", 0x7E00, 0xFE00, 9, 16}
	d.formatTable[vopc] = &Format{vopc, "vopc", 0x7C00, 0xFE00, 17, 24}
	d.formatTable[smem] = &Format{smem, "smem", 0xC000, 0xFC00, 18, 25}
	d.formatTable[vop3] = &Format{vop3, "vop3", 0xD000, 0xFC00, 16, 25}
	d.formatTable[vintrp] = &Format{vintrp, "vintrp", 0xC800, 0xFC00, 16, 17}
	d.formatTable[ds] = &Format{ds, "ds", 0xD800, 0xFC00, 17, 24}
	d.formatTable[mubuf] = &Format{mubuf, "mubuf", 0xE000, 0xFC00, 18, 24}
	d.formatTable[mtbuf] = &Format{mtbuf, "mtbuf", 0xE800, 0xFC00, 15, 18}
	d.formatTable[mimg] = &Format{mimg, "mimg", 0xF000, 0xFC00, 18, 24}
	d.formatTable[exp] = &Format{exp, "exp", 0xC400, 0xFC00, 0, 0}
	d.formatTable[flat] = &Format{flat, "flat", 0xDC00, 0xFC00, 18, 24}
	d.formatTable[sopk] = &Format{sopk, "sopk", 0xB000, 0xF000, 23, 27}
	d.formatTable[sop2] = &Format{sop2, "sop2", 0x8000, 0xA000, 23, 29}
	d.formatTable[vop2] = &Format{vop2, "vop2", 0x0000, 0x8000, 25, 30}
}

func (d *Disassembler) initializeDecodeTable() {
	d.decodeTables = make(map[formatType]*decodeTable)

	d.addInstType(&InstType{"s_add_u32", 0, d.formatTable[sop2]})
}

// NewDisassembler creates a new disassembler
func NewDisassembler() *Disassembler {
	d := new(Disassembler)

	d.initializeFormatTable()
	d.initializeDecodeTable()

	return d
}

func (d *Disassembler) matchFormat(firstTwoBytes uint16) (*Format, error) {
	for _, f := range d.formatTable {
		if (firstTwoBytes^f.Encoding)&f.Mask == 0 {
			fmt.Printf("%016b matches %s\n", firstTwoBytes, f.Name)
			return f, nil
		}
	}
	bytesString := fmt.Sprintf("%04x", firstTwoBytes)
	return nil, errors.New("cannot find the instruction format, first two " +
		"bytes are " + bytesString)
}

func (d *Disassembler) loopUp(format *Format, opcode Opcode) (*InstType, error) {
	if d.decodeTables[format.formatType] != nil &&
		d.decodeTables[format.formatType].insts[opcode] != nil {
		return d.decodeTables[format.formatType].insts[opcode], nil
	}

	errString := fmt.Sprintf("Instruction format %s, opcode %d not found",
		format.Name, opcode)
	return nil, errors.New(errString)
}

// Decode parses the head of the buffer and returns the next instruction
func (d *Disassembler) Decode(buf []byte) (*Instruction, error) {
	format, err := d.matchFormat(binary.LittleEndian.Uint16(buf[2:]))
	if err != nil {
		_ = fmt.Errorf("%s", err.Error())
		return nil, err
	}

	opcode := format.retrieveOpcode(binary.LittleEndian.Uint32(buf))
	instType, err := d.loopUp(format, opcode)
	if err != nil {
		_ = fmt.Errorf("%s", err.Error())
		return nil, err
	}

	fmt.Printf("Format %s matched %08b, opcode %d, inst %s.\n", format.Name, buf[0:4], opcode, instType.Name)

	return nil, nil
}

// Disassemble take a binary file as an input and put the assembly code in a
// write
func (d *Disassembler) Disassemble(file *elf.File, w io.Writer) {
	sections := file.Sections

	for _, sec := range sections {
		if sec.Name == ".text" {
			data, _ := sec.Data()
			co := NewHsaCo(data)
			fmt.Printf("%+v\n", co.HsaCoHeader)

			instructionData := co.InstructionData()
			for len(instructionData) > 0 {
				_, _ = d.Decode(instructionData)
				instructionData = instructionData[4:]
			}
		}
	}
}
