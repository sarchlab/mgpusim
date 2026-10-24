package cu

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sarchlab/akita/v5/sim/tasks"
)

// leakRecorder observes component work for reset-leak tests. Subscribe before
// driving the component, issue Reset, then check OpenTasks. Port buffer tasks
// are excluded because they close on retrieval rather than component reset.
type leakRecorder struct {
	tasks.NopObserver

	started int
	open    map[uint64]tasks.Task
}

// TaskStarted records a task as open.
func (r *leakRecorder) TaskStarted(s tasks.Task) {
	if s.Kind == tasks.KindIncomingBuffer || s.Kind == tasks.KindOutgoingBuffer {
		return
	}
	if r.open == nil {
		r.open = map[uint64]tasks.Task{}
	}

	r.started++
	r.open[s.ID] = s
}

// TaskEnded removes a task from the open set. Ending a task that was never started
// is a no-op, which is what makes blanket end-on-reset safe.
func (r *leakRecorder) TaskEnded(e tasks.Task) {
	if e.Kind == tasks.KindIncomingBuffer || e.Kind == tasks.KindOutgoingBuffer {
		return
	}
	delete(r.open, e.ID)
}

// NumStarted reports how many tasks were started, so a test can assert its
// traffic actually opened tasks (otherwise an empty open set is vacuous).
func (r *leakRecorder) NumStarted() int {
	return r.started
}

// OpenTasks returns the tasks that were started but not ended, sorted by ID for
// a stable failure message.
func (r *leakRecorder) OpenTasks() []tasks.Task {
	out := make([]tasks.Task, 0, len(r.open))
	for _, s := range r.open {
		out = append(out, s)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })

	return out
}

// OpenSummary renders the still-open tasks as "kind:what" entries for a test
// failure message; it returns "none" when there is no leak.
func (r *leakRecorder) OpenSummary() string {
	open := r.OpenTasks()
	if len(open) == 0 {
		return "none"
	}

	parts := make([]string, len(open))
	for i, s := range open {
		parts[i] = fmt.Sprintf("%s:%s", s.Kind, s.What)
	}

	return strings.Join(parts, ", ")
}
