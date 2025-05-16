package rob

import (
	"fmt"
	"strconv"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

// ROBVisTracer to trace the ROB visualization
type ROBVisTracer struct {
	*tracing.DBTracer
	rob        *ReorderBuffer
	timeTeller sim.TimeTeller
}

// NewROBVisTracer creates a new ROB visualization tracer
func NewROBVisTracer(
	timeTeller sim.TimeTeller,
	backend tracing.Tracer,
	rob *ReorderBuffer,
) *ROBVisTracer {
	t := &ROBVisTracer{
		DBTracer:   backend.(*tracing.DBTracer),
		rob:        rob,
		timeTeller: timeTeller,
	}
	return t
}

// StartTask overrides StartTask to trace port status
func (t *ROBVisTracer) StartTask(task tracing.Task) {
	t.DBTracer.StartTask(task)
	t.AddMilestone(tracing.Milestone{
		ID:               strconv.FormatUint(tracing.GenerateMilestoneID(), 10),
		TaskID:           task.ID,
		BlockingCategory: "Port Status",
		BlockingReason:   "Task Enqueued",
		BlockingLocation: task.Where,
		Time:             float64(t.timeTeller.CurrentTime()),
	})
}

// OnPortUpdate is called when the port status is updated
func (t *ROBVisTracer) OnPortUpdate(port sim.Port, item interface{}) {
	if item == nil {
		return
	}

	if req, ok := item.(mem.AccessReq); ok {
		taskID := tracing.MsgIDAtReceiver(req, t.rob)
		fmt.Printf("ROBVisTracer: Port Update - Port: %s, TaskID: %s\n",
			port.Name(), taskID)

		t.AddMilestone(tracing.Milestone{
			ID:               strconv.FormatUint(tracing.GenerateMilestoneID(), 10),
			TaskID:           taskID,
			BlockingCategory: "Port Status",
			BlockingReason:   "Task Front of Port",
			BlockingLocation: port.Name(),
			Time:             float64(t.timeTeller.CurrentTime()),
		})
	}
}
