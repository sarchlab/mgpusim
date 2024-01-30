package trace

import (
	"bufio"
	"container/list"
	"os"
	"strings"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpu"
)

type traceGroupReader struct {
	File             *os.File
	scanner          *bufio.Scanner
	traceHeader      *traceHeader
	ThreadBlockQueue *list.List
}

func (tg *traceGroupReader) Read(gpu *gpu.GPU) error {
	// [todo] threadblocks can be parallelized to save memory
	return nil
}

func (tg *traceGroupReader) ParseThreadBlocks() {
	for tg.scanner.Scan() {
		if strings.TrimSpace(tg.scanner.Text()) == "#BEGIN_TB" {
			threadBlocklines := make([]string, 0) // [note] store whole lines of a thread block
			for tg.scanner.Scan() {
				if strings.TrimSpace(tg.scanner.Text()) == "#END_TB" {
					tb := parseThreadBlocks(threadBlocklines)
					tb.parent = tg
					tg.ThreadBlockQueue.PushBack(tb)
					break
				}
				
				threadBlocklines = append(threadBlocklines, tg.scanner.Text())
			}
		}
	}
}
