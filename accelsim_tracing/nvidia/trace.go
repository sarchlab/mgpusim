package nvidia

type KernelList struct {
	ListDirPath string
	TraceExecs  []*TraceExec
}

type TraceExec struct {
	Type     string
	FilePath string
}

type TraceGroup struct {
	Header       *TraceGroupHeader
	ThreadBlocks []*ThreadBlock
}

type TraceGroupHeader struct {
	KernelName            string
	KernelID              int32
	GridDim               Dim3
	BlockDim              Dim3
	Shmem                 int32
	Nregs                 int32
	BinaryVersion         int32
	CudaStreamID          int32
	ShmemBaseAddr         int64
	LocalMemBaseAddr      int64
	NvbitVersion          string
	AccelsimTracerVersion string
}

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
