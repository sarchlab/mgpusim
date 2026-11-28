package cu

import (
	"encoding/binary"
	"testing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

type cuMilestoneRecorder struct {
	tasks.NopObserver

	milestones []tasks.Milestone
}

func (r *cuMilestoneRecorder) MilestoneAdded(m tasks.Milestone) {
	r.milestones = append(r.milestones, m)
}

func (r *cuMilestoneRecorder) has(
	taskID uint64, kind tasks.MilestoneKind, what string,
) bool {
	for _, m := range r.milestones {
		if m.TaskID == taskID && m.Kind == kind && m.What == what {
			return true
		}
	}

	return false
}

// When an S_WAITCNT's outstanding memory accesses have drained, the scheduler
// must record a "data" milestone on the instruction's task, attributing the
// time the wavefront spent waiting for memory.
func TestSWaitCntRecordsDataMilestone(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &cuMilestoneRecorder{}
	cu.comp.TaskTracker().Subscribe(rec, cu.comp)
	s := NewScheduler(cu, nil, nil)

	wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
	startTestWave(cu, wf)
	inst := testInst(testIDs.NewID(), insts.NewInst())
	instTask := startTestInst(cu, inst.ID)
	wf.setTestInst(inst)
	// Outstanding counts (0) already satisfy the requested counts (0), so the
	// wait resolves immediately.

	madeProgress, completed := s.evalSWaitCnt(wf)

	if !madeProgress || !completed {
		t.Fatalf("expected S_WAITCNT to retire, got progress=%v completed=%v",
			madeProgress, completed)
	}
	if !rec.has(instTask, tasks.MilestoneKindData, "s_waitcnt") {
		t.Fatalf("expected a data milestone for s_waitcnt, got %+v",
			rec.milestones)
	}
}

// S_ENDPGM retires once, independently of later completion-port retries. The
// drain milestone and instruction task must not be replayed while ACE is full.
func TestSEndPgmMilestoneEmittedOnceNotOnAceRetry(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	cu.WfPools = []*WavefrontPool{NewWavefrontPool(cu, 0, 10)}
	ace := newFakePort("CU.ToACE")
	cu.ToACE = ace
	rec := &cuMilestoneRecorder{}
	cu.comp.TaskTracker().Subscribe(rec, cu.comp)
	s := NewScheduler(cu, nil, nil)

	wg := newTestRawGroup(kernels.NewWorkGroup(testIDs.NewID()), messaging.Msg{})
	setTestRequest(wg, messaging.Msg{ID: testIDs.NewID(), Payload: protocol.MapWGReq{}})

	wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
	startTestWave(cu, wf)
	setTestCodeObject(wf.group(), &insts.KernelCodeObject{
		KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{},
	})
	wf.setTestInst(testInst(testIDs.NewID(), insts.NewInst()))
	setTestGroup(wf, wg)
	wf.execution().State = wavefront.WfRunning
	setTestWaves(wg, []*Wavefront{wf})
	testAddWave(cu.WfPools[0], wf)
	taskID := startTestInst(cu, wf.InstID())

	// Execution retires independently of ACE delivery. Completion retries must
	// retain the hardware slot and must not replay instruction milestones.
	cu.Scheduler = s
	ace.full = true
	if _, completed := s.evalSEndPgm(wf); !completed {
		t.Fatal("execution should finish before ACE delivery")
	}
	cu.completeInstruction(wf, wf.InstID())
	cu.retireGroups()
	cu.retireGroups()
	if len(ace.sent) != 0 || wf.execution().State != wavefront.WfCompleted {
		t.Fatal("full ACE released wave")
	}
	ace.full = false
	cu.retireGroups()
	if len(ace.sent) != 1 {
		t.Fatal("completion was not delivered exactly once")
	}

	count := 0
	for _, m := range rec.milestones {
		if m.TaskID == taskID && m.Kind == tasks.MilestoneKindData &&
			m.What == "s_endpgm" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one s_endpgm milestone, got %d", count)
	}
}

// orderedEndRecorder records, in order, the s_endpgm milestone and the end of a
// specific task, so a test can assert the milestone is emitted before the task
// is closed.
type orderedEndRecorder struct {
	tasks.NopObserver

	instID uint64
	events []string
}

func (r *orderedEndRecorder) MilestoneAdded(m tasks.Milestone) {
	if m.TaskID == r.instID && m.What == "s_endpgm" {
		r.events = append(r.events, "milestone")
	}
}

func (r *orderedEndRecorder) TaskEnded(e tasks.Task) {
	if e.ID == r.instID {
		r.events = append(r.events, "endTask")
	}
}

// When S_ENDPGM retires while other wavefronts are still executing, the branch
// closes the inst task via completeInstruction. The s_endpgm milestone must be emitted
// before that, so it attributes the instruction's lifetime rather than landing
// on an already-ended task.
func TestSEndPgmMilestoneEmittedBeforeInstTasks(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	cu.WfPools = []*WavefrontPool{NewWavefrontPool(cu, 0, 10)}
	s := NewScheduler(cu, nil, nil)

	wg := newTestRawGroup(kernels.NewWorkGroup(testIDs.NewID()), messaging.Msg{})
	co := &insts.KernelCodeObject{
		KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{},
	}

	wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
	startTestWave(cu, wf)
	setTestCodeObject(wf.group(), co)
	wf.setTestInst(testInst(testIDs.NewID(), insts.NewInst()))
	setTestGroup(wf, wg)
	wf.execution().State = wavefront.WfRunning

	other := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
	setTestCodeObject(other.group(), co)
	setTestGroup(other, wg)
	other.execution().State = wavefront.WfRunning // still executing -> the "executing" branch
	setTestWaves(wg, []*Wavefront{wf, other})

	rec := &orderedEndRecorder{instID: startTestInst(cu, wf.InstID())}
	cu.comp.TaskTracker().Subscribe(rec, cu.comp)

	if _, completed := s.evalSEndPgm(wf); !completed {
		t.Fatal("S_ENDPGM should retire on the executing branch")
	}

	cu.completeInstruction(wf, wf.InstID())

	if len(rec.events) != 2 ||
		rec.events[0] != "milestone" || rec.events[1] != "endTask" {
		t.Fatalf("s_endpgm milestone must be emitted before the inst task "+
			"ends; got %v", rec.events)
	}
}

// When a vector-memory load's last response returns, the CU must record a
// "data" milestone on the instruction's task so its lifetime is fully
// attributed — without it the bar shows an unexplained gap between the
// in-flight admission and the task end (the memory round trip).
func TestVectorMemDataReturnRecordsDataMilestone(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &cuMilestoneRecorder{}
	cu.comp.TaskTracker().Subscribe(rec, cu.comp)

	wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
	startTestWave(cu, wf)
	inst := testInst(testIDs.NewID(), insts.NewInst())
	instTask := startTestInst(cu, inst.ID)
	inst.ExeUnit = insts.ExeUnitVMem
	wf.setTestInst(inst)

	read := messaging.Msg{ID: testIDs.NewID(), Payload: memprotocol.ReadReq{}}

	cu.comp.State.Vector = append(cu.comp.State.Vector, testVectorState(cu,
		VectorMemAccessInfo{Read: read, InstID: inst.ID, Wavefront: wf}))

	cu.handleVectorDataLoadReturn(messaging.Msg{ID: testIDs.NewID(),
		RspTo: read.ID, Payload: memprotocol.DataReadyRsp{}},
	)

	if !rec.has(instTask, tasks.MilestoneKindData, "vmem") {
		t.Fatalf("expected a data milestone for the vmem return, got %+v",
			rec.milestones)
	}
}

// A fetch-ahead request can run while the wavefront already has instruction
// bytes for its current PC. That fetch is not the wavefront's blocker; if the
// ready instruction cannot issue yet, the later issue milestone should name the
// execution unit instead.
func TestFetchAheadDoesNotRecordFetcherMilestone(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	cu.comp.Spec.InstMem = "InstMem"
	cu.ToInstMem = newFakePort("CU.ToInstMem")
	rec := &cuMilestoneRecorder{}
	cu.comp.TaskTracker().Subscribe(rec, cu.comp)

	wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
	startTestWave(cu, wf)
	wf.SetPC(0x100)
	wf.execution().InstBufferStartPC = 0x100
	wf.execution().InstBuffer = make([]byte, 64)

	fetchArbiter := newMockWfArbitor()
	fetchArbiter.wfsToReturn = append(fetchArbiter.wfsToReturn,
		[]*Wavefront{wf})
	s := NewScheduler(cu, fetchArbiter, nil)

	s.DoFetch()

	if rec.has(cu.comp.State.WFTasks[wf.UID], tasks.MilestoneKindHardwareResource,
		"CU.InstFetcher") {
		t.Fatalf("fetch-ahead should not be marked as a blocker, got %+v",
			rec.milestones)
	}
}

// While a stalled wavefront requests several blocks, only the request for the
// block it is waiting for is a wait on the fetcher. The requests issued behind
// it must not move that milestone: the time until the first block returns is
// spent waiting for its data.
func TestOnlyTheDemandFetchRecordsFetcherMilestone(t *testing.T) {
	engine := newFakeEngine()
	cu := newTestComputeUnit("CU", engine)
	cu.comp.Spec.InstMem = "InstMem"
	cu.comp.Spec.InstBlockByteSize = 32
	cu.comp.Spec.InstBufByteSize = 128
	period := cu.comp.Spec.Freq.Period()
	toInstMem := newFakePort("CU.ToInstMem")
	cu.ToInstMem = toInstMem
	rec := &cuMilestoneRecorder{}
	cu.comp.TaskTracker().Subscribe(rec, cu.comp)

	wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
	startTestWave(cu, wf)
	wf.SetPC(0x100)
	wf.execution().InstBufferStartPC = 0x100
	cu.WfPools = []*WavefrontPool{NewWavefrontPool(cu, 0, 10)}
	cu.WfPools[0].AddWf(wf)
	task := cu.comp.State.WFTasks[wf.UID]

	fetchArbiter := newMockWfArbitor()
	s := NewScheduler(cu, fetchArbiter, nil)
	count := func(kind tasks.MilestoneKind, what string) int {
		n := 0
		for _, m := range rec.milestones {
			if m.TaskID == task && m.Kind == kind && m.What == what {
				n++
			}
		}
		return n
	}

	for tick := 1; tick <= 3; tick++ {
		engine.now = timing.VTimeInPicoSec(tick) * period
		s.DoFetch()

		if got := count(tasks.MilestoneKindHardwareResource, "CU.InstFetcher"); got != 1 {
			t.Fatalf("after %d fetches the fetcher milestone was recorded %d times, got %+v",
				tick, got, rec.milestones)
		}
	}
	if len(toInstMem.sent) != 3 || len(cu.comp.State.InstFetch) != 3 {
		t.Fatalf("issued %d requests, tracking %d; want 3", len(toInstMem.sent), len(cu.comp.State.InstFetch))
	}

	engine.now = 4 * period
	cu.handleFetchReturn(messaging.Msg{ID: testIDs.NewID(),
		RspTo: toInstMem.sent[0].ID, Payload: memprotocol.DataReadyRsp{Data: make([]byte, 32)}})
	if got := count(tasks.MilestoneKindData, "inst_fetch"); got != 1 {
		t.Fatalf("the first block's return recorded the data milestone %d times, got %+v",
			got, rec.milestones)
	}
}

// A line read ahead into the instruction cache neither starts nor ends a wait
// of the wavefront.
func TestPrefetchRecordsNoWaveMilestone(t *testing.T) {
	engine := newFakeEngine()
	cu := newTestComputeUnit("CU", engine)
	cu.comp.Spec.InstMem = "InstMem"
	cu.comp.Spec.InstBlockByteSize = 32
	cu.comp.Spec.InstBufByteSize = 64
	cu.comp.Spec.InstPrefetchLines = 1
	toInstMem := newFakePort("CU.ToInstMem")
	cu.ToInstMem = toInstMem
	cu.ToInstPrefetch = toInstMem
	rec := &cuMilestoneRecorder{}
	cu.comp.TaskTracker().Subscribe(rec, cu.comp)

	wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
	startTestWave(cu, wf)
	recordCodeEnd(wf, 0x100, 0x1000)
	wf.SetPC(0x100)
	wf.execution().InstBufferStartPC = 0x100
	wf.execution().NextFetchPC = 0x140
	wf.execution().IssuedEnd = 0x100
	cu.WfPools = []*WavefrontPool{NewWavefrontPool(cu, 0, 10)}
	cu.WfPools[0].AddWf(wf)
	s := NewScheduler(cu, newMockWfArbitor(), nil)

	s.DoFetch()

	if len(toInstMem.sent) != 1 || !cu.comp.State.InstFetch[0].Prefetch {
		t.Fatalf("expected one prefetch, sent %d requests", len(toInstMem.sent))
	}
	if req := toInstMem.sent[0].Payload.(memprotocol.ReadReq); req.Address != 0x180 || req.AccessByteSize != 64 {
		t.Fatalf("prefetched %+v, want the 64-byte line at 0x180", req)
	}
	cu.handleFetchReturn(messaging.Msg{ID: testIDs.NewID(),
		RspTo: toInstMem.sent[0].ID, Payload: memprotocol.DataReadyRsp{Data: make([]byte, 64)}})
	if len(rec.milestones) != 0 {
		t.Fatalf("a prefetch recorded %+v", rec.milestones)
	}
	if len(wf.execution().InstBuffer) != 0 || wf.execution().IsFetching {
		t.Fatal("prefetched data entered the instruction buffer")
	}
}

// If instruction bytes were already available before a prefetch response
// arrived, that response did not unblock the wavefront. Do not stamp a
// data/inst_fetch milestone over an execution-unit stall.
func TestFetchAheadReturnDoesNotRecordDataMilestone(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &cuMilestoneRecorder{}
	cu.comp.TaskTracker().Subscribe(rec, cu.comp)

	wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
	startTestWave(cu, wf)
	wf.SetPC(0x100)
	wf.execution().InstBufferStartPC = 0x100
	wf.execution().InstBuffer = make([]byte, 64)

	req := messaging.Msg{ID: testIDs.NewID(), Payload: memprotocol.ReadReq{Address: 0x140,
		AccessByteSize: 64}}

	cu.comp.State.InstFetch = append(cu.comp.State.InstFetch, testFetchState(cu, &InstFetchReqInfo{
		Wavefront:   wf,
		Req:         req,
		Address:     0x140,
		FetchTaskID: testIDs.NewID(),
	}))

	cu.handleFetchReturn(messaging.Msg{ID: testIDs.NewID(),
		RspTo: req.ID, Payload: memprotocol.DataReadyRsp{Data: make([]byte, 64)}},
	)

	if rec.has(cu.comp.State.WFTasks[wf.UID], tasks.MilestoneKindData, "inst_fetch") {
		t.Fatalf("fetch-ahead return should not be marked as data wait, got %+v",
			rec.milestones)
	}
}

func TestBlockingFetchReturnRecordsDataMilestone(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &cuMilestoneRecorder{}
	cu.comp.TaskTracker().Subscribe(rec, cu.comp)

	wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
	startTestWave(cu, wf)
	wf.SetPC(0x100)
	wf.execution().InstBufferStartPC = 0x100

	req := messaging.Msg{ID: testIDs.NewID(), Payload: memprotocol.ReadReq{Address: 0x100,
		AccessByteSize: 64}}

	cu.comp.State.InstFetch = append(cu.comp.State.InstFetch, testFetchState(cu, &InstFetchReqInfo{
		Wavefront:   wf,
		Req:         req,
		Address:     0x100,
		FetchTaskID: testIDs.NewID(),
	}))

	cu.handleFetchReturn(messaging.Msg{ID: testIDs.NewID(),
		RspTo: req.ID, Payload: memprotocol.DataReadyRsp{Data: make([]byte, 64)}},
	)

	if !rec.has(cu.comp.State.WFTasks[wf.UID], tasks.MilestoneKindData, "inst_fetch") {
		t.Fatalf("blocking fetch return should be marked as data wait, got %+v",
			rec.milestones)
	}
}

// gfx942Decoder decodes the instructions MI300X runs.
func gfx942Decoder() *insts.Disassembler {
	decoder := insts.NewDisassembler()
	decoder.IsCDNA3 = true
	return decoder
}

// sNopBytes returns n bytes of s_nop instructions.
func sNopBytes(n int) []byte {
	var b []byte
	for range n / 4 {
		b = binary.LittleEndian.AppendUint32(b, encSNop[0])
	}
	return b
}

// An 8-byte instruction whose first half ends one block is not ready until the
// next block brings its second half: the return of that block ends a wait on
// instruction fetch, not one on an execution unit. Fetching whole lines, the
// first 4 bytes count as ready, as they always have, so only the first line's
// return ends a wait.
func TestStraddlingInstructionWaitsForItsSecondBlock(t *testing.T) {
	cases := []struct {
		name           string
		block          int // block size, or 0 to fetch whole lines
		wantMilestones int
	}{
		{"block fetch", 32, 2},
		{"line fetch", 0, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checkStraddlingFetchMilestones(t, c.block, c.wantMilestones)
		})
	}
}

