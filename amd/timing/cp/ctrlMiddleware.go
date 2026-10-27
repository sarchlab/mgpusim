package cp

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/rdma"
)

// ctrlMiddleware runs cache flush, kernel-start L1 invalidation, and RDMA
// drain/restart. Driver flush drains, flushes, and invalidates L1 before L2,
// then enables all caches. Kernel start drains, invalidates, and enables
// L1S/L1V so an earlier kernel cannot leave stale private cache lines.
// Each step waits for all acknowledgements before advancing.
type ctrlMiddleware struct {
	comp *Comp
}

func (m *ctrlMiddleware) toDriver() messaging.Port {
	return m.comp.Ports.ToDriver
}

func (m *ctrlMiddleware) toCaches() messaging.Port {
	return m.comp.Ports.ToCaches
}

func (m *ctrlMiddleware) toRDMA() messaging.Port {
	return m.comp.Ports.ToRDMA
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false

	madeProgress = m.sendPendingReqs() || madeProgress
	madeProgress = m.sendPendingDriverRsps() || madeProgress
	madeProgress = m.processReqFromDriver() || madeProgress
	madeProgress = m.processRspFromRDMA() || madeProgress
	madeProgress = m.processRspFromCaches() || madeProgress

	return madeProgress
}

// sendPendingReqs drains the outbound control-message queues as the ports
// become available.
func (m *ctrlMiddleware) sendPendingReqs() bool {
	return drainCtrlQueue(m.toCaches(), &m.comp.State.PendingCacheReqs)
}

func drainCtrlQueue(
	port messaging.Port,
	queue *[]messaging.Msg,
) bool {
	madeProgress := false

	for len(*queue) > 0 && port.CanSend() {
		port.Send((*queue)[0])
		*queue = (*queue)[1:]
		madeProgress = true
	}

	return madeProgress
}

// sendPendingDriverRsps sends the queued responses to the driver.
func (m *ctrlMiddleware) sendPendingDriverRsps() bool {
	state := &m.comp.State

	if len(state.PendingDriverRsps) == 0 {
		return false
	}

	if !m.toDriver().CanSend() {
		return false
	}

	kind := state.PendingDriverRsps[0]
	meta := messaging.Msg{
		ID:  m.comp.NewID(),
		Src: m.toDriver().AsRemote(),
		Dst: state.Driver,
	}

	switch kind {
	case driverRspFlushDone:
		meta.Dst = state.CurrFlushReq.Src
		meta.RspTo = state.CurrFlushReq.ID
		meta.Payload = protocol.GeneralRsp{}
	case driverRspRDMADrainDone:
		meta.Payload = protocol.RDMADrainRspToDriver{}
	case driverRspRDMARestartDone:
		meta.Payload = protocol.RDMARestartRspToDriver{}
	default:
		panic("unknown driver response kind " + kind)
	}
	m.toDriver().Send(meta)
	if kind == driverRspFlushDone {
		// Close the request opened by cpMiddleware.processFlushReq.
		m.comp.TaskTracker().EndReqIn(state.FlushTaskID)
		state.FlushTaskID = 0
		state.CurrFlushReq = messaging.Msg{Payload: protocol.FlushReq{}}
	}

	state.PendingDriverRsps = state.PendingDriverRsps[1:]

	return true
}

func (m *ctrlMiddleware) processReqFromDriver() bool {
	msg, ok := m.toDriver().PeekIncoming()
	if !ok {
		return false
	}

	switch msg.Payload.(type) {
	case protocol.RDMADrainCmdFromDriver:
		req := msg

		return m.processRDMADrainCmd(req)
	case protocol.RDMARestartCmdFromDriver:
		req := msg

		return m.processRDMARestartCmd(req)
	}

	return false
}

