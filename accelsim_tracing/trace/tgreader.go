package trace

import (
	"bufio"
	"log"
	"os"
	"strings"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
)

type traceGroupReader struct {
	file             *os.File
	scanner          *bufio.Scanner
	flagHeaderParsed bool
	flagTBAllParsed  bool
	traceGroup       *traceGroup
}

type traceGroup struct {
	header       *traceGroupHeader
	threadBlocks []*threadBlock
}

func NewTGReader(filePath string) *traceGroupReader {
	file, err := os.Open(filePath)
	if err != nil {
		log.Panic(err)
	}

	return &traceGroupReader{
		file:             file,
		scanner:          bufio.NewScanner(file),
		flagHeaderParsed: false,
		flagTBAllParsed:  false,
		traceGroup: &traceGroup{
			header:       nil,
			threadBlocks: make([]*threadBlock, 0),
		},
	}

}

func (tg *traceGroupReader) ReadHeader() *nvidia.TraceGroupHeader {
	if tg.flagHeaderParsed {
		return tg.traceGroup.header.convertToNV()
	}

	headerLines := make([]string, 0)
	for tg.scanner.Scan() {
		if strings.HasPrefix(tg.scanner.Text(), "-") {
			headerLines = append(headerLines, tg.scanner.Text())
		} else if tg.scanner.Text() != "" {
			break
		}
	}

	tg.flagHeaderParsed = true
	tg.traceGroup.header = parseHeaderParam(headerLines)

	return tg.traceGroup.header.convertToNV()
}

func (tg *traceGroupReader) ReadNextThreadBlock() *nvidia.ThreadBlock {
	if !tg.flagHeaderParsed {
		tg.ReadHeader()
	}

	if tg.flagTBAllParsed {
		return nil
	}

	if tg.scanner.Scan() {
		if strings.TrimSpace(tg.scanner.Text()) == "#BEGIN_TB" {
			threadBlocklines := make([]string, 0) // [note] store whole lines of a thread block
			for tg.scanner.Scan() {
				if strings.TrimSpace(tg.scanner.Text()) == "#END_TB" {
					tb := parseThreadBlocks(threadBlocklines)
					tb.parent = tg
					tg.traceGroup.threadBlocks = append(tg.traceGroup.threadBlocks, tb)
					return tb.convertToNV()
				}
			}
		}
	}

	tg.flagTBAllParsed = true
	return nil
}

func (tg *traceGroupReader) ReadAll() *nvidia.TraceGroup {
	tg.ReadHeader()
	for {
		tb := tg.ReadNextThreadBlock()
		if tb == nil {
			break
		}
	}
	return tg.traceGroup.convertToNV()
}

func (tg *traceGroupReader) CloseFile() {
	tg.file.Close()
}
