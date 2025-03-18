package insts

// FormatType is a enumeration of all the instruction formats defined by GCN3
type FormatType int

// All the GCN3 instruction formats
const (
	SOP2 FormatType = iota
	SOPK
	SOP1
	SOPC
	SOPP
	SMEM
	VOP2
	VOP1
	VOP3a
	VOP3b
	VOPC
	VINTRP
	DS
	MUBUF
	MTBUF
	MIMG
	EXP
	FLAT
	formatTypeCount
)

// Format defines the possible microcode format of instructions
type Format struct {
	FormatType        FormatType
	FormatName        string
	Encoding          uint32
	Mask              uint32
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
	FormatTable[SOP1] = &Format{SOP1, "sop1", 0xBE800000, 0xFF800000, 4, 8, 15}
	FormatTable[SOPC] = &Format{SOPC, "sopc", 0xBF000000, 0xFF800000, 4, 16, 22}
	FormatTable[SOPP] = &Format{SOPP, "sopp", 0xBF800000, 0xFF800000, 4, 16, 22}
	FormatTable[VOP1] = &Format{VOP1, "vop1", 0x7E000000, 0xFE000000, 4, 9, 16}
	FormatTable[VOPC] = &Format{VOPC, "vopc", 0x7C000000, 0xFE000000, 4, 17, 24}
	FormatTable[SMEM] = &Format{SMEM, "smem", 0xC0000000, 0xFC000000, 8, 18, 25}
	FormatTable[VOP3a] = &Format{VOP3a, "vop3a", 0xD0000000, 0xFC000000, 8, 16, 25}
	FormatTable[VOP3b] = &Format{VOP3b, "vop3b", 0xD0000000, 0xFC000000, 8, 16, 25}
	FormatTable[VINTRP] = &Format{VINTRP, "vintrp", 0xC8000000, 0xFC000000, 4, 16, 17}
	FormatTable[DS] = &Format{DS, "ds", 0xD8000000, 0xFC000000, 8, 17, 24}
	FormatTable[MUBUF] = &Format{MUBUF, "mubuf", 0xE0000000, 0xFC000000, 8, 18, 24}
	FormatTable[MTBUF] = &Format{MTBUF, "mtbuf", 0xE8000000, 0xFC000000, 8, 15, 18}
	FormatTable[MIMG] = &Format{MIMG, "mimg", 0xF0000000, 0xFC000000, 8, 18, 24}
	FormatTable[EXP] = &Format{EXP, "exp", 0xC4000000, 0xFC000000, 8, 0, 0}
	FormatTable[FLAT] = &Format{FLAT, "flat", 0xDC000000, 0xFC000000, 8, 18, 24}
	FormatTable[SOPK] = &Format{SOPK, "sopk", 0xB0000000, 0xF0000000, 4, 23, 27}
	FormatTable[SOP2] = &Format{SOP2, "sop2", 0x80000000, 0xC0000000, 4, 23, 29}
	FormatTable[VOP2] = &Format{VOP2, "vop2", 0x00000000, 0x80000000, 4, 25, 30}
}
