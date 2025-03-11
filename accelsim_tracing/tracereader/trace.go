package tracereader

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/nvidia"
)

type KernelTrace struct {
	FileHeader  KernelFileHeader
	tbIDToIndex map[nvidia.Dim3]int32

	threadblocks []*ThreadblockTrace
}

type KernelFileHeader struct {
	KernelName            string      `title:"kernel name"`
	KernelID              int32       `title:"kernel id"`
	GridDim               nvidia.Dim3 `title:"grid dim"`
	BlockDim              nvidia.Dim3 `title:"block dim"`
	Shmem                 int32       `title:"shmem"`
	Nregs                 int32       `title:"nregs"`
	BinaryVersion         int32       `title:"binary version"`
	CudaStreamID          int32       `title:"cuda stream id"`
	ShmemBaseAddr         int64       `title:"shmem base_addr"`
	LocalMemBaseAddr      int64       `title:"local mem base_addr"`
	NvbitVersion          string      `title:"nvbit version"`
	AccelsimTracerVersion string      `title:"accelsim tracer version"`
	EnableLineinfo        bool        `title:"enable lineinfo"`
}

type ThreadblockTrace struct {
	id nvidia.Dim3

	Warps []*WarpTrace
}

type WarpTrace struct {
	id int32

	InstsCount   int32
	Instructions []*Instruction
}

type Instruction struct {
	threadblockID nvidia.Dim3
	warpID        int32

	PC                int32
	Mask              int64
	DestNum           int32
	DestRegs          []*nvidia.Register
	OpCode            *nvidia.Opcode
	SrcNum            int32
	SrcRegs           []*nvidia.Register
	MemWidth          int32
	AddressCompress   int32
	MemAddress        int64
	MemAddressSuffix1 int32
	MemAddressSuffix2 []int32
	Immediate         int64
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

func (k *KernelTrace) ThreadblocksCount() int64 {
	return int64(len(k.threadblocks))
}

func (k *KernelTrace) Threadblock(index int64) *ThreadblockTrace {
	return k.threadblocks[index]
}

func (tb *ThreadblockTrace) WarpsCount() int64 {
	return int64(len(tb.Warps))
}

func (tb *ThreadblockTrace) Warp(index int64) *WarpTrace {
	return tb.Warps[index]
}

func (w *WarpTrace) InstructionsCount() int64 {
	return int64(len(w.Instructions))
}
