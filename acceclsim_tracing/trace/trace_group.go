package trace

import (
	"bufio"
	"container/list"
	"log"
	"os"
	"strings"

	"github.com/sarchlab/mgpusim/accelsim_tracing/gpu"
)

type traceGroup struct {
	filePath             string
	file                 *os.File
	scanner              *bufio.Scanner
	hasParsedTraceHeader bool
	traceHeader          *traceHeader
	threadBlockQueue     *list.List
}

func NewTraceGroup() *traceGroup {
	return &traceGroup{
		filePath:         "",
		threadBlockQueue: list.New(),
	}
}

func (tg *traceGroup) WithFilePath(path string) *traceGroup {
	tg.filePath = path
	return tg
}

func (tg *traceGroup) Build() {
	tg.buildFileScanner()
	tg.parseTraceHeader()
}

func (tg *traceGroup) Exec(gpu *gpu.GPU) error {
	// [todo] threadblocks can be parallelized to save memory
	tg.parseThreadBlocks()

	for it := tg.threadBlockQueue.Front(); it != nil; it = it.Next() {
		gpu.RunThreadBlock(it.Value.(*threadBlock).generateNVThreadBlock())
	}

	tg.file.Close()
	return nil
}

func (tg *traceGroup) buildFileScanner() {
	file, err := os.Open(tg.filePath)
	if err != nil {
		log.Panic(err)
	}
	tg.file = file // [note] close after exec
	tg.scanner = bufio.NewScanner(file)
}

func (tg *traceGroup) parseTraceHeader() {
	if tg.hasParsedTraceHeader {
		return
	}

	headerLines := make([]string, 0)
	for tg.scanner.Scan() { // [note] get prefix lines that start with "-"
		if strings.HasPrefix(tg.scanner.Text(), "-") {
			headerLines = append(headerLines, tg.scanner.Text())
		} else if tg.scanner.Text() != "" {
			break
		}
	}

	tg.traceHeader = parseHeaderParam(headerLines)
	tg.hasParsedTraceHeader = true
	tg.traceHeader.parent = tg
}

func (tg *traceGroup) parseThreadBlocks() {
	if !tg.hasParsedTraceHeader {
		log.Panic("Trace header has not been parsed")
	}
	for tg.scanner.Scan() {
		if strings.TrimSpace(tg.scanner.Text()) == "#BEGIN_TB" {
			threadBlocklines := make([]string, 0) // [note] store whole lines of a thread block
			for tg.scanner.Scan() {
				if strings.TrimSpace(tg.scanner.Text()) == "#END_TB" {
					tb := parseThreadBlocks(threadBlocklines)
					tb.parent = tg
					tg.threadBlockQueue.PushBack(tb)
					break
				}
				threadBlocklines = append(threadBlocklines, tg.scanner.Text())
			}
		}
	}
}
