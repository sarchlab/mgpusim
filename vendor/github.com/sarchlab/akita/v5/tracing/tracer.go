package tracing

// A Tracer can collect task traces. Each method receives the event struct for
// the corresponding trace point.
type Tracer interface {
	StartTask(t TaskStart)
	EndTask(t TaskEnd)
	AddTaskTag(tag TaskTag)
	AddMilestone(m Milestone)
}

// NopTracer is an embeddable Tracer whose methods all do nothing. Embed it so a
// tracer only has to implement the methods it cares about.
type NopTracer struct{}

// StartTask does nothing.
func (NopTracer) StartTask(TaskStart) {}

// EndTask does nothing.
func (NopTracer) EndTask(TaskEnd) {}

// AddTaskTag does nothing.
func (NopTracer) AddTaskTag(TaskTag) {}

// AddMilestone does nothing.
func (NopTracer) AddMilestone(Milestone) {}
