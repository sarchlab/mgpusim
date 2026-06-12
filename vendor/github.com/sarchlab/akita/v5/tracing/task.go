package tracing

import "github.com/sarchlab/akita/v5/sim"

// A TaskStep represents a milestone in the processing of task
type TaskStep struct {
	Time sim.VTimeInSec `json:"time"`
	What string         `json:"what"`
}

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
// resolved.
type Milestone struct {
	ID       uint64         `json:"id"`
	TaskID   uint64         `json:"task_id"`
	Time     sim.VTimeInSec `json:"time"`
	Kind     MilestoneKind  `json:"kind"`
	What     string         `json:"what"`
	Location string         `json:"location"`
}

// A Task is a task
type Task struct {
	ID         uint64         `json:"id"`
	ParentID   uint64         `json:"parent_id"`
	Kind       string         `json:"kind"`
	What       string         `json:"what"`
	Location   string         `json:"location"`
	StartTime  sim.VTimeInSec `json:"start_time"`
	EndTime    sim.VTimeInSec `json:"end_time"`
	Steps      []TaskStep     `json:"steps"`
	Milestones []Milestone    `json:"milestones"`
	Detail     interface{}    `json:"-"`
	ParentTask *Task          `json:"-"`
}

// TaskFilter is a function that can filter interesting tasks. If this function
// returns true, the task is considered useful.
type TaskFilter func(t Task) bool
