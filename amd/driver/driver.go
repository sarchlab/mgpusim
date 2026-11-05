package driver

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// executionMW keeps only its component reference. Every tick reads the current
// State, including after a checkpoint replaces that State.
type executionMW struct{ comp *Comp }

func (m *executionMW) Tick() bool {
	c := m.comp
	progress := m.send()
	if c.State.CopyCyclesLeft > 0 {
		c.State.CopyCyclesLeft--
		progress = true
	} else if c.State.CopyCyclesLeft == 0 {
		c.State.RequestsToSend = append(c.State.RequestsToSend, c.State.CopyAwaiting...)
		c.State.CopyAwaiting = nil
		c.State.CopyCyclesLeft = -1
		progress = true
	}
	progress = m.receive() || progress
	progress = m.startWaitingCopies() || progress
	r := c.Resources.Runtime
	for i := range r.QueueCount() {
		q := gpuruntime.CommandQueue(i + 1)
		if m.active(q) {
			continue
		}
		m.markers(q)
		cmd, ok := r.TakeCommand(q)
		if !ok {
			continue
		}
		c.State.Executing = append(c.State.Executing, Execution{Queue: q, Command: cmd})
		e := &c.State.Executing[len(c.State.Executing)-1]
		m.startTask(e)
		switch cmd.Kind {
		case gpuruntime.LaunchKernel:
			m.launch(e)
		case gpuruntime.CopyH2D, gpuruntime.CopyD2H:
			m.copyUnlessMigrating(len(c.State.Executing) - 1)
		case gpuruntime.Noop:
			m.finish(len(c.State.Executing) - 1)
		default:
			panic("driver: unknown command")
		}
		progress = true
	}
	return progress
}
func (m *executionMW) active(q gpuruntime.CommandQueue) bool {
	for _, e := range m.comp.State.Executing {
		if e.Queue == q {
			return true
		}
	}
	return false
}
func (m *executionMW) markers(q gpuruntime.CommandQueue) {
	r := m.comp.Resources.Runtime
	r.CompleteEvents(q, m.comp.CurrentTime())
}
func (m *executionMW) send() bool {
	c := m.comp
	if len(c.State.RequestsToSend) == 0 || !c.Ports.GPU.CanSend() {
		return false
	}
	c.Ports.GPU.Send(c.State.RequestsToSend[0])
	c.State.RequestsToSend[0] = messaging.Msg{}
	c.State.RequestsToSend = c.State.RequestsToSend[1:]
	return true
}
func (m *executionMW) startTask(e *Execution) {
	c := m.comp
	if tracker := c.TaskTracker(); tracker != nil {
		what := []string{
			"*driver.MemCopyH2DCommand",
			"*driver.MemCopyD2HCommand",
			"*driver.LaunchKernelCommand",
			"Event",
			"*driver.NoopCommand",
		}[e.Command.Kind]
		c.State.CommandTasks[uint64(e.Command.ID)] = tracker.StartTask(
			c,
			tasks.StartInfo{Kind: "Driver Command", What: what, ParentID: c.Resources.Runtime.SimulationTask()},
		)
	}
}
func (m *executionMW) request(e *Execution, gpu int, payload any, delayed bool) {
	c := m.comp
	req := messaging.Msg{ID: c.NewID(), Src: c.Ports.GPU.AsRemote(), Dst: c.Spec.GPUs[gpu-1], Payload: payload}
	e.Requests = append(e.Requests, req)
	if delayed {
		c.State.CopyAwaiting = append(c.State.CopyAwaiting, req)
	} else {
		c.State.RequestsToSend = append(c.State.RequestsToSend, req)
	}
	if tracker := c.TaskTracker(); tracker != nil {
		c.State.RequestTasks[req.ID] = tracker.StartReqOut(c, req, c.State.CommandTasks[uint64(e.Command.ID)])
	}
}
func (m *executionMW) launch(e *Execution) {
	c := m.comp
	r := c.Resources.Runtime
	pid := r.QueuePID(e.Queue)
	cmd := &e.Command
	totalCU := 0
	for _, gpu := range cmd.GPUs {
		totalCU += r.DeviceCUCount(gpu)
	}
	first := uint64(0)
	p := cmd.Packets[0].Packet
	totalWG := uint64((p.GridSizeX-1)/uint32(p.WorkgroupSizeX)+1) *
		uint64((p.GridSizeY-1)/uint32(p.WorkgroupSizeY)+1) *
		uint64((p.GridSizeZ-1)/uint32(p.WorkgroupSizeZ)+1)
	for i, gpu := range cmd.GPUs {
		req := protocol.LaunchKernelReq{
			PID:           pid,
			CodeObject:    cmd.Code.Metadata(),
			Packet:        cmd.Packets[i].Packet,
			PacketAddress: uint64(cmd.PacketAddresses[i]),
		}
		if len(cmd.GPUs) > 1 {
			end := min(totalWG, first+((totalWG-1)/uint64(totalCU)+1)*uint64(r.DeviceCUCount(gpu)))
			if end <= first {
				continue
			}
			req.Selection = kernels.WGSelection{Partitioned: true, First: first, End: end}
			first = end
		}
		m.request(e, gpu, req, false)
	}
	r.MarkBuffersDirty(r.QueueContext(e.Queue))
}
func (m *executionMW) receive() bool {
	c := m.comp
	rsp, ok := c.Ports.GPU.PeekIncoming()
	if !ok {
		return false
	}
	switch rsp.Payload.(type) {
	case protocol.LaunchKernelRsp, protocol.MemCopyD2HRsp, protocol.GeneralRsp:
	default:
		return false
	}
	for i := range c.State.Executing {
		e := &c.State.Executing[i]
		for j, req := range e.Requests {
			if req.ID != rsp.RspTo {
				continue
			}
			if data, ok := rsp.Payload.(protocol.MemCopyD2HRsp); ok {
				copyReq := req.Payload.(protocol.MemCopyD2HReq)
				if uint64(len(data.Data)) != copyReq.ByteSize {
					panic("driver: D2H response size mismatch")
				}
				r := c.Resources.Runtime
				writeHostBytes(r, r.QueueContext(e.Queue), e.Command.Dst+gpuruntime.Ptr(copyReq.DstOffset), data.Data)
			}
			c.Ports.GPU.RetrieveIncoming()
			if tracker := c.TaskTracker(); tracker != nil {
				tracker.EndReqOut(c.State.RequestTasks[req.ID])
			}
			delete(c.State.RequestTasks, req.ID)
			e.Requests = append(e.Requests[:j], e.Requests[j+1:]...)
			if len(e.Requests) == 0 {
				m.finish(i)
			}
			return true
		}
	}
	panic(fmt.Sprintf("driver: unmatched response %d", rsp.RspTo))
}
func (m *executionMW) finish(i int) {
	c := m.comp
	e := c.State.Executing[i]
	if tracker := c.TaskTracker(); tracker != nil {
		tracker.EndTask(c.State.CommandTasks[uint64(e.Command.ID)])
	}
	delete(c.State.CommandTasks, uint64(e.Command.ID))
	c.Resources.Runtime.CompleteCommand(e.Queue, e.Command, c.CurrentTime())
	c.State.Executing = append(c.State.Executing[:i], c.State.Executing[i+1:]...)
	m.markers(e.Queue)
}

func (m *executionMW) Handle(timing.Event) bool { return m.Tick() }
