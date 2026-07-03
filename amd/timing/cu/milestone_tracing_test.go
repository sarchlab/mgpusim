package cu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

type cuMilestoneRecorder struct {
	tracing.NopTracer

	milestones []tracing.Milestone
}

func (r *cuMilestoneRecorder) AddMilestone(m tracing.Milestone) {
	r.milestones = append(r.milestones, m)
}

func (r *cuMilestoneRecorder) has(
	taskID uint64, kind tracing.MilestoneKind, what string,
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
	tracing.CollectTrace(cu.comp, rec)
	s := NewScheduler(cu, nil, nil)

	wf := wavefront.NewWavefront(kernels.NewWavefront())
	inst := wavefront.NewInst(insts.NewInst())
	wf.SetDynamicInst(inst)
	// Outstanding counts (0) already satisfy the requested counts (0), so the
	// wait resolves immediately.

	madeProgress, completed := s.evalSWaitCnt(wf)

	if !madeProgress || !completed {
		t.Fatalf("expected S_WAITCNT to retire, got progress=%v completed=%v",
			madeProgress, completed)
	}
	if !rec.has(inst.ID, tracing.MilestoneKindData, "s_waitcnt") {
		t.Fatalf("expected a data milestone for s_waitcnt, got %+v",
			rec.milestones)
	}
}

