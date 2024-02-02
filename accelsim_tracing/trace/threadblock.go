package trace

import (
	"fmt"
	"log"
	"strings"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
)

type threadBlock struct {
	parent     *traceGroupReader
	rawContext struct {
		blockDim string
	}

	threadBlockDim nvidia.Dim3
	warps          []*warp
}

func parseThreadBlocks(lines []string) *threadBlock {
	tb := &threadBlock{}
	dim := parseThreadBlockDim(lines)
	tb.threadBlockDim = *dim
	for i, line := range lines {
		if strings.HasPrefix(line, "warp") {
			wp := parseWarp(lines[i:]) // [todo] too many copies
			wp.parent = tb
			tb.warps = append(tb.warps, wp)
		}
	}

	return tb
}

func parseThreadBlockDim(lines []string) *nvidia.Dim3 {
	for _, line := range lines {
		if strings.HasPrefix(line, "thread block") {
			d := &nvidia.Dim3{}
			elems := strings.Split(line, "=")
			if len(elems) != 2 {
				log.Panicf("Invalid thread block dim line: %s", line)
			}

			value := strings.TrimSpace(elems[1])
			_, err := fmt.Sscanf(value, "%d,%d,%d", &d[0], &d[1], &d[2])
			if err != nil {
				log.Panicf("Invalid thread block dim value: %s", value)
			}

			return d
		}
	}

	log.Panic("Cannot find thread block dim")
	return nil
}
