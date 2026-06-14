package tracing

import (
	"sync"
)

// TagCountTracer counts how often each tag name is recorded, and how many
// distinct tasks carry a tag with a given name.
type TagCountTracer struct {
	NopTracer

	filter           TaskFilter
	lock             sync.Mutex
	inflightTasks    map[uint64]map[string]bool // task ID -> set of seen tag names
	tagNames         []string
	tagCount         map[string]uint64
	taskWithTagCount map[string]uint64
}

// NewTagCountTracer creates a new TagCountTracer
func NewTagCountTracer(filter TaskFilter) *TagCountTracer {
	return &TagCountTracer{
		filter:           filter,
		inflightTasks:    make(map[uint64]map[string]bool),
		tagCount:         make(map[string]uint64),
		taskWithTagCount: make(map[string]uint64),
	}
}

// GetTagNames returns all the tag names collected. It returns a copy so the
// caller cannot mutate, or race against appends to, the tracer's internal
// slice.
func (t *TagCountTracer) GetTagNames() []string {
	t.lock.Lock()
	defer t.lock.Unlock()

	names := make([]string, len(t.tagNames))
	copy(names, t.tagNames)

	return names
}

// GetTagCount returns the number of tags recorded with a certain tag name.
func (t *TagCountTracer) GetTagCount(tagName string) uint64 {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.tagCount[tagName]
}

// GetTaskCount returns the number of tasks that carry at least one tag with the
// given name.
func (t *TagCountTracer) GetTaskCount(tagName string) uint64 {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.taskWithTagCount[tagName]
}

// StartTask begins tracking a task's tags.
func (t *TagCountTracer) StartTask(task TaskStart) {
	if !t.filter(task) {
		return
	}

	t.lock.Lock()
	t.inflightTasks[task.ID] = make(map[string]bool)
	t.lock.Unlock()
}

// AddTaskTag counts the tag and the task that carries it.
func (t *TagCountTracer) AddTaskTag(tag TaskTag) {
	t.lock.Lock()
	defer t.lock.Unlock()

	if _, ok := t.tagCount[tag.What]; !ok {
		t.tagNames = append(t.tagNames, tag.What)
	}

	t.tagCount[tag.What]++

	seen, ok := t.inflightTasks[tag.TaskID]
	if !ok {
		return
	}

	if !seen[tag.What] {
		t.taskWithTagCount[tag.What]++
		seen[tag.What] = true
	}
}

// EndTask stops tracking the task.
func (t *TagCountTracer) EndTask(task TaskEnd) {
	t.lock.Lock()
	delete(t.inflightTasks, task.ID)
	t.lock.Unlock()
}
