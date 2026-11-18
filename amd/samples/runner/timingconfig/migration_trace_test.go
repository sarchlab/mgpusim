package timingconfig

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	fault "github.com/sarchlab/akita/v5/mem/vm/pagefaultprotocol"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type pageTraceObserver struct {
	events  []tasks.Task
	marks   []tasks.Milestone
	service uint64
}

func (o *pageTraceObserver) TaskStarted(task tasks.Task) {
	if task.Kind == "Page service" {
		o.service = task.ID
	}
	if task.Kind == "Page service" || task.Kind == tasks.KindReqIn || task.Kind == tasks.KindReqOut {
		o.events = append(o.events, task)
	}
}
func (o *pageTraceObserver) TaskEnded(task tasks.Task) {
	if task.Kind == "Page service" || task.Kind == tasks.KindReqIn || task.Kind == tasks.KindReqOut {
		task.Detail = nil // restored open tasks omit borrowed details
		o.events = append(o.events, task)
	}
}
func (*pageTraceObserver) TagAdded(tasks.Tag) {}
func (o *pageTraceObserver) MilestoneAdded(mark tasks.Milestone) {
	if mark.TaskID == o.service {
		o.marks = append(o.marks, mark)
	}
}

func TestPageServiceTraceAndCheckpoint(t *testing.T) {
	p := buildMigrationPlatformWithSimulation(t, "r9nano", defaultPlatform, sim.MakeBuilder().WithTaskTracking())
	r := p.driver.Resources.Runtime
	ctx := r.Init()
	pid := r.QueuePID(r.CreateCommandQueue(ctx))
	addr := r.AllocateUnifiedMemory(ctx, 4096)
	r.SetAccessedBy(ctx, addr, 1, 1) // owner retains explicit permission after movement
	p.driver.TickLater()
	p.run(t)
	peer := p.walkers["GPU[2].GMMU"]
	mover := p.movers["GPU[2].DataMover"]
	o := &pageTraceObserver{}
	p.sim.TaskTracker().Subscribe(o, p.driver, p.mmu, peer, p.cps["GPU[2].CommandProcessor"], mover)
	peer.Ports.FaultIn.Deliver(messaging.Msg{
		ID: p.sim.NewID(), Src: p.client.AsRemote(), Dst: peer.Ports.FaultIn.AsRemote(),
		Payload: fault.Req{PID: pid, VAddr: uint64(addr), DeviceID: 2}})
	p.waitForPageCopy(t)
	path := filepath.Join(t.TempDir(), "copy.tar.gz")
	if err := p.sim.SaveCheckpoint(path); err != nil {
		t.Fatal(err)
	}
	preEvents, preMarks := len(o.events), len(o.marks)
	p.run(t)
	checkServiceTrace(t, o)
	events, marks := slices.Clone(o.events[preEvents:]), slices.Clone(o.marks[preMarks:])
	if err := p.sim.LoadCheckpoint(path); err != nil {
		t.Fatal(err)
	}
	o.events, o.marks = o.events[:preEvents], o.marks[:preMarks]
	p.run(t)
	if !reflect.DeepEqual(events, o.events[preEvents:]) || !slices.Equal(marks, o.marks[preMarks:]) {
		t.Fatal("rewound migration produced different task identities or wait boundaries")
	}
	rsp, ok := p.client.RetrieveIncoming()
	if !ok || rsp.Payload.(fault.Rsp).Result != fault.Blocked {
		t.Fatalf("triggering walk did not retire as blocked: %+v", rsp)
	}
	owner := p.walkers["GPU[1].GMMU"]
	io := &pteTraffic{}
	p.mmu.Ports.Memory.AcceptHook(io)
	got := p.translate(t, owner.Ports.FaultIn, pid, uint64(addr), 1)
	if got.Result != fault.Resolved || got.Mapping.DeviceID != 2 || io.reads != 0 {
		t.Fatalf("old owner did not use its explicit mapping to the new frame: %+v, %d IOMMU reads", got, io.reads)
	}
}

func (p migrationPlatform) waitForPageCopy(t *testing.T) {
	t.Helper()
	mover := p.movers["GPU[2].DataMover"]
	engine := p.sim.Engine().(*timing.SerialEngine)
	deadline := engine.CurrentTime() + 10000000
	for edge := engine.CurrentTime() + 1000; len(mover.State.Transactions) == 0; edge += 1000 {
		if edge > deadline {
			t.Fatal("data mover did not begin")
		}
		if err := engine.RunUntil(edge); err != nil {
			t.Fatal(err)
		}
	}
}

func checkServiceTrace(t *testing.T, o *pageTraceObserver) {
	t.Helper()
	var waits []string
	for _, mark := range o.marks {
		waits = append(waits, mark.What)
	}
	if !slices.Equal(waits, []string{"address translators held", "source and destination frames prepared",
		"page copy acknowledged", "page tables committed", "TLB shootdown acknowledged", "last release acknowledged"}) {
		t.Fatalf("page service wait boundaries: %v", waits)
	}
	byID := make(map[uint64]tasks.Task)
	for _, task := range o.events {
		byID[task.ID] = task
	}
	root := byID[o.service]
	if root.EndTime != o.marks[len(o.marks)-1].Time || byID[root.ParentID].Location != "Driver.req_in" {
		t.Fatalf("service %+v has wrong fault parent or end", root)
	}
	var copyChain, tableChain bool
	for _, task := range byID {
		parent := byID[task.ParentID]
		if task.Location == "GPU[2].DataMover.req_in" {
			job := byID[parent.ParentID]
			driver := byID[job.ParentID]
			copyChain = task.EndTime > task.StartTime && parent.Location == "GPU[2].CommandProcessor.req_out" &&
				job.Location == "GPU[2].CommandProcessor.req_in" && driver.What == "PageCopyReq" &&
				driver.ParentID == root.ID && task.MsgID == parent.MsgID && driver.MsgID == job.MsgID
		}
		if task.Location == "IOMMU.req_in" && parent.Location == "Driver.req_out" && parent.ParentID == root.ID {
			tableChain = task.EndTime > task.StartTime && task.MsgID == parent.MsgID
		}
	}
	if !copyChain || !tableChain {
		t.Fatalf("missing correlated DMA (%t) or page-table update (%t)", copyChain, tableChain)
	}
}
