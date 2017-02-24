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
	formatTable []Format

	// Maps from the format to table
	decodeTables map[formatType]decodeTable
}

func (d *Disassembler) addInstType(info *InstType) {
	d.decodeTables[info.Format.formatType].insts[info.Opcode] = info
}

func (d *Disassembler) initializeFormatTable() {
	d.formatTable = make([]Format, 0)
	d.formatTable = append(d.formatTable, Format{sop1, "sop1", 0xBE80, 0xFF80, 8, 15})
	d.formatTable = append(d.formatTable, Format{sopc, "sopc", 0xBF00, 0xFF80, 16, 22})
	d.formatTable = append(d.formatTable, Format{sopp, "sopp", 0xBF80, 0xFF80, 16, 22})
	d.formatTable = append(d.formatTable, Format{vop1, "vop1", 0x7E00, 0xFE00, 9, 16})
	d.formatTable = append(d.formatTable, Format{vopc, "vopc", 0x7C00, 0xFE00, 17, 24})
	d.formatTable = append(d.formatTable, Format{smem, "smem", 0xC000, 0xFC00, 18, 25})
	d.formatTable = append(d.formatTable, Format{vop3, "vop3", 0xD000, 0xFC00, 16, 25})
	d.formatTable = append(d.formatTable, Format{vintrp, "vintrp", 0xC800, 0xFC00, 16, 17})
	d.formatTable = append(d.formatTable, Format{ds, "ds", 0xD800, 0xFC00, 17, 24})
	d.formatTable = append(d.formatTable, Format{mubuf, "mubuf", 0xE000, 0xFC00, 18, 24})
	d.formatTable = append(d.formatTable, Format{mtbuf, "mtbuf", 0xE800, 0xFC00, 15, 18})
	d.formatTable = append(d.formatTable, Format{mimg, "mimg", 0xF000, 0xFC00, 18, 24})
	d.formatTable = append(d.formatTable, Format{exp, "exp", 0xC400, 0xFC00, 0, 0})
	d.formatTable = append(d.formatTable, Format{flat, "flat", 0xDC00, 0xFC00, 18, 24})
	d.formatTable = append(d.formatTable, Format{sopk, "sopk", 0xB000, 0xF000, 23, 27})
	d.formatTable = append(d.formatTable, Format{sop2, "sop2", 0x8000, 0xA000, 23, 29})
	d.formatTable = append(d.formatTable, Format{vop2, "vop2", 0x0000, 0x8000, 25, 30})
}

func (d *Disassembler) initializeDecodeTable() {
	d.decodeTables = make(map[formatType]decodeTable)
}

// NewDisassembler creates a new disassembler
func NewDisassembler() *Disassembler {
	d := new(Disassembler)

	d.initializeFormatTable()

	return d
}

func (d *Disassembler) matchFormat(firstTwoBytes uint16) (*Format, error) {
	for _, f := range d.formatTable {
		if (firstTwoBytes^f.Encoding)&f.Mask == 0 {
			fmt.Printf("%016b matches %s\n", firstTwoBytes, f.Name)
			return &f, nil
		}
	}
	bytesString := fmt.Sprintf("%04x", firstTwoBytes)
	return nil, errors.New("cannot find the instruction format, first two " +
		"bytes are " + bytesString)
}

// Decode parses the head of the buffer and returns the next instruction
func (d *Disassembler) Decode(buf []byte) (*Instruction, error) {
	format, err := d.matchFormat(binary.LittleEndian.Uint16(buf[2:]))
	if err != nil {
		_ = fmt.Errorf("%s", err.Error())
		return nil, err
	}

	opcode := format.retrieveOpcode(binary.LittleEndian.Uint32(buf))
	fmt.Printf("Format %s matched %08b, opcode %d.\n", format.Name, buf[0:4], opcode)

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
