package datarecording

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Struct ExecInfo is feed to DataRecorder
type execInfo struct {
	Property string
	Value    string
}

// Records program execution
type execRecorder struct {
	tablename string
	recorder  DataRecorder
	entries   []execInfo
}

// Start log current execution.
func (e *execRecorder) Start() {
	currentTime := time.Now()
	startTime := currentTime.Format("2006-01-02 15:04:05.000000000")
	timeEntry := execInfo{"Start Time", startTime}
	e.entries = append(e.entries, timeEntry)

	cmd := strings.Join(os.Args, " ")
	cmdEntry := execInfo{"Command", cmd}
	e.entries = append(e.entries, cmdEntry)

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}

	cwd := filepath.Dir(ex)
	cwdEntry := execInfo{"Working Directory", cwd}
	e.entries = append(e.entries, cwdEntry)
}

// End writes data into SQLite along with program exit time.
func (e *execRecorder) End() {
	for _, entry := range e.entries {
		e.recorder.InsertData(e.tablename, entry)
	}

	endTime := time.Now()
	endValue := endTime.Format("2006-01-02 15:04:05.000000000")
	timeEntry := execInfo{"End Time", endValue}
	e.recorder.InsertData(e.tablename, timeEntry)

	e.entries = nil

	e.recorder.Flush()
}

// newExecRecorderWithWriter creates a new ExecRecorder with given writer
func newExecRecorderWithWriter(writer *sqliteWriter) *execRecorder {
	entrySlice := []execInfo{}

	e := &execRecorder{
		recorder: writer,
		entries:  entrySlice,
	}

	setupTable(e)

	return e
}

func setupTable(e *execRecorder) {
	name := "exec_info"
	e.tablename = name

	sampleEntry := execInfo{}
	e.recorder.CreateTable(name, sampleEntry)
}
