// Package noctracing provides a speficied tracer implementation for the mesh.
package noctracing

import (
	"strconv"
	"strings"
	"sync"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/tebeka/atexit"
)

const (
	channelBufferSize int            = 2048
	TimeSliceUnit     sim.VTimeInSec = 0.000001000
)

// TaskWorkers stores the data to the trace file.
type TaskWorkers struct {
	//
	// +--------------+             +----------------------+
	// |              |   channel   |                      |
	// |   End Task   +------------>|   Hungry Consumers   |
	// |  (Producer)  |  taskQueue  | (Consumers/Producers)|
	// |              |             |                      |
	// +--------------+             +----------+-----------+
	//                                 channel | writeQueue
	//                                         v
	// +--------------+             +----------------------+
	// |              |   MeshInfo  |                      |
	// |   	Metrics   |<------------+    Write Workers     |
	// |   (in file)  |     Dump    |     (Consumers)      |
	// |              |             |                      |
	// +--------------+             +----------------------+
	//
	taskQueue         chan tracing.Task
	writeQueue        chan MeshTraceRecord
	closeTaskQueue    chan struct{}
	closeWriteQueue   chan struct{}
	waitConsumers     *sync.WaitGroup
	waitWriteWorker   *sync.WaitGroup
	numHungryConsumer int
}

// MeshNetworkTracer is a task tracer that can store the tasks of mesh tracing
// into a series metrics files.
type MeshNetworkTracer struct {
	timeTeller   sim.TimeTeller
	tracingTasks map[string]tracing.Task
	startTime    sim.VTimeInSec
	endTime      sim.VTimeInSec
	workers      TaskWorkers
	mesh         *MeshInfo
}

// MeshTraceRecord is the record of a mesh tracing.
// Note: only support one GPU with one mesh currently
type MeshTraceRecord struct {
	timeSlice      uint
	isGlobalRecord bool
	srcTile        [3]int
	dstTile        [3]int
	msgTypeID      uint8 // Msg type ID
}

// Init create the goroutines for task workers.
func (t *MeshNetworkTracer) Init() {
	t.workers.LaunchHungryConsumers()
	t.workers.LaunchWriteWorker(t.mesh)
}

// StartTask marks the start of a task.
func (t *MeshNetworkTracer) StartTask(task tracing.Task) {
	task.StartTime = t.timeTeller.CurrentTime()

	if t.endTime > 0 && task.StartTime > t.endTime {
		return
	}

	if task.ID == "" {
		panic("task id is empty")
	}

	t.tracingTasks[task.ID] = task
}

// StepTask marks a milestone during the execution of a task.
func (t *MeshNetworkTracer) StepTask(task tracing.Task) {
	// Do nothing for now
}

// EndTask writes the task into the database.
func (t *MeshNetworkTracer) EndTask(task tracing.Task) {
	task.EndTime = t.timeTeller.CurrentTime()

	if t.startTime > 0 && task.EndTime < t.startTime {
		delete(t.tracingTasks, task.ID)
		return
	}

	originalTask, ok := t.tracingTasks[task.ID]
	if !ok {
		// fmt.Println("task is not started")
		return
	}

	originalTask.EndTime = task.EndTime
	originalTask.Detail = task.Detail // seems always `nil` in consumer routines
	delete(t.tracingTasks, task.ID)

	t.workers.taskQueue <- originalTask
}

// DiscretizeVTimeInSecToSlicedTimeInUint discretize the time into uint.
func DiscretizeVTimeInSecToSlicedTimeInUint(time sim.VTimeInSec) (uint, bool) {
	var idx = -1
	// like floor() method
	for time >= 0.0 {
		time -= TimeSliceUnit
		idx++
	}
	return uint(idx), time < 0.0 // true: has precision loss or time is invalid
}

// AppendRecord appends a record to the task worker.
func (w *TaskWorkers) AppendRecord(r MeshTraceRecord) {
	w.writeQueue <- r // prevent goroutine racing for map[key]
}

// LaunchWriteWorker launches the write worker.
func (w *TaskWorkers) LaunchWriteWorker(m *MeshInfo) {
	// Here we provide channel to serialize the recording, instead of simply using
	// mutex to append value to mesh info struct. In this way, we preserve the
	// opportunity to parallelize the write workers.
	w.waitWriteWorker.Add(1)
	go func() {
		finished := false
		for !finished {
			select {
			case r := <-w.writeQueue:
				if r.isGlobalRecord {
					m.AppendOverviewInfo(r.timeSlice, r.msgTypeID)
				} else {
					m.AppendEdgeInfo(r.timeSlice, r.srcTile, r.dstTile, r.msgTypeID)
				}
			case <-w.closeWriteQueue:
				finished = true
			}
		}
		w.waitWriteWorker.Done()
	}()
}

func str2int(str string) int {
	temp, err := strconv.Atoi(str)
	if err != nil {
		panic(err)
	}
	return temp
}

