package trace

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
)

type TraceParser struct {
	traceDirPath string
	traceExecs   []TraceExecs
}

type TraceExecs interface {
	Type() string
	File() string
}

func NewTraceParser(path string) *TraceParser {
	return &TraceParser{
		traceDirPath: path,
		traceExecs:   nil,
	}
}

func (t *TraceParser) BuildTraceExecutions() *[]TraceExecs {
	lines := readKernelsList(t.traceDirPath)

	for _, line := range lines {
		te := parseListToTraceExecs(line, t)
		t.traceExecs = append(t.traceExecs, te)
	}

	return &t.traceExecs
}

func readKernelsList(dirPath string) []string {
	filePath := path.Join(dirPath, "kernelslist.g")
	file, err := os.Open(filePath)
	if err != nil {
		log.Panic(err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if scanner.Text() != "" {
			lines = append(lines, scanner.Text())
		}
	}

	return lines
}

func parseListToTraceExecs(rawText string, trace *TraceParser) TraceExecs {
	if strings.HasPrefix(rawText, "Memcpy") {
		/*
			format  : H2D or D2H, start, length
			example : HtoD,0x7f0,0x1000
		*/
		res := strings.Split(rawText, ",")
		m := &memCopy{
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
		k := &Kernel{
			rawText:  rawText,
			fileName: rawText,
			filePath: path.Join(trace.traceDirPath, rawText),
		}
		return k
	}
	log.Panicf("Unknown trace group rawText: %s", rawText)
	return nil
}
