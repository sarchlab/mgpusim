package trace

import (
	"fmt"
	"log"
	"strings"

	"github.com/sarchlab/mgpusim/accelsim_tracing/nvidia"
)

type traceHeader struct {
	parent     *traceGroup
	rawContext struct {
		kernelName            string
		kernelID              string
		gridDim               string
		blockDim              string
		shmem                 string
		nregs                 string
		binaryVersion         string
		cudaStreamID          string
		shmemBaseAddr         string
		localMemBaseAddr      string
		nvbitVersion          string
		accelsimTracerVersion string
	}

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

func parseHeaderParam(lines []string) *traceHeader {
	th := &traceHeader{}

	for _, line := range lines {
		elems := strings.Split(line, "=")
		if len(elems) != 2 {
			log.Panicf("Invalid trace header line: %s", line)
		}
		key := strings.TrimSpace(elems[0])
		value := strings.TrimSpace(elems[1])

		th.updateParam(key[1:], value, line)
	}
	return th
}

// Shaoyu: Maybe we can parse the attrs in order and avoid using swicth-case here
func (th *traceHeader) updateParam(key string, value string, rawText string) {
	err := error(nil)
	switch key {
	case "kernel name":
		th.rawContext.kernelName = rawText
		th.kernelName = value
	case "kernel id":
		th.rawContext.kernelID = rawText
		_, err = fmt.Sscanf(value, "%d", &th.kernelID)
	case "grid dim":
		th.rawContext.gridDim = rawText
		_, err = fmt.Sscanf(value, "(%d,%d,%d)", &th.gridDim[0], &th.gridDim[1], &th.gridDim[2])
	case "block dim":
		th.rawContext.blockDim = rawText
		_, err = fmt.Sscanf(value, "(%d,%d,%d)", &th.blockDim[0], &th.blockDim[1], &th.blockDim[2])
	case "shmem":
		th.rawContext.shmem = rawText
		_, err = fmt.Sscanf(value, "%d", &th.shmem)
	case "nregs":
		th.rawContext.nregs = rawText
		_, err = fmt.Sscanf(value, "%d", &th.nregs)
	case "binary version":
		th.rawContext.binaryVersion = rawText
		_, err = fmt.Sscanf(value, "%d", &th.binaryVersion)
	case "cuda stream id":
		th.rawContext.cudaStreamID = rawText
		_, err = fmt.Sscanf(value, "%d", &th.cudaStreamID)
	case "shmem base_addr":
		th.rawContext.shmemBaseAddr = rawText
		_, err = fmt.Sscanf(value, "%v", &th.shmemBaseAddr)
	case "local mem base_addr":
		th.rawContext.localMemBaseAddr = rawText
		_, err = fmt.Sscanf(value, "%v", &th.localMemBaseAddr)
	case "nvbit version":
		th.rawContext.nvbitVersion = rawText
		th.nvbitVersion = value
	case "accelsim tracer version":
		th.rawContext.accelsimTracerVersion = rawText
		th.accelsimTracerVersion = value
	default:
		log.Printf("Unknown trace header key: %s", key)
	}
	if err != nil {
		log.Panicf("Invalid trace header value for [%s]: %s", key, value)
	}
}
