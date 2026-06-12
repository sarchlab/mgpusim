package tracing

// A Tracer can collect task traces
type Tracer interface {
	StartTask(task Task)
	StepTask(task Task)
	AddMilestone(milestone Milestone)
	EndTask(task Task)
}
