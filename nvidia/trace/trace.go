package trace

import (
	"fmt"
)

// ExecType identifies the kind of an entry in kernelslist.g.
type ExecType int

// The kinds of entries that can appear in kernelslist.g.
const (
	ExecUndefined ExecType = iota
	ExecKernel
	ExecMemcpy
)

// ExecMemcpyDirection is the direction of a memory copy.
type ExecMemcpyDirection string

// Memory copy directions as spelled in kernelslist.g.
const (
	ExecMemcpyDirectionUndefined ExecMemcpyDirection = ""
	H2D                          ExecMemcpyDirection = "MemcpyHtoD"
	D2H                          ExecMemcpyDirection = "MemcpyDtoH"
)

// Dim3 is a 3-dimensional index, e.g., a thread block ID.
type Dim3 [3]int32

// Register is a register operand of an instruction.
type Register struct {
	Name string
}

// Opcode is the SASS opcode of an instruction, e.g., "LDG.E".
type Opcode string

// String returns the opcode text.
func (op Opcode) String() string {
	return string(op)
}

// KernelTrace is the trace of one kernel launch.
type KernelTrace struct {
	ID           string
	FileHeader   KernelFileHeader
	Threadblocks []*ThreadblockTrace
}

// KernelFileHeader holds the "-key = value" lines at the top of a .traceg
// file.
type KernelFileHeader struct {
	KernelName            string
	KernelID              int32
	GridDim               Dim3
	BlockDim              Dim3
	Shmem                 int32
	Nregs                 int32
	BinaryVersion         int32
	CudaStreamID          int32
	ShmemBaseAddr         uint64
	LocalMemBaseAddr      uint64
	NvbitVersion          string
	AccelsimTracerVersion string
	EnableLineinfo        bool
}

// ThreadblockTrace is the trace of one thread block (CTA).
type ThreadblockTrace struct {
	ID             Dim3
	FatherKernelID string
	Warps          []*WarpTrace
}

// WarpTrace is the dynamic instruction stream of one warp.
type WarpTrace struct {
	ID                  int
	FatherThreadblockID Dim3
	Instructions        []*InstructionTrace
}

// InstructionTrace is one dynamic instruction of a warp.
type InstructionTrace struct {
	threadblockID     Dim3
	warpID            int
	instIndexInWarp   uint64
	PC                uint64
	Mask              uint64
	DestNum           int
	DestRegs          []Register
	OpCode            Opcode
	SrcNum            int
	SrcRegs           []Register
	MemWidth          int
	AddressCompress   int
	MemAddress        uint64
	MemAddressSuffix1 int
	MemAddressSuffix2 []int32
	Immediate         uint64
}

func (th *KernelFileHeader) updateTraceHeaderParam(key string, value string) {
	var err error

	switch key {
	case "kernel name":
		th.KernelName = value
	case "kernel id":
		_, err = fmt.Sscanf(value, "%d", &th.KernelID)
	case "grid dim":
		_, err = fmt.Sscanf(value, "(%d,%d,%d)",
			&th.GridDim[0], &th.GridDim[1], &th.GridDim[2])
	case "block dim":
		_, err = fmt.Sscanf(value, "(%d,%d,%d)",
			&th.BlockDim[0], &th.BlockDim[1], &th.BlockDim[2])
	case "shmem":
		_, err = fmt.Sscanf(value, "%d", &th.Shmem)
	case "nregs":
		_, err = fmt.Sscanf(value, "%d", &th.Nregs)
	case "binary version":
		_, err = fmt.Sscanf(value, "%d", &th.BinaryVersion)
	case "cuda stream id":
		_, err = fmt.Sscanf(value, "%d", &th.CudaStreamID)
	case "shmem base_addr":
		_, err = fmt.Sscanf(value, "%v", &th.ShmemBaseAddr)
	case "local mem base_addr":
		_, err = fmt.Sscanf(value, "%v", &th.LocalMemBaseAddr)
	case "nvbit version":
		th.NvbitVersion = value
	case "accelsim tracer version":
		th.AccelsimTracerVersion = value
	case "enable lineinfo":
		th.EnableLineinfo = value == "1"
	default:
		// Newer tracer versions may add header fields that the simulator
		// does not use.
	}

	if err != nil {
		panic(fmt.Sprintf("failed to parse trace header %q = %q: %v",
			key, value, err))
	}
}

// ThreadblocksCount returns the number of thread blocks in the kernel.
func (t *KernelTrace) ThreadblocksCount() uint64 {
	return uint64(len(t.Threadblocks))
}

// Threadblock returns the thread block at the given index.
func (t *KernelTrace) Threadblock(index uint64) *ThreadblockTrace {
	return t.Threadblocks[index]
}

// WarpsCount returns the number of warps in the thread block.
func (tb *ThreadblockTrace) WarpsCount() uint64 {
	return uint64(len(tb.Warps))
}

// Warp returns the warp at the given index.
func (tb *ThreadblockTrace) Warp(index uint64) *WarpTrace {
	return tb.Warps[index]
}

// ThreadblockFullID returns a human-readable ID of the thread block.
func (tb *ThreadblockTrace) ThreadblockFullID() string {
	return fmt.Sprintf("threadblock[%d,%d,%d]", tb.ID[0], tb.ID[1], tb.ID[2])
}

// InstructionsCount returns the number of instructions of the warp.
func (w *WarpTrace) InstructionsCount() uint64 {
	return uint64(len(w.Instructions))
}

// InstructionsFullID returns a human-readable ID of the instruction.
func (i *InstructionTrace) InstructionsFullID() string {
	return fmt.Sprintf("threadblock[%d,%d,%d]@warp[%d]@inst[%d]",
		i.threadblockID[0], i.threadblockID[1], i.threadblockID[2],
		i.warpID, i.instIndexInWarp)
}
