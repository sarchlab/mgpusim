package tlb

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type tlbMiddleware struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *tlbMiddleware) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *tlbMiddleware) bottomPort() messaging.Port {
	return m.comp.GetPortByName("Bottom")
}

func (m *tlbMiddleware) Tick() bool {
	madeProgress := false
	next := &m.comp.State

	switch next.TLBState {
	case tlbStateDrain:
		madeProgress = m.handleDrain() || madeProgress
	case tlbStatePause:
		return false
	default:
		madeProgress = m.handleEnable() || madeProgress
	}
	return madeProgress
}

func (m *tlbMiddleware) processPipeline() bool {
	madeProgress := false

	madeProgress = m.extractFromPipeline() || madeProgress
	madeProgress = m.tickPipeline() || madeProgress
	madeProgress = m.insertIntoPipeline() || madeProgress

	return madeProgress
}

func (m *tlbMiddleware) tickPipeline() bool {
	next := &m.comp.State
	return next.Pipeline.Tick(&next.BufferItems)
}

func (m *tlbMiddleware) insertIntoPipeline() bool {
	madeProgress := false
	spec := m.comp.Spec()
	next := &m.comp.State

	for i := 0; i < spec.NumReqPerCycle; i++ {
		if !next.Pipeline.CanAccept() {
			break
		}

		msgI := m.topPort().RetrieveIncoming()
		if msgI == nil {
			break
		}

		msg := msgI.(vmprotocol.TranslationReq)
		// Dwell one extra cycle at stage 0 to preserve the same per-stage
		// latency as the original hand-coded pipeline.
		next.Pipeline.AcceptWithDelay(pipelineTLBReqState{Msg: msg}, 1)

		madeProgress = true
	}

	return madeProgress
}

func (m *tlbMiddleware) extractFromPipeline() bool {
	madeProgress := false
	spec := m.comp.Spec()
	next := &m.comp.State

	for i := 0; i < spec.NumReqPerCycle; i++ {
		if next.BufferItems.Size() == 0 {
			break
		}

		item := next.BufferItems.Peek()
		msg := item.Msg

		ok := m.lookup(msg)
		if ok {
			next.BufferItems.Pop()
			madeProgress = true
		} else {
			break
		}
	}

	return madeProgress
}

func (m *tlbMiddleware) handleEnable() bool {
	madeProgress := false
	spec := m.comp.Spec()
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.respondMSHREntry() || madeProgress
	}

	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.parseBottom() || madeProgress
	}

	madeProgress = m.processPipeline() || madeProgress

	return madeProgress
}

func (m *tlbMiddleware) handleDrain() bool {
	madeProgress := false
	spec := m.comp.Spec()
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.respondMSHREntry() || madeProgress
	}

	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.parseBottom() || madeProgress
	}

	// Draining advances in-flight pipeline work but admits no new Top
	// requests (insertIntoPipeline), per the protocol's "Drain stops
	// accepting new traffic"; queued requests resume after Enable.
	madeProgress = m.extractFromPipeline() || madeProgress
	madeProgress = m.tickPipeline() || madeProgress

	next := &m.comp.State
	// Stay draining until the last fetched page has also been responded to
	// the top. parseBottom stages that response in RespondingMSHRData (and
	// empties MSHREntries) before respondMSHREntry can drain it, so pausing
	// on mshrIsEmpty alone would strand the final translation response.
	if mshrIsEmpty(next.MSHREntries) && !next.HasRespondingMSHR &&
		m.bottomPort().PeekIncoming() == nil {
		next.TLBState = tlbStatePause
	}

	return madeProgress
}