// The s_endpgm milestone must be emitted exactly once. If S_ENDPGM is the last
// wavefront in the work-group and the ACE port is full, the wavefront stays
// pending and re-enters evalSEndPgm next tick; the milestone must not be
// emitted on that retry, only when the instruction actually retires.
func TestSEndPgmMilestoneEmittedOnceNotOnAceRetry(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	cu.WfPools = []*WavefrontPool{NewWavefrontPool(10)}
	ace := newFakePort("CU.ToACE")
	cu.ToACE = ace
	rec := &cuMilestoneRecorder{}
	tracing.CollectTrace(cu.comp, rec)
	s := NewScheduler(cu, nil, nil)

	wg := new(wavefront.WorkGroup)
	wg.MapReq = protocol.MapWGReq{
		MsgMeta: messaging.MsgMeta{ID: timing.GetIDGenerator().Generate()},
	}
	wf := wavefront.NewWavefront(kernels.NewWavefront())
	wf.CodeObject = &insts.KernelCodeObject{
		KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{},
	}
	wf.SetDynamicInst(wavefront.NewInst(insts.NewInst()))
	wf.WG = wg
	wf.State = wavefront.WfRunning
	wg.Wfs = []*wavefront.Wavefront{wf}
	cu.WfPools[0].wfs = append(cu.WfPools[0].wfs, wf)
	taskID := wf.DynamicInst().ID

	// ACE port full: cannot send the WGCompletion, so the wavefront stays
	// pending. The drain milestone must not be emitted on this retry path.
	ace.full = true
	if _, completed := s.evalSEndPgm(wf); completed {
		t.Fatal("S_ENDPGM should not complete while the ACE port is full")
	}
	if rec.has(taskID, tracing.MilestoneKindData, "s_endpgm") {
		t.Fatal("s_endpgm milestone must not be emitted on the ACE-full retry")
	}

	// ACE port free: the wavefront retires and the milestone is emitted once.
	ace.full = false
	s.evalSEndPgm(wf)

	count := 0
	for _, m := range rec.milestones {
		if m.TaskID == taskID && m.Kind == tracing.MilestoneKindData &&
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
	tracing.NopTracer

	instID uint64
	events []string
}

func (r *orderedEndRecorder) AddMilestone(m tracing.Milestone) {
	if m.TaskID == r.instID && m.What == "s_endpgm" {
		r.events = append(r.events, "milestone")
	}
}

func (r *orderedEndRecorder) EndTask(e tracing.TaskEnd) {
	if e.ID == r.instID {
		r.events = append(r.events, "endTask")
	}
}

// When S_ENDPGM retires while other wavefronts are still executing, the branch
// closes the inst task via logInstTask. The s_endpgm milestone must be emitted
// before that, so it attributes the instruction's lifetime rather than landing
// on an already-ended task.
func TestSEndPgmMilestoneEmittedBeforeInstTaskEnds(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	cu.WfPools = []*WavefrontPool{NewWavefrontPool(10)}
	s := NewScheduler(cu, nil, nil)

	wg := new(wavefront.WorkGroup)
	co := &insts.KernelCodeObject{
		KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{},
	}

	wf := wavefront.NewWavefront(kernels.NewWavefront())
	wf.CodeObject = co
	wf.SetDynamicInst(wavefront.NewInst(insts.NewInst()))
	wf.WG = wg
	wf.State = wavefront.WfRunning

	other := wavefront.NewWavefront(kernels.NewWavefront())
	other.CodeObject = co
	other.WG = wg
	other.State = wavefront.WfRunning // still executing -> the "executing" branch
	wg.Wfs = []*wavefront.Wavefront{wf, other}

	rec := &orderedEndRecorder{instID: wf.DynamicInst().ID}
	tracing.CollectTrace(cu.comp, rec)

	if _, completed := s.evalSEndPgm(wf); !completed {
		t.Fatal("S_ENDPGM should retire on the executing branch")
	}

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
	tracing.CollectTrace(cu.comp, rec)

	wf := wavefront.NewWavefront(kernels.NewWavefront())
	inst := wavefront.NewInst(insts.NewInst())
	inst.ExeUnit = insts.ExeUnitVMem
	wf.SetDynamicInst(inst)

	read := memprotocol.ReadReq{
		MsgMeta: messaging.MsgMeta{ID: timing.GetIDGenerator().Generate()},
	}
	cu.InFlightVectorMemAccess = append(cu.InFlightVectorMemAccess,
		VectorMemAccessInfo{Read: &read, Inst: inst, Wavefront: wf})

	cu.handleVectorDataLoadReturn(memprotocol.DataReadyRsp{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			RspTo: read.ID,
		},
	})

	if !rec.has(inst.ID, tracing.MilestoneKindData, "vmem") {
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
	cu.comp.State.InstMem = "InstMem"
	cu.ToInstMem = newFakePort("CU.ToInstMem")
	rec := &cuMilestoneRecorder{}
	tracing.CollectTrace(cu.comp, rec)

	wf := wavefront.NewWavefront(kernels.NewWavefront())
	wf.SetPC(0x100)
	wf.InstBufferStartPC = 0x100
	wf.InstBuffer = make([]byte, 64)

	fetchArbiter := newMockWfArbitor()
	fetchArbiter.wfsToReturn = append(fetchArbiter.wfsToReturn,
		[]*wavefront.Wavefront{wf})
	s := NewScheduler(cu, fetchArbiter, nil)

	s.DoFetch()

	if rec.has(wf.UID, tracing.MilestoneKindHardwareResource,
		"CU.InstFetcher") {
		t.Fatalf("fetch-ahead should not be marked as a blocker, got %+v",
			rec.milestones)
	}
}

// If instruction bytes were already available before a prefetch response
// arrived, that response did not unblock the wavefront. Do not stamp a
// data/inst_fetch milestone over an execution-unit stall.
func TestFetchAheadReturnDoesNotRecordDataMilestone(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &cuMilestoneRecorder{}
	tracing.CollectTrace(cu.comp, rec)

	wf := wavefront.NewWavefront(kernels.NewWavefront())
	wf.SetPC(0x100)
	wf.InstBufferStartPC = 0x100
	wf.InstBuffer = make([]byte, 64)

	req := memprotocol.ReadReq{
		MsgMeta: messaging.MsgMeta{
			ID: timing.GetIDGenerator().Generate(),
		},
		Address:        0x140,
		AccessByteSize: 64,
	}
	cu.InFlightInstFetch = append(cu.InFlightInstFetch, &InstFetchReqInfo{
		Wavefront:   wf,
		Req:         req,
		Address:     0x140,
		FetchTaskID: timing.GetIDGenerator().Generate(),
	})

	cu.handleFetchReturn(memprotocol.DataReadyRsp{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			RspTo: req.ID,
		},
		Data: make([]byte, 64),
	})

	if rec.has(wf.UID, tracing.MilestoneKindData, "inst_fetch") {
		t.Fatalf("fetch-ahead return should not be marked as data wait, got %+v",
			rec.milestones)
	}
}