func exactIndexFromSwitchName(name string) [3]int {
	seg := strings.SplitN(name, "[", 4)
	x, y, z := seg[1], seg[2], seg[3]
	return [3]int{str2int(x[:len(x)-1]), str2int(y[:len(y)-1]), str2int(z[:len(z)-1])}
}

// ParseTileIDFromTaskWhereString is a parser to extract tile IDs from string
// related to network tracing records.
func ParseTileIDFromTaskWhereString(where string) (from, to [3]int) {
	// v3 example: "GPU1.SW[3][1][0].Bottom.GPU1.SW[4][1][0].Top"
	//   segments:  0    1           2      3    4           5

	seg := strings.SplitN(where, ".", 6)

	// if seg[0] != seg[3] {
	// 	panic("Mesh network tracers do not spport cross-GPU connection " + where)
	// }

	return exactIndexFromSwitchName(seg[1]), exactIndexFromSwitchName(seg[4])
}

// LaunchHungryConsumers launches the hungry consumer worker.
func (w *TaskWorkers) LaunchHungryConsumers() {
	for i := 0; i < w.numHungryConsumer; i++ {
		w.waitConsumers.Add(1)
		go func() {
			finished := false
			for !finished {
				select {
				case task := <-w.taskQueue:
					if task.What != "flit_through_channel" {
						break // jump out of select, continue loop
					}
					// calculate the interval coverage of sliced time
					start, _ := DiscretizeVTimeInSecToSlicedTimeInUint(task.StartTime)
					end, loss := DiscretizeVTimeInSecToSlicedTimeInUint(task.EndTime)
					if loss {
						end++
					}
					// fmt.Println(task.Where)

					tileFrom, tileTo := ParseTileIDFromTaskWhereString(task.Where)

					// example: "flit.*mem.ReadReq"
					// index:    0    5
					msgType := task.Kind[5:]
					for sliceID := start; sliceID <= end; sliceID++ {
						w.AppendRecord(MeshTraceRecord{
							timeSlice: sliceID,
							srcTile:   tileFrom,
							dstTile:   tileTo,
							msgTypeID: MeshMsgTypesToTrace[msgType],
						})
						w.AppendRecord(MeshTraceRecord{
							timeSlice:      sliceID,
							isGlobalRecord: true, // store as mesh overview info
							msgTypeID:      MeshMsgTypesToTrace[msgType],
						})
					}
				case <-w.closeTaskQueue:
					finished = true
				}
			}
			w.waitConsumers.Done()
		}()
	}
}

// NewMeshNetworkTracerWithTimeRange creates a MeshTracer which can only trace
// the tasks that at least partially overlaps with the given start and end time.
// If the start time is negative, the tracer will start tracing at the beginning
// of the simulation. If the end time is negative, the tracer will not stop
// tracing until the end of the simulation.
func NewMeshNetworkTracerWithTimeRange(
	timeTeller sim.TimeTeller,
	startTime, endTime sim.VTimeInSec,
	numHungryConsumer int,
	tileWidth, tileHeight uint16,
	outputDirName string,
) *MeshNetworkTracer {
	if startTime >= 0 && endTime >= 0 {
		if startTime >= endTime {
			panic("startTime cannot be greater than endTime")
		}
	}

	t := &MeshNetworkTracer{
		timeTeller:   timeTeller,
		startTime:    startTime,
		endTime:      endTime,
		tracingTasks: make(map[string]tracing.Task),
		workers: TaskWorkers{
			taskQueue:         make(chan tracing.Task, channelBufferSize),
			writeQueue:        make(chan MeshTraceRecord, channelBufferSize),
			closeTaskQueue:    make(chan struct{}), // taskQueue close signal
			closeWriteQueue:   make(chan struct{}), // writeQueue close signal
			waitConsumers:     new(sync.WaitGroup),
			waitWriteWorker:   new(sync.WaitGroup),
			numHungryConsumer: numHungryConsumer,
		},
		mesh: MakeMeshInfo(tileWidth, tileHeight, float64(TimeSliceUnit)),
	}

	atexit.Register(func() {
		close(t.workers.closeTaskQueue)
		t.workers.waitConsumers.Wait()
		close(t.workers.closeWriteQueue)
		t.workers.waitWriteWorker.Wait()
		t.mesh.DumpToFiles(outputDirName)
	})

	return t
}

// NewMeshNetworkTracer returns a new MeshNetworkTracer.
func NewMeshNetworkTracer(
	timeTeller sim.TimeTeller,
	numHungryConsumer int,
	tileWidth, tileHeight uint16,
	outputDirName string,
) *MeshNetworkTracer {
	return NewMeshNetworkTracerWithTimeRange(
		timeTeller,
		-1,
		-1,
		numHungryConsumer,
		tileWidth,
		tileHeight,
		outputDirName,
	)
}
