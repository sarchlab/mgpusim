package dma

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

// milestoneRecorder is a tasks.Tracer that captures the milestones a
// component emits, so a test can assert blocked-time is being attributed.
type milestoneRecorder struct {
	tasks.NopObserver

	milestones []tasks.Milestone
}

func (r *milestoneRecorder) MilestoneAdded(m tasks.Milestone) {
	r.milestones = append(r.milestones, m)
}

func (r *milestoneRecorder) has(kind tasks.MilestoneKind, what string) bool {
	for _, m := range r.milestones {
		if m.Kind == kind && m.What == what {
			return true
		}
	}

	return false
}

// A completed memory copy must record a "data" milestone on its req_in task,
// marking the end of the time the copy spent waiting on the memory system.
func TestDMARecordsDataMilestoneOnMemCopyComplete(t *testing.T) {
	const memPort = messaging.RemotePort("Mem.Top")

	reg := sim.MakeBuilder().WithTaskTracking().Build()
	t.Cleanup(func() {
		if err := reg.Terminate(); err != nil {
			t.Error(err)
		}
	})
	dma := MakeBuilder().
		WithSimulation(reg).
		WithSpec(func() Spec {
			spec := DefaultSpec()
			spec.LocalDataSource = mem.AddressMapper{Destinations: []messaging.RemotePort{memPort}}
			return spec
		}()).
		Build("DMA")

	assign := func(name string) messaging.Port {
		p := twowaybuffered.MakeBuilder().
			WithSimulation(reg).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
			Build()
		dma.BindPort(name, p)
		(&noopConn{}).BindPort(p)
		return p
	}
	_ = assign("ToCP")
	toMem := assign("ToMem")

	if err := reg.Initialize(); err != nil {
		t.Fatal(err)
	}
	rec := &milestoneRecorder{}
	reg.TaskTracker().Subscribe(rec, dma)
	mw := dma.Middlewares.DMA

	req := messaging.Msg{ID: dma.NewID(), Payload: protocol.MemCopyD2HReq{SrcAddress: 64,
		ByteSize: 64}}

	rqC := NewRequestCollection(req)
	rqC.RecvTaskID = dma.TaskTracker().StartReqIn(dma, req)
	rqC.Data = make([]byte, 64)
	mw.comp.State.ProcessingReqs = append(mw.comp.State.ProcessingReqs, *rqC)
	rqC = &mw.comp.State.ProcessingReqs[len(mw.comp.State.ProcessingReqs)-1]

	read := messaging.Msg{ID: dma.NewID(), Payload: memprotocol.ReadReq{Address: 64,
		AccessByteSize: 64}}

	mw.comp.State.PendingReqs = append(mw.comp.State.PendingReqs, read)
	rqC.appendSubordinateID(read.ID)

	toMem.Deliver(messaging.Msg{ID: dma.NewID(),
		RspTo: read.ID, Payload: memprotocol.DataReadyRsp{Data: make([]byte, 64)}},
	)
	mw.parseFromMem()

	if !rec.has(tasks.MilestoneKindData, "ToMem") {
		t.Fatalf("expected a data milestone on memcopy completion, got %+v",
			rec.milestones)
	}
}
