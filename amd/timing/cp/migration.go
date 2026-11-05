package cp

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

// Page migration in the Command Processor.
//
// The driver sends a PageHoldReq, PageShootdownReq, PageReleaseReq, or
// PageCopyReq to the "PageMigration" port. Each request runs as one control
// sequence, so it queues with the other control sequences (cache flush,
// kernel start) and never interleaves with them on the same component. The
// requests are accepted while a kernel runs; a migration never waits for a
// kernel to complete.
//
//	Hold:      Hold every address translator.
//	Release:   Release every address translator.
//	Shootdown: Pause every TLB -> Invalidate{Addresses, PID} -> Enable.
//	Copy:      one data-move request to the data mover (the source page is
//	           outside, the new frame inside).
//
// When the sequence ends, the Command Processor answers the driver with a
// GeneralRsp.

// acceptMigrationReqs queues every request that arrived from the driver, so
// that the requests never block the port.
func (m *ctrlMiddleware) acceptMigrationReqs() bool {
	port := m.comp.Ports.PageMigration
	madeProgress := false

	for {
		msg, ok := port.RetrieveIncoming()
		if !ok {
			return madeProgress
		}

		switch msg.Payload.(type) {
		case protocol.PageHoldReq, protocol.PageShootdownReq,
			protocol.PageReleaseReq, protocol.PageCopyReq:
		default:
			log.Panicf("cp: cannot handle %s on PageMigration",
				reflect.TypeOf(msg.Payload))
		}

		m.comp.State.MigrationReqs = append(m.comp.State.MigrationReqs, msg)
		madeProgress = true
	}
}

// startMigration starts the control sequence of the oldest migration
// request when no other control sequence runs.
func (m *ctrlMiddleware) startMigration() bool {
	state := &m.comp.State
	if state.CtrlSeq != ctrlSeqNone || len(state.MigrationReqs) == 0 {
		return false
	}

	state.CurrMigration = state.MigrationReqs[0]
	state.MigrationReqs = state.MigrationReqs[1:]
	m.startSeq(ctrlSeqMigration)

	return true
}

func (m *ctrlMiddleware) execMigrationStep(step int) int {
	state := &m.comp.State
	spec := m.comp.Spec
	port := m.comp.Ports.ToTranslation
	queue := &state.PendingTranslationReqs

	switch req := state.CurrMigration.Payload.(type) {
	case protocol.PageHoldReq:
		if step == 0 {
			return m.enqueueCtrlReqs(queue, port, spec.AddressTranslators,
				memcontrolprotocol.CmdHold, req.VAddrs, req.PID)
		}
	case protocol.PageReleaseReq:
		if step == 0 {
			return m.enqueueCtrlReqs(queue, port, spec.AddressTranslators,
				memcontrolprotocol.CmdRelease, req.VAddrs, req.PID)
		}
	case protocol.PageShootdownReq:
		switch step {
		case 0:
			return m.enqueueCtrlReqs(queue, port, spec.TLBs,
				memcontrolprotocol.CmdPause, nil, 0)
		case 1:
			return m.enqueueCtrlReqs(queue, port, spec.TLBs,
				memcontrolprotocol.CmdInvalidate, req.VAddrs, req.PID)
		case 2:
			return m.enqueueCtrlReqs(queue, port, spec.TLBs,
				memcontrolprotocol.CmdEnable, nil, 0)
		}
	case protocol.PageCopyReq:
		if step == 0 {
			return m.enqueueMove(req)
		}
	}

	m.finishMigration()

	return 0
}

func (m *ctrlMiddleware) enqueueMove(req protocol.PageCopyReq) int {
	state := &m.comp.State
	state.PendingMoveReqs = append(state.PendingMoveReqs, messaging.Msg{
		ID:  m.comp.NewID(),
		Src: m.comp.Ports.ToDataMover.AsRemote(),
		Dst: m.comp.Spec.DataMover,
		Payload: datamoverprotocol.Req{
			SrcAddress: req.SrcAddress,
			DstAddress: req.DstAddress,
			ByteSize:   req.ByteSize,
			SrcSide:    datamoverprotocol.SideOutside,
			DstSide:    datamoverprotocol.SideInside,
		},
	})

	return 1
}

// finishMigration ends the current migration sequence and answers the
// driver.
func (m *ctrlMiddleware) finishMigration() {
	state := &m.comp.State
	req := state.CurrMigration

	state.PendingMigrationRsps = append(state.PendingMigrationRsps,
		messaging.Msg{
			ID:      m.comp.NewID(),
			Src:     m.comp.Ports.PageMigration.AsRemote(),
			Dst:     req.Src,
			RspTo:   req.ID,
			Payload: protocol.GeneralRsp{},
		})
	state.CurrMigration = messaging.Msg{}
	state.CtrlSeq = ctrlSeqNone
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

func (m *ctrlMiddleware) processRspFromDataMover() bool {
	msg, ok := m.comp.Ports.ToDataMover.PeekIncoming()
	if !ok {
		return false
	}

	if _, ok := msg.Payload.(datamoverprotocol.Rsp); !ok {
		log.Panicf("cp: cannot handle %s from the data mover",
			reflect.TypeOf(msg.Payload))
	}

	m.comp.Ports.ToDataMover.RetrieveIncoming()
	m.ackReceived()

	return true
}