func checkStraddlingFetchMilestones(t *testing.T, block, want int) {
	t.Helper()
	engine := newFakeEngine()
	cu := newTestComputeUnit("CU", engine)
	cu.Decoder = gfx942Decoder()
	cu.comp.Spec.InstBlockByteSize = block
	unit := cu.fetchUnitByteSize()
	period := cu.comp.Spec.Freq.Period()
	rec := &cuMilestoneRecorder{}
	cu.comp.TaskTracker().Subscribe(rec, cu.comp)

	wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
	startTestWave(cu, wf)
	wf.SetPC(0x100 + unit - 4) // v_fma_f32 in the last 4 bytes of the first unit
	cu.redirectFetch(wf)
	task := cu.comp.State.WFTasks[wf.UID]

	first := append(sNopBytes(int(unit)-4), binary.LittleEndian.AppendUint32(nil, encVFma[0])...)
	second := append(binary.LittleEndian.AppendUint32(nil, encVFma[1]), sNopBytes(int(unit)-4)...)
	fetch := func(address uint64, data []byte) messaging.Msg {
		req := messaging.Msg{ID: testIDs.NewID(), Payload: memprotocol.ReadReq{Address: address,
			AccessByteSize: unit}}
		cu.comp.State.InstFetch = append(cu.comp.State.InstFetch, testFetchState(cu, &InstFetchReqInfo{
			Wavefront: wf, Req: req, Address: address, Epoch: wf.execution().FetchEpoch,
			FetchTaskID: testIDs.NewID(),
		}))
		return messaging.Msg{ID: testIDs.NewID(), RspTo: req.ID, Payload: memprotocol.DataReadyRsp{Data: data}}
	}
	firstRsp, secondRsp := fetch(0x100, first), fetch(0x100+unit, second)
	fetchMilestones := func() []tasks.Milestone {
		var ms []tasks.Milestone
		for _, m := range rec.milestones {
			if m.TaskID == task && m.Kind == tasks.MilestoneKindData && m.What == "inst_fetch" {
				ms = append(ms, m)
			}
		}
		return ms
	}

	engine.now = 16 * period
	cu.handleFetchReturn(firstRsp)
	if cu.hasCompleteInstruction(wf) {
		t.Fatal("half of the 8-byte instruction counts as complete")
	}

	engine.now = 40 * period
	cu.handleFetchReturn(secondRsp)
	ms := fetchMilestones()
	if len(ms) != want || ms[0].Time != 16*period || ms[len(ms)-1].Time != period*timing.VTimeInPicoSec(16+24*(want-1)) {
		t.Fatalf("want %d inst_fetch milestones, from cycle 16; got %+v", want, ms)
	}
	if !cu.hasCompleteInstruction(wf) {
		t.Fatal("the 8-byte instruction is not complete after its second unit")
	}
}