func (m *tlbMiddleware) respondMSHREntry() bool {
	next := &m.comp.State
	if !next.HasRespondingMSHR {
		return false
	}
	if len(next.RespondingMSHRData.Requests) == 0 {
		next.HasRespondingMSHR = false
		return false
	}
	mshrEntry := &next.RespondingMSHRData
	page := mshrEntry.Page
	reqMsg := mshrEntry.Requests[0]
	rspToTop := vmprotocol.TranslationRsp{
		Page: page,
	}
	rspToTop.ID = timing.GetIDGenerator().Generate()
	rspToTop.Src = m.topPort().AsRemote()
	rspToTop.Dst = reqMsg.Src
	rspToTop.RspTo = reqMsg.ID
	rspToTop.TrafficClass = "vmprotocol.TranslationRsp"

	if !m.topPort().CanSend() {
		return false
	}

	m.topPort().Send(rspToTop)

	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(&reqMsg, m.comp),
		Kind:   tracing.MilestoneKindNetworkBusy,
		What:   m.topPort().Name(),
	})

	mshrEntry.Requests = mshrEntry.Requests[1:]
	if len(mshrEntry.Requests) == 0 {
		next.HasRespondingMSHR = false
	}

	tracing.TraceReqComplete(m.comp, &reqMsg)

	return true
}

func (m *tlbMiddleware) lookup(msg vmprotocol.TranslationReq) bool {
	spec := m.comp.Spec()
	next := &m.comp.State

	_, found := mshrGetEntry(next.MSHREntries, msg.PID, msg.VAddr)
	if found {
		return m.processTLBMSHRHit(msg)
	}

	setID := vAddrToSetID(msg.VAddr, spec)
	wayID, page, setFound := setLookup(&next.Sets[setID], msg.PID, msg.VAddr)

	if setFound && page.Valid {
		return m.handleTranslationHit(msg, setID, wayID, page)
	}
	return m.handleTranslationMiss(msg)
}

func (m *tlbMiddleware) handleTranslationHit(
	msg vmprotocol.TranslationReq,
	setID, wayID int,
	page vm.Page,
) bool {
	ok := m.sendRspToTop(msg, page)
	if !ok {
		return false
	}

	next := &m.comp.State
	setVisit(&next.Sets[setID], wayID)

	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(msg, m.comp),
		Kind:   tracing.MilestoneKindData,
		What:   m.comp.Name() + ".Sets",
	})

	tracing.TraceReqReceive(m.comp, msg)
	tracing.AddTaskTag(m.comp, tracing.TaskTag{
		TaskID: tracing.MsgIDAtReceiver(msg, m.comp),
		What:   "hit",
	})
	tracing.TraceReqComplete(m.comp, msg)

	return true
}

func (m *tlbMiddleware) handleTranslationMiss(msg vmprotocol.TranslationReq) bool {
	next := &m.comp.State
	spec := m.comp.Spec()

	if mshrIsFull(next.MSHREntries, spec.MSHRSize) {
		return false
	}

	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(msg, m.comp),
		Kind:   tracing.MilestoneKindHardwareResource,
		What:   m.comp.Name() + ".MSHR",
	})

	fetched := m.fetchBottom(msg)
	if fetched {
		tracing.TraceReqReceive(m.comp, msg)
		tracing.AddTaskTag(m.comp, tracing.TaskTag{
			TaskID: tracing.MsgIDAtReceiver(msg, m.comp),
			What:   "miss",
		})

		return true
	}
	return false
}

func vAddrToSetID(vAddr uint64, spec Spec) (setID int) {
	return int(vAddr / spec.PageSize % uint64(spec.NumSets))
}

func (m *tlbMiddleware) sendRspToTop(
	msg vmprotocol.TranslationReq,
	page vm.Page,
) bool {
	rsp := vmprotocol.TranslationRsp{
		Page: page,
	}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = m.topPort().AsRemote()
	rsp.Dst = msg.Src
	rsp.RspTo = msg.ID
	rsp.TrafficClass = "vmprotocol.TranslationRsp"

	if !m.topPort().CanSend() {
		return false
	}

	m.topPort().Send(rsp)
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(msg, m.comp),
		Kind:   tracing.MilestoneKindNetworkBusy,
		What:   m.topPort().Name(),
	})
	return true
}

