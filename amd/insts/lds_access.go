package insts

import "fmt"

// CDNA3LDSAccess describes the LDS bytes transferred by one active lane.
// A paired DS instruction transfers each element at its own byte offset.
type CDNA3LDSAccess struct {
	Bytes   int
	Count   int
	Offsets [2]uint32
	Write   bool
}

// LDSAccess returns the access shape of an implemented gfx942 DS or shared
// FLAT instruction. All offsets are relative to the vector address register.
func (i *Inst) LDSAccess() CDNA3LDSAccess {
	a := CDNA3LDSAccess{Count: 1, Offsets: [2]uint32{i.Offset0}}
	if i.FormatType == FLAT {
		switch i.Opcode {
		case 16, 17:
			a.Bytes = 1
		case 18:
			a.Bytes = 2
		case 20, 21, 22, 23:
			a.Bytes = int(i.Opcode-19) * 4
		case 28, 29, 30, 31:
			a.Bytes, a.Write = int(i.Opcode-27)*4, true
		default:
			panic(fmt.Sprintf("unsupported shared FLAT opcode %d", i.Opcode))
		}
		return a
	}
	if i.FormatType != DS {
		panic("LDS access requires DS or FLAT")
	}
	return i.dsAccess(a)
}

func (i *Inst) dsAccess(a CDNA3LDSAccess) CDNA3LDSAccess {
	var stride uint32
	switch i.Opcode {
	case 13, 54:
		a.Bytes = 4
	case 14, 55:
		a.Bytes, stride = 4, 4
	case 15, 56:
		a.Bytes, stride = 4, 256
	case 30:
		a.Bytes = 1
	case 77, 118:
		a.Bytes = 8
	case 78, 119:
		a.Bytes, stride = 8, 8
	case 222, 254:
		a.Bytes = 12
	case 223, 255:
		a.Bytes = 16
	default:
		panic(fmt.Sprintf("unsupported DS opcode %d", i.Opcode))
	}
	a.Write = i.SRC0Width != 0
	if stride != 0 {
		a.Count, a.Offsets = 2, [2]uint32{i.Offset0 * stride, i.Offset1 * stride}
	}
	return a
}
