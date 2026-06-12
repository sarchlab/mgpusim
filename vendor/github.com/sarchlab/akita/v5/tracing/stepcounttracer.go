package tracing

import (
	"sync"
)

// StepCountTracer can collect the total time of a certain step is triggerred.
type StepCountTracer struct {
	filter            TaskFilter
	lock              sync.Mutex
	inflightTasks     map[uint64]Task
	stepNames         []string
	stepCount         map[string]uint64
	taskWithStepCount map[string]uint64
}

// NewStepCountTracer creates a new StepCountTracer
func NewStepCountTracer(filter TaskFilter) *StepCountTracer {
	t := &StepCountTracer{
		filter:            filter,
		inflightTasks:     make(map[uint64]Task),
		stepCount:         make(map[string]uint64),
		taskWithStepCount: make(map[string]uint64),
	}

	return t
}

// GetStepNames returns all the step names collected.
func (t *StepCountTracer) GetStepNames() []string {
	return t.stepNames
}

// GetStepCount returns the number of steps that is recorded with a certain step
// name.
func (t *StepCountTracer) GetStepCount(stepName string) uint64 {
	return t.stepCount[stepName]
}

// GetTaskCount returns the number of tasks that is recorded to have a certain
// step with a given name.
func (t *StepCountTracer) GetTaskCount(stepName string) uint64 {
	return t.taskWithStepCount[stepName]
}

// StartTask records the task start time
func (t *StepCountTracer) StartTask(task Task) {
	if !t.filter(task) {
		return
	}

	t.lock.Lock()
	t.inflightTasks[task.ID] = task
	t.lock.Unlock()
}

// StepTask does nothing
func (t *StepCountTracer) StepTask(task Task) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.countStep(task)
	t.countTask(task)
}

func (t *StepCountTracer) countStep(task Task) {
	step := task.Steps[0]

	_, ok := t.stepCount[step.What]
	if !ok {
		t.stepNames = append(t.stepNames, step.What)
	}

	t.stepCount[step.What]++
}

func (t *StepCountTracer) countTask(task Task) {
	step := task.Steps[0]

	originalTask, ok := t.inflightTasks[task.ID]
	if !ok {
		return
	}

	if !taskContainsStep(originalTask, step) {
		t.taskWithStepCount[step.What]++
	}

	originalTask.Steps = append(originalTask.Steps, step)
}

func taskContainsStep(task Task, step TaskStep) bool {
	for _, s := range task.Steps {
		if s.What == step.What {
			return true
		}
	}

	return false
}

// StepCountTracer does nothing
func (t *StepCountTracer) AddMilestone(_ Milestone) {
	// Do nothing
}

// EndTask records the end of the task
func (t *StepCountTracer) EndTask(task Task) {
	t.lock.Lock()

	_, ok := t.inflightTasks[task.ID]
	if !ok {
		t.lock.Unlock()
		return
	}

	delete(t.inflightTasks, task.ID)
	t.lock.Unlock()
}
