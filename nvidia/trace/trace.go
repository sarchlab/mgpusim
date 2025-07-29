package trace

import (
	"fmt"

	log "github.com/sirupsen/logrus"
)

// Opcode type was previously from trace package
// type Opcode struct {
// 	Name string
// }

// func NewOpcode(name string) *Opcode {
// 	return &Opcode{Name: name}
// }

type KernelTrace struct {
	ID           string
	FileHeader   KernelFileHeader
	tbIDToIndex  map[Dim3]int32
	Threadblocks []*ThreadblockTrace
}

type KernelFileHeader struct {
	KernelName            string `title:"kernel name"`
	KernelID              int32  `title:"kernel id"`
	GridDim               Dim3   `title:"grid dim"`
	BlockDim              Dim3   `title:"block dim"`
	Shmem                 int32  `title:"shmem"`
	Nregs                 int32  `title:"nregs"`
	BinaryVersion         int32  `title:"binary version"`
	CudaStreamID          int32  `title:"cuda stream id"`
	ShmemBaseAddr         uint64 `title:"shmem base_addr"`
	LocalMemBaseAddr      uint64 `title:"local mem base_addr"`
	NvbitVersion          string `title:"nvbit version"`
	AccelsimTracerVersion string `title:"accelsim tracer version"`
	EnableLineinfo        bool   `title:"enable lineinfo"`
}

type ThreadblockTrace struct {
	ID    Dim3
	Warps []*WarpTrace
}

type WarpTrace struct {
	ID           int
	instsCount   uint64
	Instructions []*InstructionTrace
}

type InstructionTrace struct {
	threadblockID     Dim3
	warpID            int
	PC                uint64
	Mask              uint64
	DestNum           int
	DestRegs          []Register
	OpCode            *Opcode
	SrcNum            int
	SrcRegs           []Register
	MemWidth          int
	AddressCompress   int
	MemAddress        uint64
	MemAddressSuffix1 int
	MemAddressSuffix2 []int32
	Immediate         uint64
}

// Shaoyu: Maybe we can parse the attrs in order and avoid using swicth-case here
// ChenGong: I thought it would be better to display the working pattern during the parse
// [TODO]
//
//nolint:funlen,gocyclo
func (th *KernelFileHeader) updateTraceHeaderParam(key string, value string) {
	err := error(nil)

	switch key {
	case "kernel name":
		th.KernelName = value
	case "kernel id":
		_, err = fmt.Sscanf(value, "%d", &th.KernelID)
	case "grid dim":
		_, err = fmt.Sscanf(value, "(%d,%d,%d)", &th.GridDim[0], &th.GridDim[1], &th.GridDim[2])
	case "block dim":
		_, err = fmt.Sscanf(value, "(%d,%d,%d)", &th.BlockDim[0], &th.BlockDim[1], &th.BlockDim[2])
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
		log.WithField("key", key).Panic("Unknown key")
	}

	if err != nil {
		log.WithError(err).Panic("Failed to parse value")
	}
}

func (t *KernelTrace) ThreadblocksCount() uint64 {
	return uint64(len(t.Threadblocks))
}

func (t *KernelTrace) Threadblock(index uint64) *ThreadblockTrace {
	return t.Threadblocks[index]
}

func (tb *ThreadblockTrace) WarpsCount() uint64 {
	return uint64(len(tb.Warps))
}

func (tb *ThreadblockTrace) Warp(index uint64) *WarpTrace {
	return tb.Warps[index]
}

func (w *WarpTrace) InstructionsCount() uint64 {
	return uint64(len(w.Instructions))
}

func (i *InstructionTrace) InstructionsParentID() string {
	return fmt.Sprintf("threadblock[%d,%d,%d]@warp[%d]", i.threadblockID[0], i.threadblockID[1], i.threadblockID[2], i.warpID)
}