// A line made obsolete by a taken branch ends no wait: the milestone belongs
// to the line the wavefront is waiting for.
func TestObsoleteFetchReturnDoesNotRecordDataMilestone(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	cu.comp.Spec.InstBlockByteSize = 32
	rec := &cuMilestoneRecorder{}
	cu.comp.TaskTracker().Subscribe(rec, cu.comp)

	wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
	startTestWave(cu, wf)
	wf.SetPC(0x100)
	cu.redirectFetch(wf)

	fetch := func(address, epoch uint64) messaging.Msg {
		req := messaging.Msg{ID: testIDs.NewID(), Payload: memprotocol.ReadReq{Address: address,
			AccessByteSize: 32}}
		cu.comp.State.InstFetch = append(cu.comp.State.InstFetch, testFetchState(cu, &InstFetchReqInfo{
			Wavefront:   wf,
			Req:         req,
			Address:     address,
			Epoch:       epoch,
			FetchTaskID: testIDs.NewID(),
		}))

		return messaging.Msg{ID: testIDs.NewID(),
			RspTo: req.ID, Payload: memprotocol.DataReadyRsp{Data: make([]byte, 32)}}
	}
	obsolete := fetch(0x100, wf.execution().FetchEpoch-1)
	target := fetch(0x100, wf.execution().FetchEpoch)
	task := cu.comp.State.WFTasks[wf.UID]

	cu.handleFetchReturn(obsolete)
	if rec.has(task, tasks.MilestoneKindData, "inst_fetch") {
		t.Fatalf("an obsolete line should not be marked as the awaited data, got %+v",
			rec.milestones)
	}
	if len(wf.execution().InstBuffer) != 0 {
		t.Fatal("an obsolete line entered the instruction buffer")
	}

	cu.handleFetchReturn(target)
	if !rec.has(task, tasks.MilestoneKindData, "inst_fetch") {
		t.Fatalf("the target line should be marked as the awaited data, got %+v",
			rec.milestones)
	}
}

