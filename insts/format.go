package insts

// All the GCN3 instruction formats
const (
	Sop2 = iota
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

type FormatType int

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
