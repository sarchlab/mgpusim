package cp

import (
	"fmt"
	"log"
	"reflect"

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
//	Flush:      on every L2 bank, Flush of the frame and, after its
//	            acknowledgement, Invalidate.
//	Invalidate: on every L2 bank, Invalidate of the frame.
//	Copy:       one data-move request to the inbound data mover (pull) or
//	            to the outbound data mover (push).
//	Shootdown:  Pause of every TLB, Invalidate of the page, Enable.
//
// A job answers the driver with a GeneralRsp after its last acknowledgement.
//
// An L2 bank runs one maintenance operation at a time: the Flush and the
// Invalidate of a frame are a pair that no other operation of the bank
// interrupts, and the flush sequence of the driver, which drains and flushes
// the whole bank, runs only while no bank has an operation in progress. The
// TLBs are shot down in rounds: a round pauses the TLBs once, invalidates the
// pages of every Shootdown that waits, and enables the TLBs; the Shootdowns
// that arrive during a round form the next one. The data movers take the
// Copy requests in the order they arrive, so the driver decides the order in
// which the pages of its windows are submitted.

// pageJob is a command of the driver in service. PendingAcks counts the
// acknowledgements that must arrive before the Command Processor answers.
type pageJob struct {
	Req         messaging.Msg `json:"req"`
	PendingAcks int           `json:"pending_acks"`
	TaskID      uint64        `json:"task_id"`
}

// ctrlWait tells what waits for the acknowledgement of a control request:
// the current operation of L2 bank Bank-1, the TLB shootdown round (Job is
// zero), or page job Job.
type ctrlWait struct {
	Job  uint64 `json:"job"`
	Bank int    `json:"bank"`
}

// bankOp is a maintenance operation of one L2 bank on the frame from Start
// to End: an Invalidate, after a Flush if Flush is set.
type bankOp struct {
	Job   uint64 `json:"job"`
	Flush bool   `json:"flush"`
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
}

// bankState is the maintenance of one L2 bank: the operation in progress, if
// Busy, and the operations that wait, in arrival order. Invalidating is set
// once the Invalidate of the operation in progress is sent.
type bankState struct {
	Queue        []bankOp `json:"queue"`
	Busy         bool     `json:"busy"`
	Curr         bankOp   `json:"curr"`
	Invalidating bool     `json:"invalidating"`
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
		job.PendingAcks = m.enqueueBankOps(msg.ID, true, req.PAddr, req.PageSize)
	case protocol.PageInvalidateReq:
		job.PendingAcks = m.enqueueBankOps(msg.ID, false, req.PAddr, req.PageSize)
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

// enqueueBankOps queues the maintenance of a frame on every L2 bank. With
// the interleaving of the banks, a page has lines in each of them.
func (m *ctrlMiddleware) enqueueBankOps(
	job uint64,
	flush bool,
	pAddr, size uint64,
) int {
	state := &m.comp.State
	op := bankOp{Job: job, Flush: flush, Start: pAddr, End: pAddr + size}

	for i := range state.Banks {
		state.Banks[i].Queue = append(state.Banks[i].Queue, op)
	}

	return len(state.Banks)
}

// banksBusy reports whether an L2 bank has a maintenance operation in
// progress.
func (m *ctrlMiddleware) banksBusy() bool {
	for _, b := range m.comp.State.Banks {
		if b.Busy {
			return true
		}
	}

	return false
}

// advanceBanks starts the next operation of every idle L2 bank. No
// operation starts while the flush sequence of the driver runs or waits for
// the banks.
func (m *ctrlMiddleware) advanceBanks() bool {
	state := &m.comp.State
	if state.CtrlSeq == ctrlSeqFlush || state.FlushWaiting {
		return false
	}

	madeProgress := false

	for i := range state.Banks {
		b := &state.Banks[i]
		if b.Busy || len(b.Queue) == 0 {
			continue
		}

		b.Curr = b.Queue[0]
		b.Queue = b.Queue[1:]
		b.Busy = true
		b.Invalidating = !b.Curr.Flush

		cmd := memcontrolprotocol.CmdFlush
		if b.Invalidating {
			cmd = memcontrolprotocol.CmdInvalidate
		}

		m.enqueueBankCtrl(i, cmd)

		madeProgress = true
	}

	return madeProgress
}

func (m *ctrlMiddleware) enqueueBankCtrl(
	bank int,
	cmd memcontrolprotocol.Command,
) {
	state := &m.comp.State
	op := state.Banks[bank].Curr
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
	state.CtrlWaits[req.ID] = ctrlWait{Job: op.Job, Bank: bank + 1}
}

// bankAcked takes in the acknowledgement of the current operation of an L2
// bank. The Invalidate of a frame is sent only after the acknowledgement of
// its Flush: the lower level has then acknowledged the writebacks.
func (m *ctrlMiddleware) bankAcked(bank int) {
	b := &m.comp.State.Banks[bank]

	if !b.Invalidating {
		b.Invalidating = true
		m.enqueueBankCtrl(bank, memcontrolprotocol.CmdInvalidate)

		return
	}

	job := b.Curr.Job
	*b = bankState{Queue: b.Queue}

	m.pageJobAcked(job)
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

// enqueueMove queues the data-move request of a Copy. A pull reads the page
// from outside and writes it inside with the inbound data mover; a push
// reads it inside and writes it outside with the outbound data mover.
func (m *ctrlMiddleware) enqueueMove(job uint64, req protocol.PageCopyReq) {
	state := &m.comp.State
	move := messaging.Msg{
		ID:  m.comp.NewID(),
		Src: m.comp.Ports.ToDataMover.AsRemote(),
		Dst: m.comp.Spec.InboundDataMover,
		Payload: datamoverprotocol.Req{
			SrcAddress: req.SrcAddress,
			DstAddress: req.DstAddress,
			ByteSize:   req.PageSize,
			SrcSide:    datamoverprotocol.SideOutside,
			DstSide:    datamoverprotocol.SideInside,
		},
	}

	if req.Push {
		move.Dst = m.comp.Spec.OutboundDataMover
		move.Payload = datamoverprotocol.Req{
			SrcAddress: req.SrcAddress,
			DstAddress: req.DstAddress,
			ByteSize:   req.PageSize,
			SrcSide:    datamoverprotocol.SideInside,
			DstSide:    datamoverprotocol.SideOutside,
		}
	}

	state.PendingMoveReqs = append(state.PendingMoveReqs, move)
	state.MoveJobs[move.ID] = job
}

// pageJobAcked counts an acknowledgement of a page job and ends the job with
// the last one.
func (m *ctrlMiddleware) pageJobAcked(id uint64) {
	state := &m.comp.State

	job, ok := state.PageJobs[id]
	if !ok {
		panic(fmt.Sprintf("cp: acknowledgement for unknown page job %d", id))
	}

	job.PendingAcks--
	state.PageJobs[id] = job

	if job.PendingAcks == 0 {
		m.finishPageJob(id)
	}
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
		m.bankAcked(wait.Bank - 1)
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
			log.Panicf("cp: cannot handle %s from a data mover",
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
