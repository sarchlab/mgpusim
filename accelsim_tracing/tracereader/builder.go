package tracereader

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
)

type TraceReaderBuilder struct {
	traceDirPath string
}

func (b *TraceReaderBuilder) WithTraceDirectory(traceDirPath string) *TraceReaderBuilder {
	b.traceDirPath = traceDirPath
	return b
}

func (b *TraceReaderBuilder) Build() *TraceReader {
	b.traceDirectoryMustBeSet()

	tdr := &TraceReader{
		directoryPath: b.traceDirPath,
		execMetas:     make([]TraceExecMeta, 0),
	}
	tdr.generateExcutions()

	return tdr
}

const kernelsListFileName = "kernelslist"

func (r *TraceReader) generateExcutions() {
	filepath := path.Join(r.directoryPath, kernelsListFileName)
	file, err := os.Open(filepath)
	if err != nil {
		log.Panic(err)
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		if scanner.Text() != "" {
			te := r.BuildExecFromText(scanner.Text())
			r.execMetas = append(r.execMetas, te)
		}
	}
}

const execMemcpyPrefix = "Memcpy"
const execKernelPrefix = "kernel"

func (r *TraceReader) BuildExecFromText(text string) TraceExecMeta {
	m := TraceExecMeta{}

	if strings.HasPrefix(text, execMemcpyPrefix) {
		m.execType = nvidia.ExecMemcpy

		textSplited := strings.SplitN(text, ",", 2)
		directionStr := textSplited[0]

		_, err := fmt.Sscanf(textSplited[1], "%v,%v", &m.Address, &m.Length)
		if err != nil {
			log.Panic(err)
		}

		switch directionStr {
		case string(nvidia.H2D):
			m.Direction = nvidia.H2D
		case string(nvidia.D2H):
			m.Direction = nvidia.H2D
		}

		return m
	}

	if strings.HasPrefix(text, execKernelPrefix) {
		m.execType = nvidia.ExecKernel
		m.filename = text
		m.filepath = path.Join(r.directoryPath, text)
	} else {
		log.Panic("Unknown execution type")
	}

	return m
}

func (b *TraceReaderBuilder) traceDirectoryMustBeSet() {
	if b.traceDirPath == "" {
		log.Panic("traceDirPath must be set")
	}
}
