package cp

import (
	"fmt"
	"log"
	"reflect"
	"slices"

	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

// Page migration in the Command Processor.
//
// The driver sends one command per page to the "PageMigration" port. Every
// command becomes a page job, and the jobs of different pages run at the
// same time: none waits for another page, for a kernel, or for a control
// sequence, except where a component must be shared (see below). Below the
// Command Processor, every request names one page by a half-open byte range
// and has a fixed size.
//
//	Hold:       Hold of the page in every address translator.
//	Release:    Release of the page in every address translator.
//	Flush:      on every L2 bank, Flush of the frame and, after the
//	            acknowledgement of that bank, Invalidate.
//	Invalidate: on every L2 bank, Invalidate of the frame.
//	Copy:       one data-move request to the data mover, which pulls the
//	            page from another GPU.
//	Shootdown:  Pause of every TLB, Invalidate of the page, Enable.
//
// A job answers the driver with a GeneralRsp after its last acknowledgement.
//
// The L2 maintenance of frames that have no byte in common runs at the same
// time: the requests of several jobs are with a bank together, and a bank
// gets the Invalidate of a job once it has acknowledged the Flush of that
// job, whatever the other jobs do. Frames that have a byte in common are
// maintained in the order their commands arrived. The flush sequence of the
// driver, which drains and flushes the whole of every bank, goes first: while
// a flush is next at the driver port or runs, no L2 maintenance starts. The
// maintenance that has started runs to its end, and the maintenance that
// waits starts when no flush is next anymore, so after the last of several
// flushes that follow each other.
//
// The TLBs are shot down in rounds: a round pauses the TLBs once, invalidates
// the pages of every Shootdown that waits, and enables the TLBs; the
// Shootdowns that arrive during a round form the next one. The data mover
// takes the Copy requests in the order they arrive, so the driver decides the
// order in which the pages of its windows are submitted.

// pageJob is a command of the driver in service. PendingAcks counts the
// acknowledgements that must arrive before the Command Processor answers.
type pageJob struct {
	Req         messaging.Msg `json:"req"`
	PendingAcks int           `json:"pending_acks"`
	TaskID      uint64        `json:"task_id"`
}

// ctrlWait tells what waits for the acknowledgement of a control request:
// page job Job, or the TLB shootdown round if Job is zero. A request of the
// L2 maintenance of a job went to L2 bank Bank-1; Flush is set if it is the
// Flush, which the Invalidate of that bank follows.
type ctrlWait struct {
	Job   uint64 `json:"job"`
	Bank  int    `json:"bank"`
	Flush bool   `json:"flush"`
}

// l2Op is the L2 maintenance of a page job on the frame from Start to End:
// on every bank an Invalidate, after a Flush if Flush is set. Started is set
// once its first requests are on their way to the banks.
type l2Op struct {
	Job     uint64 `json:"job"`
	Flush   bool   `json:"flush"`
	Start   uint64 `json:"start"`
	End     uint64 `json:"end"`
	Started bool   `json:"started"`
}

// Steps of a TLB shootdown round.
const (
	shootdownIdle = iota
	shootdownPause
	shootdownInvalidate
	shootdownEnable
)

// shootdownState is the TLB shootdown: the Shootdown jobs of the round in
// progress, and the jobs that wait for the next round.
type shootdownState struct {
	Step        int      `json:"step"`
	Jobs        []uint64 `json:"jobs"`
	Waiting     []uint64 `json:"waiting"`
	PendingAcks int      `json:"pending_acks"`
}

// acceptMigrationReqs starts a page job for every command that arrived from
// the driver, so that the commands never block the port.
func (m *ctrlMiddleware) acceptMigrationReqs() bool {
	port := m.comp.Ports.PageMigration
	madeProgress := false

	for {
		msg, ok := port.RetrieveIncoming()
		if !ok {
			return madeProgress
		}

		m.startPageJob(msg)
		madeProgress = true
	}
}

func (m *ctrlMiddleware) startPageJob(msg messaging.Msg) {
	state := &m.comp.State
	spec := m.comp.Spec
	job := pageJob{Req: msg, TaskID: m.comp.TaskTracker().StartReqIn(m.comp, msg)}

	switch req := msg.Payload.(type) {
	case protocol.PageHoldReq:
		job.PendingAcks = m.enqueuePageCtrl(msg.ID, spec.AddressTranslators,
			memcontrolprotocol.CmdHold, req.PID, req.VAddr, req.PageSize)
	case protocol.PageReleaseReq:
		job.PendingAcks = m.enqueuePageCtrl(msg.ID, spec.AddressTranslators,
			memcontrolprotocol.CmdRelease, req.PID, req.VAddr, req.PageSize)
	case protocol.PageShootdownReq:
		state.Shootdown.Waiting = append(state.Shootdown.Waiting, msg.ID)
		job.PendingAcks = 1
	case protocol.PageFlushReq:
		job.PendingAcks = m.enqueueL2Op(msg.ID, true, req.PAddr, req.PageSize)
	case protocol.PageInvalidateReq:
		job.PendingAcks = m.enqueueL2Op(msg.ID, false, req.PAddr, req.PageSize)
	case protocol.PageCopyReq:
		m.enqueueMove(msg.ID, req)
		job.PendingAcks = 1
	default:
		log.Panicf("cp: cannot handle %s on PageMigration",
			reflect.TypeOf(msg.Payload))
	}

	state.PageJobs[msg.ID] = job

	if job.PendingAcks == 0 {
		m.finishPageJob(msg.ID)
	}
}

// pageRange is the half-open byte range of a page or a frame.
func pageRange(addr, size uint64) memcontrolprotocol.AddressRange {
	return memcontrolprotocol.AddressRange{Start: addr, End: addr + size}
}

// enqueuePageCtrl queues one control request for the page for every
// address translator and returns the number of requests.
func (m *ctrlMiddleware) enqueuePageCtrl(
	job uint64,
	dsts []messaging.RemotePort,
	cmd memcontrolprotocol.Command,
	pid vm.PID,
	vAddr, size uint64,
) int {
	state := &m.comp.State
	port := m.comp.Ports.ToTranslation

	for _, dst := range dsts {
		req := messaging.Msg{
			ID:  m.comp.NewID(),
			Src: port.AsRemote(),
			Dst: dst,
			Payload: memcontrolprotocol.Req{
				Command:  cmd,
				PID:      pid,
				HasRange: true,
				Range:    pageRange(vAddr, size),
			},
		}

		state.PendingTranslationReqs = append(state.PendingTranslationReqs, req)
		state.CtrlWaits[req.ID] = ctrlWait{Job: job}
	}

	return len(dsts)
}

// enqueueL2Op queues the L2 maintenance of a frame behind the maintenance
// that arrived before it. Every L2 bank takes part: with the interleaving of
// the banks, a page has lines in each of them. It returns the number of
// acknowledgements that end the job, the Invalidate of every bank.
func (m *ctrlMiddleware) enqueueL2Op(
	job uint64,
	flush bool,
	pAddr, size uint64,
) int {
	state := &m.comp.State
	banks := len(m.comp.Spec.L2Caches)

	if banks > 0 {
		state.L2Ops = append(state.L2Ops,
			l2Op{Job: job, Flush: flush, Start: pAddr, End: pAddr + size})
	}

	return banks
}

// l2OpsStarted reports whether L2 maintenance of a page is in progress.
func (m *ctrlMiddleware) l2OpsStarted() bool {
	return slices.ContainsFunc(m.comp.State.L2Ops,
		func(op l2Op) bool { return op.Started })
}

// flushIsNext reports whether a flush of the driver is the next request at
// the driver port: it has not started, because L2 maintenance that has
// started, another control sequence, or the answer to the previous flush is
// in its way.
func (m *ctrlMiddleware) flushIsNext() bool {
	msg, ok := m.toDriver().PeekIncoming()
	if !ok {
		return false
	}

	_, isFlush := msg.Payload.(protocol.FlushReq)

	return isFlush
}

// startL2Ops starts the L2 maintenance that waits. An operation waits while
// an operation that arrived before it, started or not, has a byte of its
// frame in common, so such frames are maintained in arrival order. No
// operation starts while the flush sequence of the driver runs or is next
// at the driver port, so flushes that follow each other all go before the
// operations that have not started.
func (m *ctrlMiddleware) startL2Ops() bool {
	state := &m.comp.State
	if state.CtrlSeq == ctrlSeqFlush || m.flushIsNext() {
		return false
	}

	madeProgress := false

	for i := range state.L2Ops {
		op := &state.L2Ops[i]
		if op.Started || slices.ContainsFunc(state.L2Ops[:i], op.intersects) {
			continue
		}

		op.Started = true

		cmd := memcontrolprotocol.CmdInvalidate
		if op.Flush {
			cmd = memcontrolprotocol.CmdFlush
		}

		for bank := range m.comp.Spec.L2Caches {
			m.enqueueL2Ctrl(*op, bank, cmd)
		}

		madeProgress = true
	}

	return madeProgress
}

// intersects reports whether the frames of two operations have a byte in
// common.
func (op l2Op) intersects(other l2Op) bool {
	return op.Start < other.End && other.Start < op.End
}

func (m *ctrlMiddleware) enqueueL2Ctrl(
	op l2Op,
	bank int,
	cmd memcontrolprotocol.Command,
) {
	state := &m.comp.State
	req := messaging.Msg{
		ID:  m.comp.NewID(),
		Src: m.toCaches().AsRemote(),
		Dst: m.comp.Spec.L2Caches[bank],
		Payload: memcontrolprotocol.Req{
			Command:  cmd,
			HasRange: true,
			Range:    memcontrolprotocol.AddressRange{Start: op.Start, End: op.End},
		},
	}

	state.PendingCacheReqs = append(state.PendingCacheReqs, req)
	state.CtrlWaits[req.ID] = ctrlWait{
		Job: op.Job, Bank: bank + 1, Flush: cmd == memcontrolprotocol.CmdFlush,
	}
}

// l2Acked takes in the acknowledgement of an L2 bank for a page job. A bank
// gets the Invalidate of a frame only after it has acknowledged the Flush of
// the job: the lower level has then acknowledged the writebacks. The
// operation ends with the Invalidate acknowledgement of the last bank.
func (m *ctrlMiddleware) l2Acked(wait ctrlWait) {
	state := &m.comp.State
	i := slices.IndexFunc(state.L2Ops,
		func(op l2Op) bool { return op.Job == wait.Job })

	if wait.Flush {
		m.enqueueL2Ctrl(state.L2Ops[i], wait.Bank-1, memcontrolprotocol.CmdInvalidate)
		return
	}

	if m.pageJobAcked(wait.Job) {
		state.L2Ops = slices.Delete(state.L2Ops, i, i+1)
	}
}

// advanceShootdown starts a round for the Shootdown jobs that wait when no
// round is in progress.
func (m *ctrlMiddleware) advanceShootdown() bool {
	s := &m.comp.State.Shootdown
	if s.Step != shootdownIdle || len(s.Waiting) == 0 {
		return false
	}

	s.Jobs, s.Waiting = s.Waiting, nil
	s.Step = shootdownPause
	s.PendingAcks = m.enqueueTLBCtrl(memcontrolprotocol.Req{
		Command: memcontrolprotocol.CmdPause})

	if s.PendingAcks == 0 {
		m.finishShootdown()
	}

	return true
}

// enqueueTLBCtrl queues a control request for every TLB.
func (m *ctrlMiddleware) enqueueTLBCtrl(payload memcontrolprotocol.Req) int {
	state := &m.comp.State
	port := m.comp.Ports.ToTranslation

	for _, dst := range m.comp.Spec.TLBs {
		req := messaging.Msg{
			ID:      m.comp.NewID(),
			Src:     port.AsRemote(),
			Dst:     dst,
			Payload: payload,
		}

		state.PendingTranslationReqs = append(state.PendingTranslationReqs, req)
		state.CtrlWaits[req.ID] = ctrlWait{}
	}

	return len(m.comp.Spec.TLBs)
}

// shootdownAcked takes in an acknowledgement of a TLB and advances the round
// once every TLB has answered the step.
func (m *ctrlMiddleware) shootdownAcked() {
	state := &m.comp.State
	s := &state.Shootdown

	s.PendingAcks--
	if s.PendingAcks > 0 {
		return
	}

	switch s.Step {
	case shootdownPause:
		s.Step = shootdownInvalidate

		// One page after the other, so that a full Control port of a TLB
		// delays one request of every page instead of every request.
		for _, id := range s.Jobs {
			req := state.PageJobs[id].Req.Payload.(protocol.PageShootdownReq)
			s.PendingAcks += m.enqueueTLBCtrl(memcontrolprotocol.Req{
				Command:  memcontrolprotocol.CmdInvalidate,
				PID:      req.PID,
				HasRange: true,
				Range:    pageRange(req.VAddr, req.PageSize),
			})
		}
	case shootdownInvalidate:
		s.Step = shootdownEnable
		s.PendingAcks = m.enqueueTLBCtrl(memcontrolprotocol.Req{
			Command: memcontrolprotocol.CmdEnable})
	case shootdownEnable:
		m.finishShootdown()
	default:
		panic(fmt.Sprintf("cp: TLB acknowledgement in shootdown step %d", s.Step))
	}
}

func (m *ctrlMiddleware) finishShootdown() {
	s := &m.comp.State.Shootdown
	jobs := s.Jobs
	s.Jobs = nil
	s.Step = shootdownIdle

	for _, id := range jobs {
		m.pageJobAcked(id)
	}
}

// enqueueMove queues the data-move request of a Copy: the data mover of this
// GPU, the destination, reads the page from outside and writes it inside. A
// GPU that was built without the memory of other GPUs has no data mover, and
// no page can migrate to it.
func (m *ctrlMiddleware) enqueueMove(job uint64, req protocol.PageCopyReq) {
	if m.comp.Spec.DataMover == "" {
		panic("cp: " + m.comp.Name() + " has no data mover to copy a page with")
	}

	state := &m.comp.State
	move := messaging.Msg{
		ID:  m.comp.NewID(),
		Src: m.comp.Ports.ToDataMover.AsRemote(),
		Dst: m.comp.Spec.DataMover,
		Payload: datamoverprotocol.Req{
			SrcAddress: req.SrcAddress,
			DstAddress: req.DstAddress,
			ByteSize:   req.PageSize,
			SrcSide:    datamoverprotocol.SideOutside,
			DstSide:    datamoverprotocol.SideInside,
		},
	}

	state.PendingMoveReqs = append(state.PendingMoveReqs, move)
	state.MoveJobs[move.ID] = job
}

// pageJobAcked counts an acknowledgement of a page job, ends the job with
// the last one, and reports whether it did.
func (m *ctrlMiddleware) pageJobAcked(id uint64) (finished bool) {
	state := &m.comp.State

	job, ok := state.PageJobs[id]
	if !ok {
		panic(fmt.Sprintf("cp: acknowledgement for unknown page job %d", id))
	}

	job.PendingAcks--
	state.PageJobs[id] = job

	if job.PendingAcks > 0 {
		return false
	}

	m.finishPageJob(id)

	return true
}

// finishPageJob ends a page job and queues its answer to the driver.
func (m *ctrlMiddleware) finishPageJob(id uint64) {
	state := &m.comp.State
	job := state.PageJobs[id]

	delete(state.PageJobs, id)
	state.PendingMigrationRsps = append(state.PendingMigrationRsps,
		messaging.Msg{
			ID:      m.comp.NewID(),
			Src:     m.comp.Ports.PageMigration.AsRemote(),
			Dst:     job.Req.Src,
			RspTo:   job.Req.ID,
			Payload: protocol.GeneralRsp{},
		})
	m.comp.TaskTracker().EndReqIn(job.TaskID)
}

// sendMigrationMsgs sends the queued control requests, data-move requests,
// and answers to the driver.
func (m *ctrlMiddleware) sendMigrationMsgs() bool {
	state := &m.comp.State
	madeProgress := drainCtrlQueue(m.comp.Ports.ToTranslation,
		&state.PendingTranslationReqs)
	madeProgress = drainCtrlQueue(m.comp.Ports.ToDataMover,
		&state.PendingMoveReqs) || madeProgress
	madeProgress = drainCtrlQueue(m.comp.Ports.PageMigration,
		&state.PendingMigrationRsps) || madeProgress

	return madeProgress
}

// migrationAcked takes in the acknowledgement of a control request of a
// page job.
func (m *ctrlMiddleware) migrationAcked(reqID uint64) {
	state := &m.comp.State

	wait, ok := state.CtrlWaits[reqID]
	if !ok {
		panic(fmt.Sprintf("cp: unexpected control response to %d", reqID))
	}

	delete(state.CtrlWaits, reqID)

	switch {
	case wait.Bank > 0:
		m.l2Acked(wait)
	case wait.Job == 0:
		m.shootdownAcked()
	default:
		m.pageJobAcked(wait.Job)
	}
}

// processRspFromTranslation takes in every acknowledgement of the address
// translators and the TLBs.
func (m *ctrlMiddleware) processRspFromTranslation() bool {
	port := m.comp.Ports.ToTranslation
	madeProgress := false

	for {
		msg, ok := port.RetrieveIncoming()
		if !ok {
			return madeProgress
		}

		mustBeSuccessfulCtrlRsp(msg)
		m.migrationAcked(msg.RspTo)

		madeProgress = true
	}
}

// processRspFromDataMover takes in every completed page copy.
func (m *ctrlMiddleware) processRspFromDataMover() bool {
	state := &m.comp.State
	port := m.comp.Ports.ToDataMover
	madeProgress := false

	for {
		msg, ok := port.RetrieveIncoming()
		if !ok {
			return madeProgress
		}

		if _, ok := msg.Payload.(datamoverprotocol.Rsp); !ok {
			log.Panicf("cp: cannot handle %s from the data mover",
				reflect.TypeOf(msg.Payload))
		}

		job, ok := state.MoveJobs[msg.RspTo]
		if !ok {
			panic(fmt.Sprintf("cp: unexpected data-move response to %d", msg.RspTo))
		}

		delete(state.MoveJobs, msg.RspTo)
		m.pageJobAcked(job)

		madeProgress = true
	}
}