func (m *tlbMiddleware) processTLBMSHRHit(
	msg vmprotocol.TranslationReq,
) bool {
	next := &m.comp.State
	idx, found := mshrGetEntry(next.MSHREntries, msg.PID, msg.VAddr)
	if !found {
		return false
	}
	next.MSHREntries[idx].Requests = append(next.MSHREntries[idx].Requests, msg)

	tracing.TraceReqReceive(m.comp, msg)
	tracing.AddTaskTag(m.comp, tracing.TaskTag{
		TaskID: tracing.MsgIDAtReceiver(msg, m.comp),
		What:   "mshr-hit",
	})

	return true
}

func (m *tlbMiddleware) fetchBottom(msg vmprotocol.TranslationReq) bool {
	spec := m.comp.Spec()
	mapper := m.comp.Resources().TranslationProviderMapper

	fetchBottom := vmprotocol.TranslationReq{}
	fetchBottom.ID = timing.GetIDGenerator().Generate()
	fetchBottom.Src = m.bottomPort().AsRemote()
	fetchBottom.Dst = findTranslationPort(mapper, msg.VAddr)
	fetchBottom.PID = msg.PID
	fetchBottom.VAddr = msg.VAddr
	fetchBottom.DeviceID = msg.DeviceID
	fetchBottom.TrafficClass = "vmprotocol.TranslationReq"

	if !m.bottomPort().CanSend() {
		return false
	}

	m.bottomPort().Send(fetchBottom)

	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(msg, m.comp),
		Kind:   tracing.MilestoneKindNetworkBusy,
		What:   m.bottomPort().Name(),
	})

	next := &m.comp.State
	var idx int
	next.MSHREntries, idx = mshrAdd(next.MSHREntries, spec.MSHRSize, msg.PID, msg.VAddr)
	next.MSHREntries[idx].Requests = append(next.MSHREntries[idx].Requests, msg)
	next.MSHREntries[idx].HasReqToBottom = true
	next.MSHREntries[idx].ReqToBottom = fetchBottom

	tracing.TraceReqInitiate(m.comp, fetchBottom,
		tracing.MsgIDAtReceiver(msg, m.comp))

	return true
}

func (m *tlbMiddleware) parseBottom() bool {
	next := &m.comp.State
	if next.HasRespondingMSHR {
		return false
	}
	itemI := m.bottomPort().PeekIncoming()
	if itemI == nil {
		return false
	}

	item := itemI.(vmprotocol.TranslationRsp)
	spec := m.comp.Spec()
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(item, m.comp),
		Kind:   tracing.MilestoneKindData,
		What:   m.bottomPort().Name(),
	})
	page := item.Page

	mshrIdx, found := mshrGetEntry(next.MSHREntries, page.PID, page.VAddr)
	if !found || next.MSHREntries[mshrIdx].ReqToBottom.ID != item.RspTo {
		// Either no request is outstanding for this page, or this is a stale
		// response whose request was discarded by a Reset (the current MSHR
		// entry, if any, belongs to a newer request). Correlating by the
		// outstanding request's ID keeps a stale pre-reset translation from
		// filling the reset TLB and satisfying a fresh request.
		m.bottomPort().RetrieveIncoming()
		return true
	}

	setID := vAddrToSetID(page.VAddr, spec)
	wayID, ok := setEvict(&next.Sets[setID])

	if !ok {
		panic("failed to evict")
	}

	setUpdate(&next.Sets[setID], wayID, page)
	setVisit(&next.Sets[setID], wayID)

	next.HasRespondingMSHR = true
	next.RespondingMSHRData = next.MSHREntries[mshrIdx]
	next.RespondingMSHRData.Page = page

	// Get reqToBottom for tracing before removing
	reqToBottom := next.MSHREntries[mshrIdx].ReqToBottom

	next.MSHREntries = mshrRemove(next.MSHREntries, page.PID, page.VAddr)
	m.bottomPort().RetrieveIncoming()

	if next.RespondingMSHRData.HasReqToBottom {
		tracing.TraceReqFinalize(m.comp, &reqToBottom)
	}

	return true
}
