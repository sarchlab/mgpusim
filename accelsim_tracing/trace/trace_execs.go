package trace

import (
	"fmt"
	"log"
	"strings"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpu"
)

type traceExecs interface {
	Type() string
	Execute(gpu *gpu.GPU) error
}

func parseTraceExecs(rawText string, trace *Trace) traceExecs {
	if strings.HasPrefix(rawText, "Memcpy") {
		/*
			format  : H2D or D2H, start, length
			example : HtoD,0x7f0,0x1000
		*/
		res := strings.Split(rawText, ",")
		m := &memCopy{
			parent:  trace,
			rawText: rawText,
			h2d:     strings.Contains(res[0], "HtoD"),
		}
		fmt.Sscanf(res[1], "%v", &m.startAddr)
		fmt.Sscanf(res[2], "%v", &m.length)
		return m
	} else if strings.HasPrefix(rawText, "kernel") {
		/*
			format  : kernel name
			example : kernel_0
		*/
		k := &kernel{
			parent:   trace,
			rawText:  rawText,
			filePath: rawText,
		}
		return k
	}
	log.Panicf("Unknown trace group rawText: %s", rawText)
	return nil
}
