package cp

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/rdma"
)

// ctrlMiddleware implements the control plane of the Command Processor: cache
// flushing, TLB shootdown, GPU restart, and RDMA drain/restart.
//
// # Control-verb mapping (Akita v4 -> v5 memcontrolprotocol)
//
// The v4 per-component flush/restart messages are replaced by the unified
// memcontrolprotocol verbs, sent to each component's "Control" port. The v4
// compound semantics map to verb sequences as follows (see
// akita/mem/CONTROL_PROTOCOL.md):
//
//   - Driver flush (v4 cache.FlushReq with no flags): per cache level,
//     CmdDrain -> CmdFlush -> CmdInvalidate, then CmdEnable for all caches.
//     Drain (rather than Pause) is needed because CmdFlush only starts once
//     no transaction is in flight. L1 caches are drained/flushed before L2
//     caches so that in-flight L1 misses can still complete in the L2. The
//     CmdInvalidate is required for correctness: v4's flush unconditionally
//     reset the cache directory (writeback flusher.go and writethrough
//     controlstage.go both call directory.Reset() on every flush), so a plain
//     v4 flush dropped all cache lines, not just wrote back dirty data.
//     Without invalidation a kernel that reads a buffer rewritten by a host
//     MemCopy (e.g. the per-iteration cluster buffer in kmeans) hits stale
//     cache lines and computes incorrect results.
//   - Shootdown cache flush (v4 cache.FlushReq with PauseAfterFlushing +
//     DiscardInflight + InvalidateAllCacheLines): per cache level,
//     CmdDrain -> CmdFlush -> CmdInvalidate, and the cache stays paused.
//     v4 discarded in-flight transactions; v5 drains them instead (CmdFlush
//     requires a quiescent cache, and CmdPause would freeze in-flight
//     transactions, deadlocking the flush). Dirty data is written back
//     before the lines are invalidated, preserving the v4 intent of the
//     shootdown (memory must hold the data before pages migrate).
//   - Cache restart (v4 cache.RestartReq, which discarded in-flight work and
//     resumed): CmdReset (hard reset; legal from paused; lands the cache
//     enabled).
//   - TLB shootdown (v4 tlb.FlushReq{VAddrs, PID}, which invalidated the
//     matching entries and paused the TLB): CmdPause -> CmdInvalidate
//     {Addresses, PID}; the TLB stays paused.
//   - TLB restart (v4 tlb.RestartReq): CmdEnable (cached translations
//     outside the shootdown filter stay valid, as in v4).
//   - Address translator flush (v4 mem.ControlMsg{DiscardTransactions}):
//     CmdPause -> CmdReset. Restart (v4 mem.ControlMsg{Restart}): CmdEnable.
//   - ROB flush (v4 mem.ControlMsg{DiscardTransactions}): CmdPause. Restart:
//     CmdReset (discards the transactions that were frozen at flush time).
//   - DRAM controllers: no verbs are sent. The v4 CP only commanded DRAM
//     during page migration, which has been dropped.
//   - CU pipelines: mgpusim-internal protocol.CUPipelineFlushReq /
//     CUPipelineRestartReq value messages (unchanged flow).
//   - RDMA: mgpusim-internal rdma.DrainReq / rdma.RestartReq value messages
//     (unchanged flow).
//
// # Sequences
//
// Flush (driver protocol.FlushReq -> protocol.GeneralRsp):
//
//	Drain L1s -> Flush L1s -> Invalidate L1s ->
//	Drain L2s -> Flush L2s -> Invalidate L2s -> Enable all caches.
//
// Shootdown (driver protocol.ShootDownCommand ->
// protocol.ShootDownCompleteRsp):
//
//	Flush CU pipelines -> Pause ATs+ROBs -> Reset ATs ->
//	Drain/Flush/Invalidate L1s -> Drain/Flush/Invalidate L2s ->
//	Pause TLBs -> Invalidate TLBs (filtered by VAddr/PID).
//
// Restart (driver protocol.GPURestartReq -> protocol.GPURestartRsp):
//
//	Reset caches -> Enable TLBs -> Enable ATs + Reset ROBs ->
//	Restart CU pipelines.
//
// Each step fans out to every component of the class and waits for all
// responses (State.PendingAcks) before the next step starts.
type ctrlMiddleware struct {
	comp *Comp
}

