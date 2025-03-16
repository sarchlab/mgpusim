package tracereader

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/sarchlab/mgpusim/v4/nvidia/nvidiaconfig"
	log "github.com/sirupsen/logrus"
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

const kernelsListFileName = "kernelslist.g"

func (r *TraceReader) generateExcutions() {
	filepath := path.Join(r.directoryPath, kernelsListFileName)
	file, err := os.Open(filepath)
	if err != nil {
		log.WithError(err).WithField("filepath", filepath).Error("Failed to open file")
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
		m.execType = nvidiaconfig.ExecMemcpy

		textSplited := strings.SplitN(text, ",", 2)
		directionStr := textSplited[0]

		_, err := fmt.Sscanf(textSplited[1], "%v,%v", &m.Address, &m.Length)
		if err != nil {
			log.WithError(err).WithField("text", text).Panic("Failed to parse text")
		}

		switch directionStr {
		case string(nvidiaconfig.H2D):
			m.Direction = nvidiaconfig.H2D
		case string(nvidiaconfig.D2H):
			m.Direction = nvidiaconfig.H2D
		}

		return m
	}

	if strings.HasPrefix(text, execKernelPrefix) {
		m.execType = nvidiaconfig.ExecKernel
		m.filename = text
		m.filepath = path.Join(r.directoryPath, text)
	} else {
		log.WithField("text", text).Panic("Unknown execution type")
	}

	return m
}

func (b *TraceReaderBuilder) traceDirectoryMustBeSet() {
	if b.traceDirPath == "" {
		log.Panic("traceDirPath must be set")
	}
}
