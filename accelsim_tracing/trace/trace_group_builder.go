package trace

import (
	"bufio"
	"log"
	"os"
	"strings"
)

type traceGroupReaderBuilder struct {
	filePath string
}

func NewTraceGroupReaderBuilder() *traceGroupReaderBuilder {
	return &traceGroupReaderBuilder{
		filePath: "",
	}
}

func (tg *traceGroupReaderBuilder) WithFilePath(path string) *traceGroupReaderBuilder {
	tg.filePath = path
	return tg
}

func (tg *traceGroupReaderBuilder) Build() *traceGroupReader {
	tgReader := &traceGroupReader{}
	tg.buildFileScanner(tgReader)
	tg.parseTraceHeader(tgReader)
	return tgReader
}

func (tg *traceGroupReaderBuilder) buildFileScanner(tgReader *traceGroupReader) {
	file, err := os.Open(tg.filePath)
	if err != nil {
		log.Panic(err)
	}
	
	tgReader.File = file // [note] close after exec
	tgReader.scanner = bufio.NewScanner(file)
}

func (tg *traceGroupReaderBuilder) parseTraceHeader(tgReader *traceGroupReader) {
	headerLines := make([]string, 0)
	for tgReader.scanner.Scan() { // [note] get prefix lines that start with "-"
		if strings.HasPrefix(tgReader.scanner.Text(), "-") {
			headerLines = append(headerLines, tgReader.scanner.Text())
		} else if tgReader.scanner.Text() != "" {
			break
		}
	}

	tgReader.traceHeader = parseHeaderParam(headerLines)
}
