package tracing

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/sarchlab/akita/v5/datarecording"
	"github.com/sarchlab/akita/v5/timing"
)

// Table names for tracing data.
// TODO(v5): Rename to "daisen$tasks" and "daisen$milestones" for clearer namespacing.
const (
	traceTableName     = "trace"
	milestoneTableName = "milestone"
	tagTableName       = "tag"
	segmentTableName   = "daisen$segments"
)

type runningTask struct {
	Task

	toRecord bool
}

// taskTableEntry is the table structure for storing task information.
// All tasks are stored in a single "trace" table.
type taskTableEntry struct {
	ID        uint64  `json:"id" akita_data:"unique"`
	ParentID  uint64  `json:"parent_id" akita_data:"index"`
	Kind      string  `json:"kind" akita_data:"index"`
	What      string  `json:"what" akita_data:"index"`
	Location  string  `json:"location" akita_data:"location"`
	StartTime float64 `json:"start_time" akita_data:"index"`
	EndTime   float64 `json:"end_time" akita_data:"index"`
}

// milestoneTableEntry is the table structure for storing milestone information.
// All milestones are stored in a single "milestone" table. A milestone's
// location is inherited from its owning task (joined via TaskID), so it is not
// stored here.
type milestoneTableEntry struct {
	ID     uint64  `json:"id" akita_data:"unique"`
	TaskID uint64  `json:"task_id" akita_data:"index"`
	Time   float64 `json:"time" akita_data:"index"`
	Kind   string  `json:"kind" akita_data:"index"`
	What   string  `json:"what" akita_data:"index"`
}

// tagTableEntry is the table structure for storing task tag information. All
// tags are stored in a single "tag" table. Location is inherited from the
// owning task.
type tagTableEntry struct {
	ID     uint64  `json:"id" akita_data:"unique"`
	TaskID uint64  `json:"task_id" akita_data:"index"`
	Time   float64 `json:"time" akita_data:"index"`
	What   string  `json:"what" akita_data:"index"`
}

// segmentTableEntry is the table structure for storing tracing segment information.
// A segment represents a time period between StartTracing and StopTracing calls.
type segmentTableEntry struct {
	StartTime float64 `json:"start_time" akita_data:"index"`
	EndTime   float64 `json:"end_time" akita_data:"index"`
}

// DBTracer is a tracer that can store tasks into a database.
// DBTracers can connect with different backends so that the tasks can be stored
// in different types of databases (e.g., CSV files, SQL databases, etc.)
type DBTracer struct {
	mu         sync.Mutex
	timeTeller timing.TimeTeller
	backend    datarecording.DataRecorder

	tracingTasks     map[uint64]*runningTask
	isTracing        bool
	tracingStartTime timing.VTimeInPicoSec

	terminated              bool
	firstTerminateBacktrace string
}

// IsTracing reports whether the tracer is currently tracing.
func (t *DBTracer) IsTracing() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.isTracing
}

// StartTask marks the start of a task.
func (t *DBTracer) StartTask(task TaskStart) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.startingTaskMustBeValid(task)

	// A task may first be mentioned by a tag or a milestone.
	rt, found := t.tracingTasks[task.ID]
	if !found {
		rt = &runningTask{}
		t.tracingTasks[task.ID] = rt
	}

	rt.ID = task.ID
	rt.ParentID = task.ParentID
	rt.Kind = task.Kind
	rt.What = task.What
	rt.Location = task.Location
	rt.StartTime = task.Time

	if t.isTracing {
		rt.toRecord = true
	}
}

func (t *DBTracer) startingTaskMustBeValid(task TaskStart) {
	if task.ID == 0 {
		panic("task ID must be set")
	}

	if task.Kind == "" {
		panic("task kind must be set")
	}

	if task.What == "" {
		panic("task what must be set")
	}

	if task.Location == "" {
		panic("task location must be set")
	}
}

// AddTaskTag records a tag on a task.
func (t *DBTracer) AddTaskTag(tag TaskTag) {
	t.mu.Lock()
	defer t.mu.Unlock()

	task, found := t.tracingTasks[tag.TaskID]
	if !found {
		task = &runningTask{}
		task.ID = tag.TaskID
		t.tracingTasks[tag.TaskID] = task
	}

	task.Tags = append(task.Tags, tag)
}