type decodeTraceUnit struct {
	canAccept bool
	accepted  []*Wavefront
}

func (u *decodeTraceUnit) CanAcceptWave() bool {
	return u.canAccept
}

func (u *decodeTraceUnit) AcceptWave(wf *Wavefront) {
	u.accepted = append(u.accepted, wf)
}

func (u *decodeTraceUnit) Run() bool {
	return false
}

func (u *decodeTraceUnit) IsIdle() bool {
	return len(u.accepted) == 0
}

// Decode is the first pipeline interval inside an instruction task. Record it
// explicitly so the instruction timeline does not have an unexplained prefix
// before the execution unit's own subtasks begin.
func TestDecodeRecordsWorkMilestone(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &cuMilestoneRecorder{}
	cu.comp.TaskTracker().Subscribe(rec, cu.comp)

	du := NewDecodeUnit(cu)
	exec := &decodeTraceUnit{canAccept: true}
	du.AddExecutionUnit(exec)

	wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
	startTestWave(cu, wf)
	inst := testInst(testIDs.NewID(), insts.NewInst())
	instTask := startTestInst(cu, inst.ID)
	inst.ExeUnit = insts.ExeUnitScalar
	wf.setTestInst(inst)

	registerTestWave(cu, wf)
	du.AcceptWave(wf)
	du.Run()

	if !rec.has(instTask, tasks.MilestoneKindWork, "CU.decode") {
		t.Fatalf("expected a work milestone for decode, got %+v",
			rec.milestones)
	}
	if _, ok := du.state().Tasks[inst.ID]; ok {
		t.Fatal("the decode subtask should be closed after execution-unit admission")
	}
	if len(exec.accepted) != 1 {
		t.Fatalf("expected decode to forward the wavefront, got %d accepts",
			len(exec.accepted))
	}
}

