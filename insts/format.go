package insts

// FormatType is a enumeration of all the instruction formats defined by GCN3
type FormatType int

// All the GCN3 instruction formats
const (
	Sop2 FormatType = iota
	Sopk
	Sop1
	Sopc
	Sopp
	Smem
	Vop2
	Vop1
	Vop3
	Vopc
	Vintrp
	Ds
	Mubuf
	Mtbuf
	Mimg
	Exp
	Flat
	formatTypeCount
)

// Format defines the possible microcode format of instructions
type Format struct {
	FormatType        FormatType
	FormatName        string
	Encoding          uint16
	Mask              uint16
	ByteSizeExLiteral int
	OpcodeLow         uint8
	OpcodeHigh        uint8
}

// FormatTable is a table that uses FormatType to find detailed format
// information about the format.
var FormatTable map[FormatType]*Format

func init() {
	initFormatTable()
}

func initFormatTable() {
	FormatTable = make(map[FormatType]*Format)
	FormatTable[Sop1] = &Format{Sop1, "sop1", 0xBE80, 0xFF80, 4, 8, 15}
	FormatTable[Sopc] = &Format{Sopc, "sopc", 0xBF00, 0xFF80, 4, 16, 22}
	FormatTable[Sopp] = &Format{Sopp, "sopp", 0xBF80, 0xFF80, 4, 16, 22}
	FormatTable[Vop1] = &Format{Vop1, "vop1", 0x7E00, 0xFE00, 4, 9, 16}
	FormatTable[Vopc] = &Format{Vopc, "vopc", 0x7C00, 0xFE00, 4, 17, 24}
	FormatTable[Smem] = &Format{Smem, "smem", 0xC000, 0xFC00, 8, 18, 25}
	FormatTable[Vop3] = &Format{Vop3, "vop3", 0xD000, 0xFC00, 8, 16, 25}
	FormatTable[Vintrp] = &Format{Vintrp, "vintrp", 0xC800, 0xFC00, 4, 16, 17}
	FormatTable[Ds] = &Format{Ds, "ds", 0xD800, 0xFC00, 8, 17, 24}
	FormatTable[Mubuf] = &Format{Mubuf, "mubuf", 0xE000, 0xFC00, 8, 18, 24}
	FormatTable[Mtbuf] = &Format{Mtbuf, "mtbuf", 0xE800, 0xFC00, 8, 15, 18}
	FormatTable[Mimg] = &Format{Mimg, "mimg", 0xF000, 0xFC00, 8, 18, 24}
	FormatTable[Exp] = &Format{Exp, "exp", 0xC400, 0xFC00, 8, 0, 0}
	FormatTable[Flat] = &Format{Flat, "flat", 0xDC00, 0xFC00, 8, 18, 24}
	FormatTable[Sopk] = &Format{Sopk, "sopk", 0xB000, 0xF000, 4, 23, 27}
	FormatTable[Sop2] = &Format{Sop2, "sop2", 0x8000, 0xA000, 4, 23, 29}
	FormatTable[Vop2] = &Format{Vop2, "vop2", 0x0000, 0x8000, 4, 25, 30}
}