func (m *ctrlMiddleware) toDriver() messaging.Port {
	return m.comp.GetPortByName("ToDriver")
}

func (m *ctrlMiddleware) toCUs() messaging.Port {
	return m.comp.GetPortByName("ToCUs")
}

func (m *ctrlMiddleware) toTLBs() messaging.Port {
	return m.comp.GetPortByName("ToTLBs")
}

func (m *ctrlMiddleware) toATs() messaging.Port {
	return m.comp.GetPortByName("ToAddressTranslators")
}

func (m *ctrlMiddleware) toCaches() messaging.Port {
	return m.comp.GetPortByName("ToCaches")
}

func (m *ctrlMiddleware) toRDMA() messaging.Port {
	return m.comp.GetPortByName("ToRDMA")
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false

	madeProgress = m.sendPendingReqs() || madeProgress
	madeProgress = m.sendPendingDriverRsps() || madeProgress
	madeProgress = m.processReqFromDriver() || madeProgress
	madeProgress = m.processRspFromRDMA() || madeProgress
	madeProgress = m.processRspFromCUs() || madeProgress
	madeProgress = m.processRspFromATs() || madeProgress
	madeProgress = m.processRspFromCaches() || madeProgress
	madeProgress = m.processRspFromTLBs() || madeProgress

	return madeProgress
}

// sendPendingReqs drains the outbound control-message queues as the ports
// become available.
func (m *ctrlMiddleware) sendPendingReqs() bool {
	state := &m.comp.State
	madeProgress := false

	madeProgress = drainCtrlQueue(
		m.toCaches(), &state.PendingCacheReqs) || madeProgress
	madeProgress = drainCtrlQueue(
		m.toTLBs(), &state.PendingTLBReqs) || madeProgress
	madeProgress = drainCtrlQueue(
		m.toATs(), &state.PendingATReqs) || madeProgress

	for len(state.PendingCUFlushReqs) > 0 && m.toCUs().CanSend() {
		m.toCUs().Send(state.PendingCUFlushReqs[0])
		state.PendingCUFlushReqs = state.PendingCUFlushReqs[1:]
		madeProgress = true
	}

	for len(state.PendingCURestartReqs) > 0 && m.toCUs().CanSend() {
		m.toCUs().Send(state.PendingCURestartReqs[0])
		state.PendingCURestartReqs = state.PendingCURestartReqs[1:]
		madeProgress = true
	}

	return madeProgress
}

func drainCtrlQueue(
	port messaging.Port,
	queue *[]memcontrolprotocol.Req,
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
	meta := messaging.MsgMeta{
		ID:  timing.GetIDGenerator().Generate(),
		Src: m.toDriver().AsRemote(),
		Dst: state.Driver,
	}

	switch kind {
	case driverRspFlushDone:
		meta.Dst = state.CurrFlushReq.Src
		meta.RspTo = state.CurrFlushReq.ID
		m.toDriver().Send(protocol.GeneralRsp{MsgMeta: meta})
		state.CurrFlushReq = protocol.FlushReq{}
	case driverRspShootdownDone:
		m.toDriver().Send(protocol.ShootDownCompleteRsp{MsgMeta: meta})
		state.ShootDownInProcess = false
	case driverRspRestartDone:
		m.toDriver().Send(protocol.GPURestartRsp{MsgMeta: meta})
	case driverRspRDMADrainDone:
		m.toDriver().Send(protocol.RDMADrainRspToDriver{MsgMeta: meta})
	case driverRspRDMARestartDone:
		m.toDriver().Send(protocol.RDMARestartRspToDriver{MsgMeta: meta})
	default:
		panic("unknown driver response kind " + kind)
	}

	state.PendingDriverRsps = state.PendingDriverRsps[1:]

	return true
}

func (m *ctrlMiddleware) processReqFromDriver() bool {
	msg := m.toDriver().PeekIncoming()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case protocol.RDMADrainCmdFromDriver:
		return m.processRDMADrainCmd(req)
	case protocol.RDMARestartCmdFromDriver:
		return m.processRDMARestartCmd(req)
	case protocol.ShootDownCommand:
		return m.processShootdownCommand(req)
	case protocol.GPURestartReq:
		return m.processGPURestartReq(req)
	}

	return false
}