// A scalar-memory instruction spends a few cycles in the scalar unit before its
// read request exists. Mark that interval as issue work so the later data
// milestone starts at the memory request, not at the instruction task start.
func TestScalarMemIssueRecordsWorkMilestone(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &cuMilestoneRecorder{}
	cu.comp.TaskTracker().Subscribe(rec, cu.comp)

	cu.ToScalarMem = newFakePort("CU.ToScalarMem")
	cu.comp.Spec.ScalarMem = "ScalarMem"
	su := NewScalarUnit(cu, nil)
	su.log2CachelineSize = 6

	wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
	startTestWave(cu, wf)
	regAccessor := newMockRegFileAccessor()
	setTestAccessor(wf, regAccessor)

	inst := testInst(testIDs.NewID(), insts.NewInst())
	instTask := startTestInst(cu, inst.ID)
	inst.FormatType = insts.SMEM
	inst.Opcode = 0
	inst.Data = insts.NewSRegOperand(0, 0, 1)
	inst.Base = insts.NewSRegOperand(2, 2, 2)
	inst.Offset = insts.NewIntOperand(0, 0)
	wf.setTestInst(inst)

	baseReg := insts.SReg(2)
	regAccessor.setRegValue(baseReg, 2, 0, wf.execution().SRegOffset,
		insts.Uint64ToBytes(0x1000)[:8])

	registerTestWave(cu, wf)
	su.AcceptWave(wf)
	su.Run() // read -> exec
	su.Run() // exec -> readBuf/in-flight scalar access

	if !rec.has(instTask, tasks.MilestoneKindWork, "CU.smem_issue") {
		t.Fatalf("expected a work milestone for SMEM issue, got %+v",
			rec.milestones)
	}
	if _, ok := su.state().Tasks[inst.ID]; ok {
		t.Fatal("the SMEM issue subtask should be closed after request admission")
	}
}