func (m *ctrlMiddleware) processRDMADrainCmd(
	cmd messaging.Msg,
) bool {
	if !m.toRDMA().CanSend() {
		return false
	}

	req := messaging.Msg{ID: m.comp.NewID(),
		Src: m.toRDMA().AsRemote(),
		Dst: m.comp.State.RDMA, Payload: rdma.DrainReq{}}

	m.toRDMA().Send(req)

	m.toDriver().RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processRDMARestartCmd(
	cmd messaging.Msg,
) bool {
	if !m.toRDMA().CanSend() {
		return false
	}

	req := messaging.Msg{ID: m.comp.NewID(),
		Src: m.toRDMA().AsRemote(),
		Dst: m.comp.State.RDMA, Payload: rdma.RestartReq{}}

	m.toRDMA().Send(req)

	m.toDriver().RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processRspFromRDMA() bool {
	msg, ok := m.toRDMA().PeekIncoming()
	if !ok {
		return false
	}

	state := &m.comp.State

	switch msg.Payload.(type) {
	case rdma.DrainRsp:
		state.PendingDriverRsps = append(
			state.PendingDriverRsps, driverRspRDMADrainDone)
	case rdma.RestartRsp:
		state.PendingDriverRsps = append(
			state.PendingDriverRsps, driverRspRDMARestartDone)
	default:
		panic("never")
	}

	m.toRDMA().RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processRspFromCaches() bool {
	return m.processCtrlRsp(m.toCaches())
}

func (m *ctrlMiddleware) processCtrlRsp(port messaging.Port) bool {
	msg, ok := port.PeekIncoming()
	if !ok {
		return false
	}

	rsp, ok := msg.Payload.(memcontrolprotocol.Rsp)
	if !ok {
		panic("never")
	}

	if !rsp.Success {
		panic(fmt.Sprintf(
			"control command %d to %s failed: %s",
			rsp.Command, msg.Src, rsp.Error))
	}

	port.RetrieveIncoming()
	m.ackReceived()

	return true
}

func (m *ctrlMiddleware) ackReceived() {
	state := &m.comp.State

	if state.PendingAcks == 0 {
		panic("received a control response while no ack is pending")
	}

	state.PendingAcks--
	if state.PendingAcks == 0 {
		m.advanceSeq()
	}
}

// startSeq starts a control sequence and runs its first step.
func (m *ctrlMiddleware) startSeq(seq string) {
	state := &m.comp.State

	state.CtrlSeq = seq
	state.CtrlStep = -1

	m.advanceSeq()
}

// advanceSeq runs the steps of the current sequence until a step expects
// responses, or until the sequence finishes.
func (m *ctrlMiddleware) advanceSeq() {
	state := &m.comp.State

	for state.CtrlSeq != ctrlSeqNone {
		state.CtrlStep++

		expectedAcks := m.execStep()
		if expectedAcks > 0 {
			state.PendingAcks = uint64(expectedAcks)
			return
		}
	}
}

func (m *ctrlMiddleware) execStep() int {
	state := &m.comp.State

	switch state.CtrlSeq {
	case ctrlSeqFlush:
		return m.execFlushStep(state.CtrlStep)
	case ctrlSeqKernelStart:
		return m.execKernelStartStep(state.CtrlStep)
	default:
		panic("unknown control sequence " + state.CtrlSeq)
	}
}

func (m *ctrlMiddleware) l1Caches() []messaging.RemotePort {
	state := &m.comp.State

	l1s := make([]messaging.RemotePort, 0,
		len(state.L1ICaches)+len(state.L1SCaches)+len(state.L1VCaches))
	l1s = append(l1s, state.L1ICaches...)
	l1s = append(l1s, state.L1SCaches...)
	l1s = append(l1s, state.L1VCaches...)

	return l1s
}

func (m *ctrlMiddleware) allCaches() []messaging.RemotePort {
	return append(m.l1Caches(), m.comp.State.L2Caches...)
}

// l1DataCaches returns the scalar and vector L1 caches, which can hold data
// written by an earlier kernel or copy. The instruction caches are left
// alone because code objects are not rewritten while they run.
func (m *ctrlMiddleware) l1DataCaches() []messaging.RemotePort {
	state := &m.comp.State

	l1s := make([]messaging.RemotePort, 0,
		len(state.L1SCaches)+len(state.L1VCaches))
	l1s = append(l1s, state.L1SCaches...)
	l1s = append(l1s, state.L1VCaches...)

	return l1s
}

// execKernelStartStep invalidates the L1 data caches before a kernel is
// dispatched. The L1s are write-through, so draining them leaves no dirty
// data and no Flush is needed. cpMiddleware dispatches the kernel once the
// sequence finishes.
func (m *ctrlMiddleware) execKernelStartStep(step int) int {
	state := &m.comp.State

	switch step {
	case 0:
		return m.enqueueCtrlReqs(&state.PendingCacheReqs, m.toCaches(),
			m.l1DataCaches(), memcontrolprotocol.CmdDrain, nil, 0)
	case 1:
		return m.enqueueCtrlReqs(&state.PendingCacheReqs, m.toCaches(),
			m.l1DataCaches(), memcontrolprotocol.CmdInvalidate, nil, 0)
	case 2:
		return m.enqueueCtrlReqs(&state.PendingCacheReqs, m.toCaches(),
			m.l1DataCaches(), memcontrolprotocol.CmdEnable, nil, 0)
	case 3:
		state.CtrlSeq = ctrlSeqNone
		return 0
	default:
		panic("invalid kernel start step")
	}
}

func (m *ctrlMiddleware) execFlushStep(step int) int {
	state := &m.comp.State

	switch step {
	case 0:
		return m.enqueueCtrlReqs(&state.PendingCacheReqs, m.toCaches(),
			m.l1Caches(), memcontrolprotocol.CmdDrain, nil, 0)
	case 1:
		return m.enqueueCtrlReqs(&state.PendingCacheReqs, m.toCaches(),
			m.l1Caches(), memcontrolprotocol.CmdFlush, nil, 0)
	case 2:
		return m.enqueueCtrlReqs(&state.PendingCacheReqs, m.toCaches(),
			m.l1Caches(), memcontrolprotocol.CmdInvalidate, nil, 0)
	case 3:
		return m.enqueueCtrlReqs(&state.PendingCacheReqs, m.toCaches(),
			state.L2Caches, memcontrolprotocol.CmdDrain, nil, 0)
	case 4:
		return m.enqueueCtrlReqs(&state.PendingCacheReqs, m.toCaches(),
			state.L2Caches, memcontrolprotocol.CmdFlush, nil, 0)
	case 5:
		return m.enqueueCtrlReqs(&state.PendingCacheReqs, m.toCaches(),
			state.L2Caches, memcontrolprotocol.CmdInvalidate, nil, 0)
	case 6:
		return m.enqueueCtrlReqs(&state.PendingCacheReqs, m.toCaches(),
			m.allCaches(), memcontrolprotocol.CmdEnable, nil, 0)
	case 7:
		state.PendingDriverRsps = append(
			state.PendingDriverRsps, driverRspFlushDone)
		state.CtrlSeq = ctrlSeqNone
		return 0
	default:
		panic("invalid flush step")
	}
}

func (m *ctrlMiddleware) enqueueCtrlReqs(
	queue *[]messaging.Msg,
	port messaging.Port,
	dsts []messaging.RemotePort,
	cmd memcontrolprotocol.Command,
	addresses []uint64,
	pid vm.PID,
) int {
	for _, dst := range dsts {
		req := messaging.Msg{ID: m.comp.NewID(),
			Src: port.AsRemote(),
			Dst: dst, Payload: memcontrolprotocol.Req{Command: cmd,
				Addresses: addresses,
				PID:       pid}}

		*queue = append(*queue, req)
	}

	return len(dsts)
}

func (m *ctrlMiddleware) Handle(timing.Event) bool { return m.Tick() }
