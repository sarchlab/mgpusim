package sample

import (
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/datarecording"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp"
)

const kernelDispatchTable = "mgpusim_kernel_dispatches"

type kernelDispatchRow struct {
	TaskID                 uint64
	Location               string
	Kernel                 string
	GridX, GridY, GridZ    uint32
	GroupX, GroupY, GroupZ uint16
	StartPS, EndPS         int64
}

type kernelDispatchRecorder struct {
	tasks.NopObserver
	recorder datarecording.DataRecorder
}

func (r *reporter) injectKernelDispatchRecorder(s *simulation.Simulation) {
	r.dataRecorder.CreateTable(kernelDispatchTable, kernelDispatchRow{})
	observer := &kernelDispatchRecorder{recorder: r.dataRecorder}
	for _, component := range s.Components() {
		if processor, ok := component.(*cp.Comp); ok {
			s.TaskTracker().Subscribe(observer, processor)
		}
	}
}

// TaskEnded records the command processor's dispatch interval, independent of
// time spent waiting in the simulated driver's queue or returning a completion.
func (r *kernelDispatchRecorder) TaskEnded(task tasks.Task) {
	if task.Kind != tasks.KindReqIn || task.What != "LaunchKernelReq" {
		return
	}
	msg, ok := task.Detail.(messaging.Msg)
	if !ok {
		return
	}
	req, ok := msg.Payload.(protocol.LaunchKernelReq)
	if !ok {
		return
	}

	packet := req.Packet
	r.recorder.InsertData(kernelDispatchTable, kernelDispatchRow{
		TaskID: task.ID, Location: task.Location, Kernel: req.CodeObject.Symbol.Name,
		GridX: packet.GridSizeX, GridY: packet.GridSizeY, GridZ: packet.GridSizeZ,
		GroupX: packet.WorkgroupSizeX, GroupY: packet.WorkgroupSizeY, GroupZ: packet.WorkgroupSizeZ,
		StartPS: int64(task.StartTime), EndPS: int64(task.EndTime),
	})
}
