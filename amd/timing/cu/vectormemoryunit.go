package cu

import (
	"log"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

type vectorMemInst struct {
	Handle waveHandle
}

// A VectorMemoryUnit is the block in a compute unit that can performs vector
// memory operations.
type VectorMemoryUnit struct {
	cu *ComputeUnit

	coalescer coalescer

	maxCoalescingPenalty int
}

// NewVectorMemoryUnit creates a new Vector Memory Unit.
func NewVectorMemoryUnit(
	cu *ComputeUnit,
	coalescer coalescer,
) *VectorMemoryUnit {
	u := new(VectorMemoryUnit)
	u.cu = cu
	u.coalescer = coalescer

	return u
}

// startIssueSubtask opens the "pipeline" subtask that spans an instruction's
// coalescing / transaction-issue work (from the in-flight admission to the
// first transaction send), parented to the instruction's task. It is the child
// subtask that backs the "work" milestone emitted at first send.
func (u *VectorMemoryUnit) startIssueSubtask(inst uint64) {
	taskID := u.cu.comp.TaskTracker().StartTask(u.cu.comp, tasks.StartInfo{
		ParentID: u.cu.comp.State.InstTasks[inst],
		Kind:     "pipeline",
		What:     u.cu.comp.Name() + ".coalesce",
	})
	u.state().IssueTasks[inst] = taskID
}

// endIssueSubtask closes an instruction's issue subtask (if still open) when
// its first transaction reaches the head of the send buffer (the issue work is
// done), and emits the "work" milestone the subtask backs. Later transactions
// of the same instruction — and port-full retries on the same one — find no
// open subtask and are no-ops, so the milestone lands once, at the moment the
// transaction is ready to send rather than when it actually leaves.
func (u *VectorMemoryUnit) endIssueSubtask(inst uint64) {
	taskID, ok := u.state().IssueTasks[inst]
	if !ok {
		return
	}

	u.cu.comp.TaskTracker().EndTask(taskID)
	u.cu.comp.TaskTracker().AddMilestone(tasks.Milestone{
		TaskID: u.cu.comp.State.InstTasks[inst],
		Kind:   tasks.MilestoneKindWork,
		What:   u.cu.comp.Name() + ".coalesce",
	})
	delete(u.state().IssueTasks, inst)
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *VectorMemoryUnit) CanAcceptWave() bool {
	return u.state().InstructionsPipeline.CanAccept()
}

// AcceptWave moves one wavefront into the read buffer of the Scalar unit
func (u *VectorMemoryUnit) AcceptWave(
	wave *Wavefront,
) {
	u.state().InstructionsPipeline.Accept(vectorMemInst{Handle: handleOf(wave)})
	u.state().Instructions++
}

// IsIdle moves one wavefront into the read buffer of the Scalar unit
func (u *VectorMemoryUnit) IsIdle() bool {
	u.state().Idle = (u.state().Instructions == 0) && (u.state().Transactions == 0)
	return u.state().Idle
}

// Run executes three pipeline stages that are controlled by the
// VectorMemoryUnit
func (u *VectorMemoryUnit) Run() bool {
	madeProgress := false
	madeProgress = u.sendRequest() || madeProgress
	madeProgress = u.state().TransactionsPipeline.Tick(
		&u.state().TransactionsOutput) || madeProgress
	madeProgress = u.instToTransaction() || madeProgress
	madeProgress = u.state().InstructionsPipeline.Tick(
		&u.state().InstructionsOutput) || madeProgress
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

	if u.state().Stall > 0 {
		u.state().Stall--
		return false
	}

	for len(u.state().Waiting) > 0 {
		if !u.state().TransactionsPipeline.CanAccept() {
			break
		}

		txn := u.state().Waiting[0]
		u.state().TransactionsPipeline.Accept(txn.requestID())
		u.state().Waiting = u.state().Waiting[1:]
		madeProgress = true

		penalty := u.computeCoalescingPenalty(txn)
		if penalty > 0 {
			u.state().Stall = penalty
			break
		}
	}

	return madeProgress
}

func (u *VectorMemoryUnit) computeCoalescingPenalty(
	txn VectorAccessState,
) int {
	if txn.Read.ID == 0 {
		return 0
	}

	maxLanes := 64 / 4 // 64B cacheline / 4B elements = 16
	usedLanes := len(txn.Lanes)

	if usedLanes >= maxLanes {
		return 0
	}

	wastedFraction := float64(maxLanes-usedLanes) / float64(maxLanes)
	penalty := int(wastedFraction * float64(u.maxCoalescingPenalty))

	return penalty
}

func (u *VectorMemoryUnit) execute() (madeProgress bool) {
	if u.state().InstructionsOutput.Size() == 0 {
		return false
	}

	item, _ := u.state().InstructionsOutput.Peek()

	wave := u.cu.wave(item.Handle)
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

	u.state().InstructionsOutput.Pop()
	u.cu.UpdatePCAndSetReady(wave)
	u.state().Instructions--

	return true
}

func (u *VectorMemoryUnit) executeFlatInsts(
	wavefront *Wavefront,
) bool {
	inst := wavefront.Inst()
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
	wave *Wavefront,
) bool {
	transactions := u.coalescer.generateMemTransactions(wave)

	if len(transactions) == 0 {
		u.cu.completeInstruction(wave,
			wave.InstID())
		return true
	}

	if len(transactions)+len(u.cu.comp.State.Vector) >
		u.cu.InFlightVectorMemAccessLimit {
		return false
	}

	// The in-flight vector-memory-access budget admitted this instruction's
	// transactions: mark the resolution of any wait for a free slot, then open
	// the subtask that records the coalescing / transaction-issue work until
	// the first transaction is sent.
	u.cu.comp.TaskTracker().AddMilestone(tasks.Milestone{
		TaskID: u.cu.comp.State.InstTasks[wave.InstID()],
		Kind:   tasks.MilestoneKindHardwareResource,
		What:   u.cu.comp.Name() + ".vmem-inflight-reg",
	})
	u.startIssueSubtask(wave.InstID())

	wave.execution().OutstandingVectorMemAccess++
	wave.execution().OutstandingScalarMemAccess++

	for _, t := range transactions {
		payload := t.Read.Payload.(memprotocol.ReadReq)

		lowModule := u.cu.comp.Spec.VectorMemModules.Find(
			t.Read.Payload.(memprotocol.ReadReq).Address)
		t.Read.Dst = lowModule
		t.Read.Src = u.cu.vectorMemPort().AsRemote()
		payload.PID = wave.PID()
		t.Read.Payload = payload
		// The coalescer allocates IDs in slice order. If the capacity check
		// above rejects a batch, a retry allocates newer IDs; admitted IDs
		// therefore increase monotonically.
		u.cu.admitVector(u.cu.vectorState(t))
		u.state().Waiting = append(u.state().Waiting, u.cu.vectorState(t))
	}

	return true
}

func (u *VectorMemoryUnit) executeFlatStore(
	wave *Wavefront,
) bool {
	transactions := u.coalescer.generateMemTransactions(wave)

	if len(transactions) == 0 {
		u.cu.completeInstruction(wave,
			wave.InstID())
		return true
	}

	if len(transactions)+len(u.cu.comp.State.Vector) >
		u.cu.InFlightVectorMemAccessLimit {
		return false
	}

	// The in-flight vector-memory-access budget admitted this instruction's
	// transactions: mark the resolution of any wait for a free slot, then open
	// the subtask that records the coalescing / transaction-issue work until
	// the first transaction is sent.
	u.cu.comp.TaskTracker().AddMilestone(tasks.Milestone{
		TaskID: u.cu.comp.State.InstTasks[wave.InstID()],
		Kind:   tasks.MilestoneKindHardwareResource,
		What:   u.cu.comp.Name() + ".vmem-inflight-reg",
	})
	u.startIssueSubtask(wave.InstID())

	wave.execution().OutstandingVectorMemAccess++
	wave.execution().OutstandingScalarMemAccess++

	for _, t := range transactions {
		payload := t.Write.Payload.(memprotocol.WriteReq)
		lowModule := u.cu.comp.Spec.VectorMemModules.Find(
			t.Write.Payload.(memprotocol.WriteReq).Address)
		t.Write.Dst = lowModule
		t.Write.Src = u.cu.vectorMemPort().AsRemote()
		payload.PID = wave.PID()
		t.Write.Payload = payload
		// The coalescer allocates IDs in slice order. If the capacity check
		// above rejects a batch, a retry allocates newer IDs; admitted IDs
		// therefore increase monotonically.
		u.cu.admitVector(u.cu.vectorState(t))
		u.state().Waiting = append(u.state().Waiting, u.cu.vectorState(t))
	}

	return true
}

func (u *VectorMemoryUnit) sendRequest() bool {
	madeProgress := false
	for i := 0; i < 16; i++ {
		if u.state().TransactionsOutput.Size() == 0 {
			break
		}

		id, _ := u.state().TransactionsOutput.Peek()
		info := u.cu.vectorTransactionByID(id)

		// The transaction has reached the head of the send buffer: the
		// coalescing / transaction-issue work is done. Close the issue subtask
		// and emit the "coalesce" work milestone now — before the CanSend check
		// below — so that port backpressure (cycles spent waiting to send) is
		// not counted as coalescing work. It is idempotent (only the first
		// transaction per instruction finds an open subtask), so a port-full
		// retry on the same transaction does not re-emit it.
		u.endIssueSubtask(info.Inst)

		var req messaging.Msg
		if info.Read.ID != 0 {
			req = info.Read
		} else {
			req = info.Write
		}

		if !u.cu.vectorMemPort().CanSend() {
			break
		}

		u.cu.startRequest(req, u.cu.comp.State.InstTasks[info.Inst])
		u.cu.vectorMemPort().Send(req)
		u.state().TransactionsOutput.Pop()
		u.state().Transactions--

		madeProgress = true
	}
	return madeProgress
}

func (u *VectorMemoryUnit) state() *VectorUnitState { return &u.cu.comp.State.VectorUnit }
