package trace

import (
	"bufio"
	"log"
	"os"
	"path"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpu"
)

type Trace struct {
	traceDirPath string
	traceExecs   []traceExecs
}

func NewTrace() *Trace {
	return &Trace{
		traceDirPath: "",
		traceExecs:   nil,
	}
}

func (t *Trace) WithTraceDirPath(path string) *Trace {
	t.traceDirPath = path
	return t
}

func (t *Trace) Build() {
	t.parseKernelsList()
}

func (t *Trace) Exec(gpu *gpu.GPU) error {
	for _, tg := range t.traceExecs {
		err := tg.Execute(gpu)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *Trace) parseKernelsList() {
	filePath := path.Join(t.traceDirPath, "kernelslist.g")
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

	for _, line := range lines {
		te := parseTraceExecs(line, t)
		t.traceExecs = append(t.traceExecs, te)
	}
}