// Sending the first vector-memory transaction closes the issue subtask and
// records a "work" milestone (not a hardware_resource one): the coalescing /
// transaction-issue phase is the unit doing work, and it is backed by the
// pipeline subtask opened at admission. The data wait is attributed only from
// this point, once a request is actually outstanding.
func TestVectorMemSendRecordsCoalesceWorkMilestone(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &cuMilestoneRecorder{}
	cu.comp.TaskTracker().Subscribe(rec, cu.comp)

	cu.ToVectorMem = newFakePort("CU.ToVectorMem")
	vmu := NewVectorMemoryUnit(cu, nil)
	vmu.state().TransactionsOutput =
		queueing.MakeBuffer[uint64](8)

	inst := testInst(testIDs.NewID(), nil)
	instTask := startTestInst(cu, inst.ID)
	vmu.startIssueSubtask(inst.ID) // as the in-flight admission would

	read := messaging.Msg{ID: testIDs.NewID(),
		Src: cu.ToVectorMem.AsRemote(),
		Dst: "VectorMem", Payload: memprotocol.ReadReq{}}

	vmu.state().Transactions = 1
	cu.comp.State.Vector = append(cu.comp.State.Vector,
		testVectorState(cu, VectorMemAccessInfo{Read: read, InstID: inst.ID}))
	vmu.state().TransactionsOutput.Push(read.ID)

	vmu.sendRequest()

	if !rec.has(instTask, tasks.MilestoneKindWork, "CU.coalesce") {
		t.Fatalf("expected a work milestone at transaction send, got %+v",
			rec.milestones)
	}
	if _, ok := vmu.state().IssueTasks[inst.ID]; ok {
		t.Fatal("the issue subtask should be closed after the first send")
	}
}

