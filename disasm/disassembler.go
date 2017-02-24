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

// Format defines the possible microcode format of instructions
type Format struct {
	formatType int
	Name       string
	Encoding   uint16
	Mask       uint16
}

// Disassembler is the unit that can decode .hsaco file
type Disassembler struct {
	formatTable []Format
}

func (d *Disassembler) initializeFormatTable() {
	d.formatTable = make([]Format, 0)
	d.formatTable = append(d.formatTable, Format{sop1, "sop1", 0xBE80, 0xFF80})
	d.formatTable = append(d.formatTable, Format{sopc, "sopc", 0xBF00, 0xFF80})
	d.formatTable = append(d.formatTable, Format{sopp, "sopp", 0xBF80, 0xFF80})
	d.formatTable = append(d.formatTable, Format{vop1, "vop1", 0x7E00, 0xFE00})
	d.formatTable = append(d.formatTable, Format{vopc, "vopc", 0x7C00, 0xFE00})
	d.formatTable = append(d.formatTable, Format{smem, "smem", 0xC000, 0xFC00})
	d.formatTable = append(d.formatTable, Format{vop3, "vop3", 0xD000, 0xFC00})
	d.formatTable = append(d.formatTable, Format{vintrp, "vintrp", 0xC800, 0xFC00})
	d.formatTable = append(d.formatTable, Format{ds, "ds", 0xD800, 0xFC00})
	d.formatTable = append(d.formatTable, Format{mubuf, "mubuf", 0xE000, 0xFC00})
	d.formatTable = append(d.formatTable, Format{mtbuf, "mtbuf", 0xE800, 0xFC00})
	d.formatTable = append(d.formatTable, Format{mimg, "mimg", 0xF000, 0xFC00})
	d.formatTable = append(d.formatTable, Format{exp, "exp", 0xC400, 0xFC00})
	d.formatTable = append(d.formatTable, Format{flat, "flat", 0xDC00, 0xFC00})
	d.formatTable = append(d.formatTable, Format{sopk, "sopk", 0xB000, 0xF000})
	d.formatTable = append(d.formatTable, Format{sop2, "sop2", 0x8000, 0xA000})
	d.formatTable = append(d.formatTable, Format{vop2, "vop2", 0x0000, 0x8000})
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
	fmt.Printf("Format %s matched %08b.\n", format.Name, buf[0:4])

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
