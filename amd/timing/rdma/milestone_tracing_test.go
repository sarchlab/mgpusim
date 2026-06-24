package rdma

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type milestoneRecorder struct {
	tracing.NopTracer

	milestones []tracing.Milestone
}

func (r *milestoneRecorder) AddMilestone(m tracing.Milestone) {
	r.milestones = append(r.milestones, m)
}

// When a forwarded request's response returns, the RDMA engine must record a
// "data" milestone on the req_in task, attributing the time spent waiting on
// the round trip to the other side.
func TestRDMARecordsDataMilestoneOnResponse(t *testing.T) {
	engine := timing.NewSerialEngine()
	localCache := messaging.RemotePort("LocalCache")
	remoteGPU := messaging.RemotePort("RemoteGPU")

	rdmaEngine := MakeBuilder().
		WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
		WithResources(Resources{
			LocalModules:           &mem.SinglePortMapper{Port: localCache},
			RemoteRDMAAddressTable: &mem.SinglePortMapper{Port: remoteGPU},
		}).
		Build("RDMAEngine")

	makePort := func(name string) messaging.Port {
		port := messaging.NewPort(
			rdmaEngine, 4, 4, rdmaEngine.Name()+"."+name)
		rdmaEngine.AssignPort(name, port)
		(&noopConn{}).PlugIn(port)
		return port
	}
	makePort("RDMARequestInside")
	requestOutside := makePort("RDMARequestOutside")
	makePort("RDMADataInside")
	makePort("RDMADataOutside")
	makePort("Ctrl")

	rec := &milestoneRecorder{}
	tracing.CollectTrace(rdmaEngine, rec)

	var mw *forwardMiddleware
	for _, m := range rdmaEngine.Middlewares() {
		if fm, ok := m.(*forwardMiddleware); ok {
			mw = fm
		}
	}
	if mw == nil {
		t.Fatal("forwardMiddleware not found")
	}

	forwardedID := timing.GetIDGenerator().Generate()
	recvTaskID := timing.GetIDGenerator().Generate()
	rdmaEngine.State.TransactionsFromInside = append(
		rdmaEngine.State.TransactionsFromInside,
		transaction{
			OriginalReqID:  timing.GetIDGenerator().Generate(),
			OriginalSrc:    localCache,
			ForwardedReqID: forwardedID,
			RecvTaskID:     recvTaskID,
		})

	requestOutside.Deliver(memprotocol.DataReadyRsp{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			Src:   remoteGPU,
			Dst:   requestOutside.AsRemote(),
			RspTo: forwardedID,
		},
		Data: []byte{1, 2, 3, 4},
	})

	mw.Tick()

	found := false
	for _, ms := range rec.milestones {
		if ms.TaskID == recvTaskID &&
			ms.Kind == tracing.MilestoneKindData &&
			ms.What == "remote" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a data milestone on the req_in, got %+v",
			rec.milestones)
	}
}