func (m *ctrlMiddleware) processRDMADrainCmd(
	cmd protocol.RDMADrainCmdFromDriver,
) bool {
	if !m.toRDMA().CanSend() {
		return false
	}

	req := rdma.DrainReq{
		MsgMeta: messaging.MsgMeta{
			ID:  timing.GetIDGenerator().Generate(),
			Src: m.toRDMA().AsRemote(),
			Dst: m.comp.State.RDMA,
		},
	}
	m.toRDMA().Send(req)

	m.toDriver().RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processRDMARestartCmd(
	cmd protocol.RDMARestartCmdFromDriver,
) bool {
	if !m.toRDMA().CanSend() {
		return false
	}

	req := rdma.RestartReq{
		MsgMeta: messaging.MsgMeta{
			ID:  timing.GetIDGenerator().Generate(),
			Src: m.toRDMA().AsRemote(),
			Dst: m.comp.State.RDMA,
		},
	}
	m.toRDMA().Send(req)

	m.toDriver().RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processShootdownCommand(
	cmd protocol.ShootDownCommand,
) bool {
	state := &m.comp.State

	if state.ShootDownInProcess || state.CtrlSeq != ctrlSeqNone {
		return false
	}

	state.CurrShootdown = cmd
	state.ShootDownInProcess = true
	m.startSeq(ctrlSeqShootdown)

	m.toDriver().RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processGPURestartReq(
	cmd protocol.GPURestartReq,
) bool {
	state := &m.comp.State

	if state.CtrlSeq != ctrlSeqNone {
		return false
	}

	m.startSeq(ctrlSeqRestart)

	m.toDriver().RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processRspFromRDMA() bool {
	msg := m.toRDMA().PeekIncoming()
	if msg == nil {
		return false
	}

	state := &m.comp.State

	switch msg.(type) {
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

func (m *ctrlMiddleware) processRspFromCUs() bool {
	msg := m.toCUs().PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg.(type) {
	case protocol.CUPipelineFlushRsp, protocol.CUPipelineRestartRsp:
		m.toCUs().RetrieveIncoming()
		m.ackReceived()
		return true
	}

	// Other messages (e.g., WGCompletionMsg) are handled by the dispatchers.
	return false
}

func (m *ctrlMiddleware) processRspFromCaches() bool {
	return m.processCtrlRsp(m.toCaches())
}

func (m *ctrlMiddleware) processRspFromTLBs() bool {
	return m.processCtrlRsp(m.toTLBs())
}

func (m *ctrlMiddleware) processRspFromATs() bool {
	return m.processCtrlRsp(m.toATs())
}

func (m *ctrlMiddleware) processCtrlRsp(port messaging.Port) bool {
	msg := port.PeekIncoming()
	if msg == nil {
		return false
	}

	rsp, ok := msg.(memcontrolprotocol.Rsp)
	if !ok {
		panic("never")
	}

	if !rsp.Success {
		panic(fmt.Sprintf(
			"control command %d to %s failed: %s",
			rsp.Command, rsp.Src, rsp.Error))
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
	case ctrlSeqShootdown:
		return m.execShootdownStep(state.CtrlStep)
	case ctrlSeqRestart:
		return m.execRestartStep(state.CtrlStep)
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

func (m *ctrlMiddleware) execShootdownStep(step int) int {
	state := &m.comp.State

	switch step {
	case 0:
		return m.enqueueCUFlushReqs()
	case 1:
		acks := m.enqueueCtrlReqs(&state.PendingATReqs, m.toATs(),
			state.AddressTranslators, memcontrolprotocol.CmdPause, nil, 0)
		acks += m.enqueueCtrlReqs(&state.PendingATReqs, m.toATs(),
			state.ROBs, memcontrolprotocol.CmdPause, nil, 0)
		return acks
	case 2:
		return m.enqueueCtrlReqs(&state.PendingATReqs, m.toATs(),
			state.AddressTranslators, memcontrolprotocol.CmdReset, nil, 0)
	case 3:
		return m.enqueueCtrlReqs(&state.PendingCacheReqs, m.toCaches(),
			m.l1Caches(), memcontrolprotocol.CmdDrain, nil, 0)
	case 4:
		return m.enqueueCtrlReqs(&state.PendingCacheReqs, m.toCaches(),
			m.l1Caches(), memcontrolprotocol.CmdFlush, nil, 0)
	case 5:
		return m.enqueueCtrlReqs(&state.PendingCacheReqs, m.toCaches(),
			m.l1Caches(), memcontrolprotocol.CmdInvalidate, nil, 0)
	case 6:
		return m.enqueueCtrlReqs(&state.PendingCacheReqs, m.toCaches(),
			state.L2Caches, memcontrolprotocol.CmdDrain, nil, 0)
	case 7:
		return m.enqueueCtrlReqs(&state.PendingCacheReqs, m.toCaches(),
			state.L2Caches, memcontrolprotocol.CmdFlush, nil, 0)
	case 8:
		return m.enqueueCtrlReqs(&state.PendingCacheReqs, m.toCaches(),
			state.L2Caches, memcontrolprotocol.CmdInvalidate, nil, 0)
	case 9:
		return m.enqueueCtrlReqs(&state.PendingTLBReqs, m.toTLBs(),
			state.TLBs, memcontrolprotocol.CmdPause, nil, 0)
	case 10:
		return m.enqueueCtrlReqs(&state.PendingTLBReqs, m.toTLBs(),
			state.TLBs, memcontrolprotocol.CmdInvalidate,
			state.CurrShootdown.VAddr, state.CurrShootdown.PID)
	case 11:
		state.PendingDriverRsps = append(
			state.PendingDriverRsps, driverRspShootdownDone)
		state.CtrlSeq = ctrlSeqNone
		return 0
	default:
		panic("invalid shootdown step")
	}
}

func (m *ctrlMiddleware) execRestartStep(step int) int {
	state := &m.comp.State

	switch step {
	case 0:
		return m.enqueueCtrlReqs(&state.PendingCacheReqs, m.toCaches(),
			m.allCaches(), memcontrolprotocol.CmdReset, nil, 0)
	case 1:
		return m.enqueueCtrlReqs(&state.PendingTLBReqs, m.toTLBs(),
			state.TLBs, memcontrolprotocol.CmdEnable, nil, 0)
	case 2:
		acks := m.enqueueCtrlReqs(&state.PendingATReqs, m.toATs(),
			state.AddressTranslators, memcontrolprotocol.CmdEnable, nil, 0)
		acks += m.enqueueCtrlReqs(&state.PendingATReqs, m.toATs(),
			state.ROBs, memcontrolprotocol.CmdReset, nil, 0)
		return acks
	case 3:
		return m.enqueueCURestartReqs()
	case 4:
		state.PendingDriverRsps = append(
			state.PendingDriverRsps, driverRspRestartDone)
		state.CtrlSeq = ctrlSeqNone
		return 0
	default:
		panic("invalid restart step")
	}
}

func (m *ctrlMiddleware) enqueueCtrlReqs(
	queue *[]memcontrolprotocol.Req,
	port messaging.Port,
	dsts []messaging.RemotePort,
	cmd memcontrolprotocol.Command,
	addresses []uint64,
	pid vm.PID,
) int {
	for _, dst := range dsts {
		req := memcontrolprotocol.Req{
			MsgMeta: messaging.MsgMeta{
				ID:           timing.GetIDGenerator().Generate(),
				Src:          port.AsRemote(),
				Dst:          dst,
				TrafficClass: "memcontrolprotocol.Req",
			},
			Command:   cmd,
			Addresses: addresses,
			PID:       pid,
		}
		*queue = append(*queue, req)
	}

	return len(dsts)
}

func (m *ctrlMiddleware) enqueueCUFlushReqs() int {
	state := &m.comp.State

	for _, cu := range state.CUs {
		req := protocol.CUPipelineFlushReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: m.toCUs().AsRemote(),
				Dst: cu,
			},
		}
		state.PendingCUFlushReqs = append(state.PendingCUFlushReqs, req)
	}

	return len(state.CUs)
}

func (m *ctrlMiddleware) enqueueCURestartReqs() int {
	state := &m.comp.State

	for _, cu := range state.CUs {
		req := protocol.CUPipelineRestartReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: m.toCUs().AsRemote(),
				Dst: cu,
			},
		}
		state.PendingCURestartReqs = append(state.PendingCURestartReqs, req)
	}

	return len(state.CUs)
}