func TestBlockingFetchReturnRecordsDataMilestone(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &cuMilestoneRecorder{}
	tracing.CollectTrace(cu.comp, rec)

	wf := wavefront.NewWavefront(kernels.NewWavefront())
	wf.SetPC(0x100)
	wf.InstBufferStartPC = 0x100

	req := memprotocol.ReadReq{
		MsgMeta: messaging.MsgMeta{
			ID: timing.GetIDGenerator().Generate(),
		},
		Address:        0x100,
		AccessByteSize: 64,
	}
	cu.InFlightInstFetch = append(cu.InFlightInstFetch, &InstFetchReqInfo{
		Wavefront:   wf,
		Req:         req,
		Address:     0x100,
		FetchTaskID: timing.GetIDGenerator().Generate(),
	})

	cu.handleFetchReturn(memprotocol.DataReadyRsp{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			RspTo: req.ID,
		},
		Data: make([]byte, 64),
	})

	if !rec.has(wf.UID, tracing.MilestoneKindData, "inst_fetch") {
		t.Fatalf("blocking fetch return should be marked as data wait, got %+v",
			rec.milestones)
	}
}

type decodeTraceUnit struct {
	canAccept bool
	accepted  []*wavefront.Wavefront
}

func (u *decodeTraceUnit) CanAcceptWave() bool {
	return u.canAccept
}

func (u *decodeTraceUnit) AcceptWave(wf *wavefront.Wavefront) {
	u.accepted = append(u.accepted, wf)
}

func (u *decodeTraceUnit) Run() bool {
	return false
}

func (u *decodeTraceUnit) IsIdle() bool {
	return len(u.accepted) == 0
}

func (u *decodeTraceUnit) Flush() {}