func countCoalesceWork(rec *cuMilestoneRecorder, instID uint64) int {
	count := 0
	for _, m := range rec.milestones {
		if m.TaskID == instID && m.Kind == tasks.MilestoneKindWork &&
			m.What == "CU.coalesce" {
			count++
		}
	}

	return count
}

// Port backpressure must not be counted as coalescing work. When the vector
// memory port is full, the transaction is ready but cannot be sent; the
// coalesce work milestone must still be emitted now (the work is done) and must
// not be re-emitted when the port frees and the transaction is finally sent.
func TestVectorMemCoalesceWorkNotDelayedByPortBackpressure(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &cuMilestoneRecorder{}
	cu.comp.TaskTracker().Subscribe(rec, cu.comp)

	port := newFakePort("CU.ToVectorMem")
	cu.ToVectorMem = port
	vmu := NewVectorMemoryUnit(cu, nil)
	vmu.state().TransactionsOutput =
		queueing.MakeBuffer[uint64](8)

	inst := testInst(testIDs.NewID(), nil)
	instTask := startTestInst(cu, inst.ID)
	vmu.startIssueSubtask(inst.ID)
	read := messaging.Msg{ID: testIDs.NewID(),
		Src: cu.ToVectorMem.AsRemote(),
		Dst: "VectorMem", Payload: memprotocol.ReadReq{}}

	vmu.state().Transactions = 1
	cu.comp.State.Vector = append(cu.comp.State.Vector,
		testVectorState(cu, VectorMemAccessInfo{Read: read, InstID: inst.ID}))
	vmu.state().TransactionsOutput.Push(read.ID)

	// Port full: the transaction is ready but cannot be sent. The work
	// milestone fires now; the transaction stays unsent.
	port.full = true
	vmu.sendRequest()

	if got := countCoalesceWork(rec, instTask); got != 1 {
		t.Fatalf("coalesce work must be emitted when ready even if the port is "+
			"full; got %d", got)
	}
	if len(port.sent) != 0 {
		t.Fatal("the transaction must not be sent while the port is full")
	}

	// Port frees: the transaction is sent, but no second work milestone.
	port.full = false
	vmu.sendRequest()

	if got := countCoalesceWork(rec, instTask); got != 1 {
		t.Fatalf("coalesce work must not be re-emitted on the actual send; "+
			"got %d", got)
	}
	if len(port.sent) != 1 {
		t.Fatal("the transaction should be sent once the port frees")
	}
}

