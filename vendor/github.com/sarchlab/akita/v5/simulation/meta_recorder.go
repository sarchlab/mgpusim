package simulation

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sarchlab/akita/v5/datarecording"
	"github.com/sarchlab/akita/v5/timing"
)

type simulationInfo struct {
	Property string
	Value    string
}

// metaRecorder records simulation-level metadata into exec_info.
type metaRecorder struct {
	tableName  string
	recorder   datarecording.DataRecorder
	timeTeller timing.TimeTeller
	ended      bool
}

func newMetaRecorder(
	recorder datarecording.DataRecorder,
	timeTeller timing.TimeTeller,
) *metaRecorder {
	r := &metaRecorder{
		tableName:  "exec_info",
		recorder:   recorder,
		timeTeller: timeTeller,
	}
	r.recorder.CreateTable(r.tableName, simulationInfo{})
	r.Start()
	return r
}

func (r *metaRecorder) Start() {
	currentTime := time.Now()
	r.insertEntry("Start Time", currentTime.Format("2006-01-02 15:04:05.000000000"))

	r.insertEntry("Command", strings.Join(os.Args, " "))

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}

	r.insertEntry("Working Directory", filepath.Dir(ex))

	if r.timeTeller != nil {
		r.insertEntry("Start Virtual Time",
			strconv.FormatUint(uint64(r.timeTeller.CurrentTime()), 10))
	}

	r.recorder.Flush()
}

func (r *metaRecorder) End() {
	if r.ended {
		return
	}
	r.ended = true

	r.insertEntry("End Time", time.Now().Format("2006-01-02 15:04:05.000000000"))

	if r.timeTeller != nil {
		r.insertEntry("End Virtual Time",
			strconv.FormatUint(uint64(r.timeTeller.CurrentTime()), 10))
	}

	r.recorder.Flush()
}

func (r *metaRecorder) insertEntry(property, value string) {
	r.recorder.InsertData(r.tableName, simulationInfo{
		Property: property,
		Value:    value,
	})
}
