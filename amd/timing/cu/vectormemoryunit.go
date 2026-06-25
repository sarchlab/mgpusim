package cu

import (
	"log"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

type vectorMemInst struct {
	wavefront *wavefront.Wavefront
}

// A VectorMemoryUnit is the block in a compute unit that can performs vector
// memory operations.
type VectorMemoryUnit struct {
	cu *ComputeUnit

	coalescer coalescer

	numInstInFlight         uint64
	numTransactionInFlight  uint64
	maxInstructionsInFlight uint64

	maxCoalescingPenalty     int
	coalescingStallRemaining int

	instructionPipeline           queueing.Pipeline[vectorMemInst]
	postInstructionPipelineBuffer queueing.Buffer[vectorMemInst]
	transactionsWaiting           []VectorMemAccessInfo
	transactionPipeline           queueing.Pipeline[VectorMemAccessInfo]
	postTransactionPipelineBuffer queueing.Buffer[VectorMemAccessInfo]

	// issueTaskIDs maps an instruction's task ID to the "pipeline" subtask
	// that records its coalescing / transaction-issue work — opened when its
	// transactions are admitted and closed when the first one is sent.
	issueTaskIDs map[uint64]uint64

	isIdle bool
}

// NewVectorMemoryUnit creates a new Vector Memory Unit.
func NewVectorMemoryUnit(
	cu *ComputeUnit,
	coalescer coalescer,
) *VectorMemoryUnit {
	u := new(VectorMemoryUnit)
	u.cu = cu
	u.coalescer = coalescer
	u.issueTaskIDs = make(map[uint64]uint64)

	return u
}

// startIssueSubtask opens the "pipeline" subtask that spans an instruction's
// coalescing / transaction-issue work (from the in-flight admission to the
// first transaction send), parented to the instruction's task. It is the child
// subtask that backs the "work" milestone emitted at first send.
func (u *VectorMemoryUnit) startIssueSubtask(inst *wavefront.Inst) {
	taskID := timing.GetIDGenerator().Generate()
	tracing.StartTask(u.cu.comp, tracing.TaskStart{
		ID:       taskID,
		ParentID: inst.ID,
		Kind:     "pipeline",
		What:     "coalesce",
	})
	u.issueTaskIDs[inst.ID] = taskID
}

// endIssueSubtask closes an instruction's issue subtask (if still open) when
// its first transaction reaches the head of the send buffer (the issue work is
// done), and emits the "work" milestone the subtask backs. Later transactions
// of the same instruction — and port-full retries on the same one — find no
// open subtask and are no-ops, so the milestone lands once, at the moment the
// transaction is ready to send rather than when it actually leaves.
func (u *VectorMemoryUnit) endIssueSubtask(inst *wavefront.Inst) {
	taskID, ok := u.issueTaskIDs[inst.ID]
	if !ok {
		return
	}

	tracing.EndTask(u.cu.comp, tracing.TaskEnd{ID: taskID})
	tracing.AddMilestone(u.cu.comp, tracing.Milestone{
		TaskID: inst.ID,
		Kind:   tracing.MilestoneKindWork,
		What:   "coalesce",
	})
	delete(u.issueTaskIDs, inst.ID)
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *VectorMemoryUnit) CanAcceptWave() bool {
	return u.instructionPipeline.CanAccept()
}

// AcceptWave moves one wavefront into the read buffer of the Scalar unit
func (u *VectorMemoryUnit) AcceptWave(
	wave *wavefront.Wavefront,
) {
	u.instructionPipeline.Accept(vectorMemInst{wavefront: wave})
	u.numInstInFlight++
}

// IsIdle moves one wavefront into the read buffer of the Scalar unit
func (u *VectorMemoryUnit) IsIdle() bool {
	u.isIdle = (u.numInstInFlight == 0) && (u.numTransactionInFlight == 0)
	return u.isIdle
}

// Run executes three pipeline stages that are controlled by the
// VectorMemoryUnit
func (u *VectorMemoryUnit) Run() bool {
	madeProgress := false
	madeProgress = u.sendRequest() || madeProgress
	madeProgress = u.transactionPipeline.Tick(
		&u.postTransactionPipelineBuffer) || madeProgress
	madeProgress = u.instToTransaction() || madeProgress
	madeProgress = u.instructionPipeline.Tick(
		&u.postInstructionPipelineBuffer) || madeProgress
	return madeProgress
}

func (u *VectorMemoryUnit) instToTransaction() bool {
	madeProgress := false

	madeProgress = u.insertTransactionToPipeline() || madeProgress

	// Process up to 4 instructions per cycle (matching simdCount / inst
	// pipeline width)
	for i := 0; i < 4; i++ {
		progress := u.execute()
		madeProgress = progress || madeProgress
		if !progress {
			break
		}
	}

	return madeProgress
}

func (u *VectorMemoryUnit) insertTransactionToPipeline() bool {
	madeProgress := false

	if u.coalescingStallRemaining > 0 {
		u.coalescingStallRemaining--
		return false
	}

	for len(u.transactionsWaiting) > 0 {
		if !u.transactionPipeline.CanAccept() {
			break
		}

		txn := u.transactionsWaiting[0]
		u.transactionPipeline.Accept(txn)
		u.transactionsWaiting = u.transactionsWaiting[1:]
		madeProgress = true

		penalty := u.computeCoalescingPenalty(txn)
		if penalty > 0 {
			u.coalescingStallRemaining = penalty
			break
		}
	}

	return madeProgress
}

func (u *VectorMemoryUnit) computeCoalescingPenalty(
	txn VectorMemAccessInfo,
) int {
	if txn.Read == nil {
		return 0
	}

	maxLanes := 64 / 4 // 64B cacheline / 4B elements = 16
	usedLanes := len(txn.laneInfo)

	if usedLanes >= maxLanes {
		return 0
	}

	wastedFraction := float64(maxLanes-usedLanes) / float64(maxLanes)
	penalty := int(wastedFraction * float64(u.maxCoalescingPenalty))

	return penalty
}

func (u *VectorMemoryUnit) execute() (madeProgress bool) {
	if u.postInstructionPipelineBuffer.Size() == 0 {
		return false
	}

	item := u.postInstructionPipelineBuffer.Peek()

	wave := item.wavefront
	inst := wave.Inst()
	switch inst.FormatType {
	case insts.FLAT:
		ok := u.executeFlatInsts(wave)
		if !ok {
			return false
		}
	default:
		log.Panicf("running inst %s in vector memory unit is not supported",
			insts.NewInstPrinter(nil).Print(inst))
	}

	u.postInstructionPipelineBuffer.Pop()
	u.cu.UpdatePCAndSetReady(wave)
	u.numInstInFlight--

	return true
}

func (u *VectorMemoryUnit) executeFlatInsts(
	wavefront *wavefront.Wavefront,
) bool {
	inst := wavefront.DynamicInst()
	switch inst.Opcode {
	case 16, 17, 18, 19, 20, 21, 22, 23: // FLAT_LOAD_BYTE
		return u.executeFlatLoad(wavefront)
	case 24, 25, 26, 27, 28, 29, 30, 31:
		return u.executeFlatStore(wavefront)
	default:
		log.Panicf("Opcode %d for format FLAT is not supported.", inst.Opcode)
	}

	panic("never")
}

func (u *VectorMemoryUnit) executeFlatLoad(
	wave *wavefront.Wavefront,
) bool {
	transactions := u.coalescer.generateMemTransactions(wave)

	if len(transactions) == 0 {
		u.cu.logInstTask(
			wave,
			wave.DynamicInst(),
			true,
		)
		return true
	}

	if len(transactions)+len(u.cu.InFlightVectorMemAccess) >
		u.cu.InFlightVectorMemAccessLimit {
		return false
	}

	// The in-flight vector-memory-access budget admitted this instruction's
	// transactions: mark the resolution of any wait for a free slot, then open
	// the subtask that records the coalescing / transaction-issue work until
	// the first transaction is sent.
	tracing.AddMilestone(u.cu.comp, tracing.Milestone{
		TaskID: wave.DynamicInst().ID,
		Kind:   tracing.MilestoneKindHardwareResource,
		What:   "vmem-inflight",
	})
	u.startIssueSubtask(wave.DynamicInst())

	wave.OutstandingVectorMemAccess++
	wave.OutstandingScalarMemAccess++

	for i, t := range transactions {
		u.cu.InFlightVectorMemAccess = append(u.cu.InFlightVectorMemAccess, t)
		if i != len(transactions)-1 {
			t.Read.CanWaitForCoalesce = true
		}

		lowModule := u.cu.comp.Resources().VectorMemModules.Find(
			t.Read.Address)
		t.Read.Dst = lowModule
		t.Read.Src = u.cu.vectorMemPort().AsRemote()
		t.Read.PID = wave.PID()
		u.transactionsWaiting = append(u.transactionsWaiting, t)
	}

	return true
}

func (u *VectorMemoryUnit) executeFlatStore(
	wave *wavefront.Wavefront,
) bool {
	transactions := u.coalescer.generateMemTransactions(wave)

	if len(transactions) == 0 {
		u.cu.logInstTask(
			wave,
			wave.DynamicInst(),
			true,
		)
		return true
	}

	if len(transactions)+len(u.cu.InFlightVectorMemAccess) >
		u.cu.InFlightVectorMemAccessLimit {
		return false
	}

	// The in-flight vector-memory-access budget admitted this instruction's
	// transactions: mark the resolution of any wait for a free slot, then open
	// the subtask that records the coalescing / transaction-issue work until
	// the first transaction is sent.
	tracing.AddMilestone(u.cu.comp, tracing.Milestone{
		TaskID: wave.DynamicInst().ID,
		Kind:   tracing.MilestoneKindHardwareResource,
		What:   "vmem-inflight",
	})
	u.startIssueSubtask(wave.DynamicInst())

	wave.OutstandingVectorMemAccess++
	wave.OutstandingScalarMemAccess++

	for i, t := range transactions {
		u.cu.InFlightVectorMemAccess = append(u.cu.InFlightVectorMemAccess, t)
		if i != len(transactions)-1 {
			t.Write.CanWaitForCoalesce = true
		}
		lowModule := u.cu.comp.Resources().VectorMemModules.Find(
			t.Write.Address)
		t.Write.Dst = lowModule
		t.Write.Src = u.cu.vectorMemPort().AsRemote()
		t.Write.PID = wave.PID()
		u.transactionsWaiting = append(u.transactionsWaiting, t)
	}

	return true
}

func (u *VectorMemoryUnit) sendRequest() bool {
	madeProgress := false
	for i := 0; i < 16; i++ {
		if u.postTransactionPipelineBuffer.Size() == 0 {
			break
		}

		info := u.postTransactionPipelineBuffer.Peek()

		// The transaction has reached the head of the send buffer: the
		// coalescing / transaction-issue work is done. Close the issue subtask
		// and emit the "coalesce" work milestone now — before the CanSend check
		// below — so that port backpressure (cycles spent waiting to send) is
		// not counted as coalescing work. It is idempotent (only the first
		// transaction per instruction finds an open subtask), so a port-full
		// retry on the same transaction does not re-emit it.
		u.endIssueSubtask(info.Inst)

		var req messaging.Msg
		if info.Read != nil {
			req = *info.Read
		} else {
			req = *info.Write
		}

		if !u.cu.vectorMemPort().CanSend() {
			break
		}

		u.cu.vectorMemPort().Send(req)
		u.postTransactionPipelineBuffer.Pop()
		u.numTransactionInFlight--

		tracing.TraceReqInitiate(u.cu.comp, req, info.Inst.ID)
		madeProgress = true
	}
	return madeProgress
}

// Flush flushes
func (u *VectorMemoryUnit) Flush() {
	// End any open issue subtasks so they do not leak as started-never-ended
	// (their parent inst tasks are ended by ComputeUnit.endInflightTracingTasks).
	for _, id := range u.issueTaskIDs {
		tracing.EndTaskOnReset(u.cu.comp, id)
	}
	u.issueTaskIDs = make(map[uint64]uint64)

	u.instructionPipeline.Clear()
	u.transactionPipeline.Clear()
	u.postInstructionPipelineBuffer.Clear()
	u.postTransactionPipelineBuffer.Clear()
	u.transactionsWaiting = nil
	u.numInstInFlight = 0
	u.numTransactionInFlight = 0
	u.coalescingStallRemaining = 0
}
