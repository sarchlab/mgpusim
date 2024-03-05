package tracereader

import (
	"fmt"

	"github.com/sarchlab/accelsimtracing/nvidia"
)

type KernelTrace struct {
	fileHeader   KernelFileHeader
	threadblocks []ThreadblockTrace
}

type KernelFileHeader struct {
	kernelName            string
	kernelID              int32
	gridDim               nvidia.Dim3
	blockDim              nvidia.Dim3
	shmem                 int32
	nregs                 int32
	binaryVersion         int32
	cudaStreamID          int32
	shmemBaseAddr         int64
	localMemBaseAddr      int64
	nvbitVersion          string
	accelsimTracerVersion string
}

type ThreadblockTrace struct {
	ThreadblockDim nvidia.Dim3
	Warps          []WarpTrace
}

type WarpTrace struct {
	WarpID       int32
	InstsCount   int32
	Instructions []Instruction
}

type Instruction struct {
	rawtext           string
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
		th.kernelName = value
	case "kernel id":
		_, err = fmt.Sscanf(value, "%d", &th.kernelID)
	case "grid dim":
		_, err = fmt.Sscanf(value, "(%d,%d,%d)", &th.gridDim[0], &th.gridDim[1], &th.gridDim[2])
	case "block dim":
		_, err = fmt.Sscanf(value, "(%d,%d,%d)", &th.blockDim[0], &th.blockDim[1], &th.blockDim[2])
	case "shmem":
		_, err = fmt.Sscanf(value, "%d", &th.shmem)
	case "nregs":
		_, err = fmt.Sscanf(value, "%d", &th.nregs)
	case "binary version":
		_, err = fmt.Sscanf(value, "%d", &th.binaryVersion)
	case "cuda stream id":
		_, err = fmt.Sscanf(value, "%d", &th.cudaStreamID)
	case "shmem base_addr":
		_, err = fmt.Sscanf(value, "%v", &th.shmemBaseAddr)
	case "local mem base_addr":
		_, err = fmt.Sscanf(value, "%v", &th.localMemBaseAddr)
	case "nvbit version":
		th.nvbitVersion = value
	case "accelsim tracer version":
		th.accelsimTracerVersion = value
	default:
		panic("never")
	}

	if err != nil {
		panic(err)
	}
}

func (k *KernelTrace) ThreadblocksCount() int64 {
	return int64(len(k.threadblocks))
}

func (k *KernelTrace) Threadblock(id int64) *ThreadblockTrace {
	return &k.threadblocks[id]
}

func (tb *ThreadblockTrace) WarpsCount() int64 {
	return int64(len(tb.Warps))
}

func (tb *ThreadblockTrace) Warp(id int64) *WarpTrace {
	return &tb.Warps[id]
}

func (w *WarpTrace) InstructionsCount() int64 {
	return int64(len(w.Instructions))
}