func countVMemData(rec *cuMilestoneRecorder, instID uint64) int {
	count := 0
	for _, m := range rec.milestones {
		if m.TaskID == instID && m.Kind == tasks.MilestoneKindData &&
			m.What == "vmem" {
			count++
		}
	}

	return count
}

// Coalesced responses can return out of order: the last request generated
// may return before its siblings. The vmem data
// milestone must wait for the actual last response, gated on no remaining
// in-flight transactions for the instruction.
func TestVectorMemDataMilestoneWaitsForLastResponse(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &cuMilestoneRecorder{}
	cu.comp.TaskTracker().Subscribe(rec, cu.comp)

	inst := testInst(testIDs.NewID(), insts.NewInst())
	instTask := startTestInst(cu, inst.ID)
	wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
	startTestWave(cu, wf)
	wf.setTestInst(inst)

	readSibling := messaging.Msg{ID: testIDs.NewID(), Payload: memprotocol.ReadReq{}}

	readLastGenerated := messaging.Msg{ID: testIDs.NewID(), Payload: memprotocol.ReadReq{}}

	cu.comp.State.Vector = testVectorStates(cu, []VectorMemAccessInfo{
		{Read: readSibling, InstID: inst.ID, Wavefront: wf},
		{Read: readLastGenerated, InstID: inst.ID, Wavefront: wf},
	})

	// The last-generated transaction returns first (out of order); its sibling
	// is still in flight, so no data milestone yet.
	cu.handleVectorDataLoadReturn(messaging.Msg{ID: testIDs.NewID(),
		RspTo: readLastGenerated.ID, Payload: memprotocol.DataReadyRsp{}},
	)
	if countVMemData(rec, instTask) != 0 {
		t.Fatal("data milestone must not fire while a sibling is still in flight")
	}

	// The actual last response returns: the milestone fires exactly once.
	cu.handleVectorDataLoadReturn(messaging.Msg{ID: testIDs.NewID(),
		RspTo: readSibling.ID, Payload: memprotocol.DataReadyRsp{}},
	)
	if got := countVMemData(rec, instTask); got != 1 {
		t.Fatalf("expected one data milestone after the last response; got %d",
			got)
	}
}

func startTestInst(c *ComputeUnit, inst uint64) uint64 {
	id := c.comp.TaskTracker().StartTask(c.comp, tasks.StartInfo{Kind: "inst", What: "test", Detail: inst})
	c.comp.State.InstTasks[inst] = id
	return id
}
func startTestWave(c *ComputeUnit, wf *Wavefront) {
	registerTestWave(c, wf)
	c.comp.State.WFTasks[wf.UID] = c.comp.TaskTracker().
		StartTask(c.comp, tasks.StartInfo{Kind: "wavefront", What: "test"})
}
