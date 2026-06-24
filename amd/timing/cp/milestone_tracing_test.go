package cp

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

// milestoneRecorder is a tracing.Tracer that captures the milestones a
// component emits, so a test can assert blocked-time is being attributed.
type milestoneRecorder struct {
	tracing.NopTracer

	milestones []tracing.Milestone
}

func (r *milestoneRecorder) AddMilestone(m tracing.Milestone) {
	r.milestones = append(r.milestones, m)
}

func (r *milestoneRecorder) has(kind tracing.MilestoneKind, what string) bool {
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

	engine := timing.NewSerialEngine()
	reg := modeling.NewStandaloneRegistrar(engine)
	dma := MakeDMAEngineBuilder().
		WithRegistrar(reg).
		WithResources(DMAResources{
			LocalDataSource: &mem.SinglePortMapper{Port: memPort},
		}).
		Build("DMA")

	assign := func(name string) messaging.Port {
		p := modeling.MakePortBuilder().
			WithRegistrar(reg).
			WithComponent(dma).
			WithSpec(modeling.PortSpec{BufSize: 16}).
			Build(name)
		dma.AssignPort(name, p)
		(&noopConn{}).PlugIn(p)
		return p
	}
	_ = assign("ToCP")
	toMem := assign("ToMem")

	rec := &milestoneRecorder{}
	tracing.CollectTrace(dma, rec)
	mw := dma.Middlewares()[0].(*dmaMiddleware)

	req := protocol.MemCopyD2HReq{
		MsgMeta: messaging.MsgMeta{
			ID: timing.GetIDGenerator().Generate(),
		},
		SrcAddress: 64,
		DstBuffer:  make([]byte, 64),
	}
	rqC := NewRequestCollection(req)
	mw.processingReqs = append(mw.processingReqs, rqC)

	read := memprotocol.ReadReq{
		MsgMeta:        messaging.MsgMeta{ID: timing.GetIDGenerator().Generate()},
		Address:        64,
		AccessByteSize: 64,
	}
	mw.pendingReqs = append(mw.pendingReqs, read)
	rqC.appendSubordinateID(read.Meta().ID)

	toMem.Deliver(memprotocol.DataReadyRsp{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			RspTo: read.ID,
		},
		Data: make([]byte, 64),
	})
	mw.parseFromMem()

	if !rec.has(tracing.MilestoneKindData, "ToMem") {
		t.Fatalf("expected a data milestone on memcopy completion, got %+v",
			rec.milestones)
	}
}
