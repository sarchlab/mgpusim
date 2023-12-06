package nvidia

type ThreadBlock struct {
	WarpNum int
	Warps   []*Warp
}

type Warp struct {
	InstNum int
	Insts   []*Instruction
}

type Instruction struct {
	PC                int32
	Mask              int64
	DestNum           int32
	DestRegs          []*Register
	OpCode            *Opcode
	SrcNum            int32
	SrcRegs           []*Register
	MemWidth          int32
	AddressCompress   int32
	MemAddress        int64
	MemAddressSuffix1 int32
	MemAddressSuffix2 []int32
}
