package trace

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
)

type kernelListReader struct {
	traceDirPath string
	flagHasRead  bool
	kernelList   *kernelList
}

type kernelList struct {
	listDirPath string
	traceExecs  []traceExecs
}

type traceExecs interface {
	execType() string
}

type kernel struct { // trace execs interface
	rawText    string
	fileName   string
	filePath   string
	traceGroup *traceGroupReader
}

type memCopy struct { // trace execs interface
	rawText   string
	h2d       bool
	startAddr uint64
	length    uint64
}

func (te *kernel) execType() string {
	return "kernel"
}

func (te *memCopy) execType() string {
	return "memcopy"
}

func NewKLReader(traceDirPath string) *kernelListReader {
	return &kernelListReader{
		traceDirPath: traceDirPath,
		flagHasRead:  false,
		kernelList: &kernelList{
			listDirPath: traceDirPath,
			traceExecs:  make([]traceExecs, 0),
		},
	}
}

func (t *kernelListReader) Read() *nvidia.KernelList {
	if t.flagHasRead {
		return t.kernelList.convertToNV()
	}

	lines := readKernelsList(t.traceDirPath)
	for _, line := range lines {
		te := parseListToTraceExecs(line, t)
		t.kernelList.traceExecs = append(t.kernelList.traceExecs, te)
	}

	t.flagHasRead = true
	return t.kernelList.convertToNV()
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

func parseListToTraceExecs(rawText string, trace *kernelListReader) traceExecs {
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
	}

	if strings.HasPrefix(rawText, "kernel") {
		/*
			format  : kernel name
			example : kernel_0
		*/
		k := &kernel{
			rawText:  rawText,
			fileName: rawText,
			filePath: path.Join(trace.traceDirPath, rawText),
		}

		return k
	}

	log.Panicf("Unknown trace group rawText: %s", rawText)
	return nil
}