// AddMilestone adds a milestone.
func (t *DBTracer) AddMilestone(milestone Milestone) {
	t.mu.Lock()
	defer t.mu.Unlock()

	task, found := t.tracingTasks[milestone.TaskID]
	if !found {
		task = &runningTask{}
		task.ID = milestone.TaskID
		t.tracingTasks[milestone.TaskID] = task
	}

	for _, existingMilestone := range task.Milestones {
		if sameMilestone(existingMilestone, milestone) {
			return
		}

		// Only record the first milestone if multiple milestones occur at the same time
		if existingMilestone.Time == milestone.Time {
			return
		}
	}

	task.Milestones = append(task.Milestones, milestone)
}

func sameMilestone(a, b Milestone) bool {
	return a.Kind == b.Kind && a.What == b.What
}

// EndTask marks the end of a task.
func (t *DBTracer) EndTask(task TaskEnd) {
	t.mu.Lock()
	defer t.mu.Unlock()

	originalTask, ok := t.tracingTasks[task.ID]
	if !ok {
		return
	}

	originalTask.EndTime = task.Time
	delete(t.tracingTasks, task.ID)

	if !originalTask.toRecord {
		return
	}

	// Write task to the trace table
	entry := taskTableEntry{
		ID:        originalTask.ID,
		ParentID:  originalTask.ParentID,
		Kind:      originalTask.Kind,
		What:      originalTask.What,
		Location:  originalTask.Location,
		StartTime: float64(originalTask.StartTime),
		EndTime:   float64(originalTask.EndTime),
	}
	t.backend.InsertData(traceTableName, entry)

	// Write milestones to the milestone table. A milestone's location is
	// the task's location, recovered by readers via the TaskID join.
	for _, m := range originalTask.Milestones {
		milestoneEntry := milestoneTableEntry{
			ID:     m.ID,
			TaskID: m.TaskID,
			Time:   float64(m.Time),
			Kind:   string(m.Kind),
			What:   m.What,
		}
		t.backend.InsertData(milestoneTableName, milestoneEntry)
	}

	// Write tags to the tag table.
	for _, tag := range originalTask.Tags {
		tagEntry := tagTableEntry{
			ID:     tag.ID,
			TaskID: tag.TaskID,
			Time:   float64(tag.Time),
			What:   tag.What,
		}
		t.backend.InsertData(tagTableName, tagEntry)
	}
}

// Terminate terminates the tracer. Terminated traces cannot be used again.
func (t *DBTracer) Terminate() {
	t.mu.Lock()

	// Check for double termination
	currentBacktrace := captureBacktrace()
	if t.terminated {
		fmt.Println("ERROR: DBTracer.Terminate called multiple times!")
		fmt.Println("First termination backtrace:")
		fmt.Println(t.firstTerminateBacktrace)
		fmt.Println("Second termination backtrace:")
		fmt.Println(currentBacktrace)
		t.mu.Unlock()
		return
	}

	t.terminated = true
	t.firstTerminateBacktrace = currentBacktrace
	t.mu.Unlock()

	// If tracing is still going, stop it first.
	if t.IsTracing() {
		t.StopTracing()
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.tracingTasks = nil
	t.backend.Flush()
}

func captureBacktrace() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// StartTracing manually enables tracing.
// All ongoing tasks are marked for recording since their time range overlaps
// with the tracing period.
func (t *DBTracer) StartTracing() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.isTracing = true
	t.tracingStartTime = t.timeTeller.CurrentTime()

	// Mark all ongoing tasks for recording
	for _, task := range t.tracingTasks {
		task.toRecord = true
	}
}

// StopTracing stops tracing and finalizes tasks.
func (t *DBTracer) StopTracing() {
	t.mu.Lock()
	defer t.mu.Unlock()

	endTime := t.timeTeller.CurrentTime()

	// Insert segment entry
	segment := segmentTableEntry{
		StartTime: float64(t.tracingStartTime),
		EndTime:   float64(endTime),
	}
	t.backend.InsertData(segmentTableName, segment)

	t.isTracing = false

	// Flush to ensure all data is written to database immediately
	t.backend.Flush()
}

// NewDBTracer creates a new DBTracer.
func NewDBTracer(
	timeTeller timing.TimeTeller,
	dataRecorder datarecording.DataRecorder,
) *DBTracer {
	dataRecorder.CreateTable(traceTableName, taskTableEntry{})
	dataRecorder.CreateTable(milestoneTableName, milestoneTableEntry{})
	dataRecorder.CreateTable(tagTableName, tagTableEntry{})
	dataRecorder.CreateTable(segmentTableName, segmentTableEntry{})

	t := &DBTracer{
		timeTeller:   timeTeller,
		backend:      dataRecorder,
		tracingTasks: make(map[uint64]*runningTask),
	}

	return t
}