// Decode is the first pipeline interval inside an instruction task. Record it
// explicitly so the instruction timeline does not have an unexplained prefix
// before the execution unit's own subtasks begin.
func TestDecodeRecordsWorkMilestone(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &cuMilestoneRecorder{}
	tracing.CollectTrace(cu.comp, rec)

	du := NewDecodeUnit(cu)
	exec := &decodeTraceUnit{canAccept: true}
	du.AddExecutionUnit(exec)

	wf := wavefront.NewWavefront(kernels.NewWavefront())
	inst := wavefront.NewInst(insts.NewInst())
	inst.ExeUnit = insts.ExeUnitScalar
	wf.SetDynamicInst(inst)

	du.AcceptWave(wf)
	du.Run()

	if !rec.has(inst.ID, tracing.MilestoneKindWork, "decode") {
		t.Fatalf("expected a work milestone for decode, got %+v",
			rec.milestones)
	}
	if _, ok := du.decodeTaskIDs[inst.ID]; ok {
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
	tracing.CollectTrace(cu.comp, rec)

	cu.ToScalarMem = newFakePort("CU.ToScalarMem")
	cu.comp.State.ScalarMem = "ScalarMem"
	su := NewScalarUnit(cu, nil)
	su.log2CachelineSize = 6

	wf := wavefront.NewWavefront(kernels.NewWavefront())
	regAccessor := newMockRegFileAccessor()
	wf.RegAccessor = regAccessor

	inst := wavefront.NewInst(insts.NewInst())
	inst.FormatType = insts.SMEM
	inst.Opcode = 0
	inst.Data = insts.NewSRegOperand(0, 0, 1)
	inst.Base = insts.NewSRegOperand(2, 2, 2)
	inst.Offset = insts.NewIntOperand(0, 0)
	wf.SetDynamicInst(inst)

	baseReg := insts.SReg(2)
	regAccessor.setRegValue(baseReg, 2, 0, wf.SRegOffset,
		insts.Uint64ToBytes(0x1000)[:8])

	su.AcceptWave(wf)
	su.Run() // read -> exec
	su.Run() // exec -> readBuf/in-flight scalar access

	if !rec.has(inst.ID, tracing.MilestoneKindWork, "smem_issue") {
		t.Fatalf("expected a work milestone for SMEM issue, got %+v",
			rec.milestones)
	}
	if _, ok := su.issueTaskIDs[inst.ID]; ok {
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
	tracing.CollectTrace(cu.comp, rec)

	cu.ToVectorMem = newFakePort("CU.ToVectorMem")
	vmu := NewVectorMemoryUnit(cu, nil)
	vmu.postTransactionPipelineBuffer =
		queueing.NewBuffer[VectorMemAccessInfo]("CU.PostTransBuf", 8)

	inst := wavefront.NewInst(nil)
	vmu.startIssueSubtask(inst) // as the in-flight admission would

	read := &memprotocol.ReadReq{
		MsgMeta: messaging.MsgMeta{
			ID:  timing.GetIDGenerator().Generate(),
			Src: cu.ToVectorMem.AsRemote(),
			Dst: "VectorMem",
		},
	}
	vmu.numTransactionInFlight = 1
	vmu.postTransactionPipelineBuffer.PushTyped(
		VectorMemAccessInfo{Read: read, Inst: inst})

	vmu.sendRequest()

	if !rec.has(inst.ID, tracing.MilestoneKindWork, "coalesce") {
		t.Fatalf("expected a work milestone at transaction send, got %+v",
			rec.milestones)
	}
	if _, ok := vmu.issueTaskIDs[inst.ID]; ok {
		t.Fatal("the issue subtask should be closed after the first send")
	}
}

func countCoalesceWork(rec *cuMilestoneRecorder, instID uint64) int {
	count := 0
	for _, m := range rec.milestones {
		if m.TaskID == instID && m.Kind == tracing.MilestoneKindWork &&
			m.What == "coalesce" {
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
	tracing.CollectTrace(cu.comp, rec)

	port := newFakePort("CU.ToVectorMem")
	cu.ToVectorMem = port
	vmu := NewVectorMemoryUnit(cu, nil)
	vmu.postTransactionPipelineBuffer =
		queueing.NewBuffer[VectorMemAccessInfo]("CU.PostTransBuf", 8)

	inst := wavefront.NewInst(nil)
	vmu.startIssueSubtask(inst)
	read := &memprotocol.ReadReq{
		MsgMeta: messaging.MsgMeta{
			ID:  timing.GetIDGenerator().Generate(),
			Src: cu.ToVectorMem.AsRemote(),
			Dst: "VectorMem",
		},
	}
	vmu.numTransactionInFlight = 1
	vmu.postTransactionPipelineBuffer.PushTyped(
		VectorMemAccessInfo{Read: read, Inst: inst})

	// Port full: the transaction is ready but cannot be sent. The work
	// milestone fires now; the transaction stays unsent.
	port.full = true
	vmu.sendRequest()

	if got := countCoalesceWork(rec, inst.ID); got != 1 {
		t.Fatalf("coalesce work must be emitted when ready even if the port is "+
			"full; got %d", got)
	}
	if len(port.sent) != 0 {
		t.Fatal("the transaction must not be sent while the port is full")
	}

	// Port frees: the transaction is sent, but no second work milestone.
	port.full = false
	vmu.sendRequest()

	if got := countCoalesceWork(rec, inst.ID); got != 1 {
		t.Fatalf("coalesce work must not be re-emitted on the actual send; "+
			"got %d", got)
	}
	if len(port.sent) != 1 {
		t.Fatal("the transaction should be sent once the port frees")
	}
}

// A vector-memory instruction flushed after admission but before its first send
// must keep its coalesce work milestone. Its parent inst task survives the
// flush (for the shadow response) and the resend bypasses the unit, so Flush
// itself must emit the milestone and end the subtask.
func TestVectorMemFlushPreservesCoalesceWork(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &cuMilestoneRecorder{}
	tracing.CollectTrace(cu.comp, rec)

	vmu := NewVectorMemoryUnit(cu, nil)
	vmu.instructionPipeline = queueing.NewPipeline[vectorMemInst](1, 6)
	vmu.postInstructionPipelineBuffer =
		queueing.NewBuffer[vectorMemInst]("CU.PostInstBuf", 16)
	vmu.transactionPipeline = queueing.NewPipeline[VectorMemAccessInfo](2, 10)
	vmu.postTransactionPipelineBuffer =
		queueing.NewBuffer[VectorMemAccessInfo]("CU.PostTransBuf", 8)

	inst := wavefront.NewInst(nil)
	vmu.startIssueSubtask(inst) // admitted; issue work not yet done

	vmu.Flush()

	if got := countCoalesceWork(rec, inst.ID); got != 1 {
		t.Fatalf("flush must preserve the coalesce work milestone for an "+
			"in-issue instruction; got %d", got)
	}
	if _, ok := vmu.issueTaskIDs[inst.ID]; ok {
		t.Fatal("the issue subtask should be cleared on flush")
	}
}

func countVMemData(rec *cuMilestoneRecorder, instID uint64) int {
	count := 0
	for _, m := range rec.milestones {
		if m.TaskID == instID && m.Kind == tracing.MilestoneKindData &&
			m.What == "vmem" {
			count++
		}
	}

	return count
}

// Coalesced responses can return out of order: the last request generated
// (CanWaitForCoalesce==false) may return before its siblings. The vmem data
// milestone must wait for the actual last response, gated on no remaining
// in-flight transactions for the instruction.
func TestVectorMemDataMilestoneWaitsForLastResponse(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &cuMilestoneRecorder{}
	tracing.CollectTrace(cu.comp, rec)

	inst := wavefront.NewInst(insts.NewInst())
	wf := wavefront.NewWavefront(kernels.NewWavefront())

	readSibling := &memprotocol.ReadReq{
		MsgMeta:            messaging.MsgMeta{ID: timing.GetIDGenerator().Generate()},
		CanWaitForCoalesce: true,
	}
	readLastGenerated := &memprotocol.ReadReq{
		MsgMeta: messaging.MsgMeta{ID: timing.GetIDGenerator().Generate()},
	}
	cu.InFlightVectorMemAccess = []VectorMemAccessInfo{
		{Read: readSibling, Inst: inst, Wavefront: wf},
		{Read: readLastGenerated, Inst: inst, Wavefront: wf},
	}

	// The last-generated transaction returns first (out of order); its sibling
	// is still in flight, so no data milestone yet.
	cu.handleVectorDataLoadReturn(memprotocol.DataReadyRsp{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			RspTo: readLastGenerated.ID,
		},
	})
	if countVMemData(rec, inst.ID) != 0 {
		t.Fatal("data milestone must not fire while a sibling is still in flight")
	}

	// The actual last response returns: the milestone fires exactly once.
	cu.handleVectorDataLoadReturn(memprotocol.DataReadyRsp{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			RspTo: readSibling.ID,
		},
	})
	if got := countVMemData(rec, inst.ID); got != 1 {
		t.Fatalf("expected one data milestone after the last response; got %d",
			got)
	}
}

// During a flush resend, coalesced siblings that have not been re-sent yet live
// in the shadow buffer. The data milestone must not fire while such a sibling
// is still parked there.
func TestVectorMemDataMilestoneWaitsForShadowSiblings(t *testing.T) {
	cu := newTestComputeUnit("CU", newFakeEngine())
	rec := &cuMilestoneRecorder{}
	tracing.CollectTrace(cu.comp, rec)

	inst := wavefront.NewInst(insts.NewInst())
	wf := wavefront.NewWavefront(kernels.NewWavefront())

	resent := &memprotocol.ReadReq{
		MsgMeta: messaging.MsgMeta{ID: timing.GetIDGenerator().Generate()},
	}
	cu.InFlightVectorMemAccess = []VectorMemAccessInfo{
		{Read: resent, Inst: inst, Wavefront: wf},
	}
	// A coalesced sibling still parked in the shadow buffer, not yet re-sent.
	shadowSibling := &memprotocol.ReadReq{
		MsgMeta: messaging.MsgMeta{ID: timing.GetIDGenerator().Generate()},
	}
	cu.shadowInFlightVectorMemAccess = []VectorMemAccessInfo{
		{Read: shadowSibling, Inst: inst, Wavefront: wf},
	}

	cu.handleVectorDataLoadReturn(memprotocol.DataReadyRsp{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			RspTo: resent.ID,
		},
	})

	if countVMemData(rec, inst.ID) != 0 {
		t.Fatal("data milestone must not fire while a sibling is parked in the " +
			"shadow buffer")
	}
}
