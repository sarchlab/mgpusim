package rdma

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type milestoneRecorder struct {
	tasks.NopObserver
	milestones []tasks.Milestone
}

func (r *milestoneRecorder) MilestoneAdded(m tasks.Milestone) { r.milestones = append(r.milestones, m) }

func tracedRDMA(t *testing.T) (*sim.Simulation, *Comp, *milestoneRecorder) {
	t.Helper()
	s := sim.MakeBuilder().WithTaskTracking().Build()
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})
	c := MakeBuilder().WithSimulation(s).WithSpec(func() Spec {
		spec := DefaultSpec()
		spec.LocalModules = mem.AddressMapper{Destinations: []messaging.RemotePort{"LocalCache"}}
		spec.RemoteRDMAAddressTable = mem.AddressMapper{Destinations: []messaging.RemotePort{"RemoteGPU"}}
		return spec
	}()).Build("RDMAEngine")
	for _, name := range []string{"RDMARequestInside",
		"RDMARequestOutside",
		"RDMADataInside",
		"RDMADataOutside",
		"Ctrl",
		"AccessCounter"} {
		p := twowaybuffered.MakeBuilder().
			WithSimulation(s).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 4, OutgoingCapacity: 4}).
			Build()
		c.BindPort(name, p)
		(&noopConn{}).BindPort(p)
	}
	rec := &milestoneRecorder{}
	s.TaskTracker().Subscribe(rec, c)
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	return s, c, rec
}

// Exercise real request tasks, including their independent sender/receiver IDs.
// Arrival ends the remote wait even when forwarding is blocked, and retries do
// not duplicate that milestone.
func TestRDMARemoteMilestone(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		name := "available"
		if blocked {
			name = "egress-full"
		}
		t.Run(name, func(t *testing.T) {
			checkRemoteMilestone(t, blocked)
		})
	}
}

func checkRemoteMilestone(t *testing.T, blocked bool) {
	t.Helper()
	s, c, rec := tracedRDMA(t)
	c.Ports.RDMARequestInside.Deliver(
		messaging.Msg{
			ID:      s.NewID(),
			Src:     "LocalCache",
			Dst:     c.Ports.RDMARequestInside.AsRemote(),
			Payload: memprotocol.ReadReq{Address: 64, AccessByteSize: 4},
		},
	)
	if err := s.Engine().(*timing.SerialEngine).RunUntil(1000); err != nil {
		t.Fatal(err)
	}
	if len(c.State.TransactionsFromInside) != 1 {
		t.Fatal("request was not forwarded")
	}
	trans := c.State.TransactionsFromInside[0]
	if trans.RecvTaskID == 0 || trans.SendTaskID == 0 || trans.SendTaskID == trans.ForwardedReqID {
		t.Fatal("missing independent task handles")
	}
	if blocked {
		for c.Ports.RDMARequestInside.CanSend() {
			c.Ports.RDMARequestInside.Send(
				messaging.Msg{
					ID:      s.NewID(),
					Src:     c.Ports.RDMARequestInside.AsRemote(),
					Dst:     "LocalCache",
					Payload: memprotocol.WriteDoneRsp{},
				},
			)
		}
	}
	c.Ports.RDMARequestOutside.Deliver(
		messaging.Msg{
			ID:      s.NewID(),
			Src:     "RemoteGPU",
			Dst:     c.Ports.RDMARequestOutside.AsRemote(),
			RspTo:   trans.ForwardedReqID,
			Payload: memprotocol.DataReadyRsp{Data: []byte{1, 2, 3, 4}},
		},
	)
	if err := s.Engine().(*timing.SerialEngine).RunUntil(2000); err != nil {
		t.Fatal(err)
	}
	c.Middlewares.Forward.Tick()
	count := 0
	for _, m := range rec.milestones {
		if m.TaskID == trans.RecvTaskID && m.Kind == tasks.MilestoneKindData && m.What == "remote" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one remote milestone at arrival, got %d: %+v", count, rec.milestones)
	}
	if blocked && len(c.State.TransactionsFromInside) != 1 {
		t.Fatal("response bypassed egress backpressure")
	}
}
