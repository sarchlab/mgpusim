package tracing

import (
	"github.com/sarchlab/akita/v5/timing"
)

// TaskStart carries the information needed to begin a task. Callers pass it to
// [StartTask]. Time is not set by the caller: [StartTask] stamps it from the
// domain clock, after the NumHooks==0 guard, so the clock is never read when
// tracing is disabled.
type TaskStart struct {
	ID       uint64
	ParentID uint64
	Kind     string
	What     string
	Location string // optional; defaults to the domain name when empty
	Time     timing.VTimeInPicoSec
	Detail   any
}

// TaskEnd marks the completion of a task. Callers pass it to [EndTask].
type TaskEnd struct {
	ID   uint64
	Time timing.VTimeInPicoSec
}

// A TaskTag is a categorical label attached to a task while it is processed,
// for example "read-hit" or "write-miss". Tags inherit their location from the
// owning task.
type TaskTag struct {
	ID     uint64                `json:"id"`
	TaskID uint64                `json:"task_id"`
	What   string                `json:"what"`
	Time   timing.VTimeInPicoSec `json:"time"`
}

// MilestoneKind categorizes the blocking condition a milestone resolves.
type MilestoneKind string

const (
	MilestoneKindHardwareResource MilestoneKind = "hardware_resource"
	MilestoneKindNetworkTransfer  MilestoneKind = "network_transfer"
	MilestoneKindNetworkBusy      MilestoneKind = "network_busy"
	MilestoneKindQueue            MilestoneKind = "queue"
	MilestoneKindData             MilestoneKind = "data"
	MilestoneKindDependency       MilestoneKind = "dependency"
	MilestoneKindOther            MilestoneKind = "other"
	MilestoneKindTranslation      MilestoneKind = "translation"
	MilestoneKindSubTask          MilestoneKind = "subtask"
)

// Milestone represents a point in time where a task's blocking status is
// resolved. Its location is inherited from the owning task.
type Milestone struct {
	ID     uint64                `json:"id"`
	TaskID uint64                `json:"task_id"`
	Time   timing.VTimeInPicoSec `json:"time"`
	Kind   MilestoneKind         `json:"kind"`
	What   string                `json:"what"`
}

// A Task is the aggregate record that a stateful tracer builds from the stream
// of trace events.
type Task struct {
	ID         uint64                `json:"id"`
	ParentID   uint64                `json:"parent_id"`
	Kind       string                `json:"kind"`
	What       string                `json:"what"`
	Location   string                `json:"location"`
	StartTime  timing.VTimeInPicoSec `json:"start_time"`
	EndTime    timing.VTimeInPicoSec `json:"end_time"`
	Tags       []TaskTag             `json:"tags"`
	Milestones []Milestone           `json:"milestones"`
	Detail     any                   `json:"-"`
	ParentTask *Task                 `json:"-"`
}

// TaskFilter decides whether a task is interesting. It is evaluated when the
// task starts. Returning true means the task is useful.
type TaskFilter func(t TaskStart) bool
