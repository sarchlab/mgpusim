package addresstranslator

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// respondPipelineMW handles translation responses and bottom-port responses.
type respondPipelineMW struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *respondPipelineMW) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *respondPipelineMW) bottomPort() messaging.Port {
	return m.comp.GetPortByName("Bottom")
}

func (m *respondPipelineMW) translationPort() messaging.Port {
	return m.comp.GetPortByName("Translation")
}

// Tick runs the respond pipeline: respond + parseTranslation. It is
// gated by ControlState — paused agents do not advance in-flight
// transactions; draining and enabled agents do, so a Drain can
// converge.
func (m *respondPipelineMW) Tick() bool {
	if m.comp.State.ControlState == memcontrolprotocol.StatePaused {
		return false
	}

	madeProgress := false

	spec := m.comp.Spec()

	for range spec.NumReqPerCycle {
		madeProgress = m.respond() || madeProgress
	}

	for range spec.NumReqPerCycle {
		madeProgress = m.parseTranslation() || madeProgress
	}

	return madeProgress
}

func (m *respondPipelineMW) parseTranslation() bool {
	// First, drain coalesced requests from transactions whose translation
	// already completed. Each call forwards at most one request.
	drained, blocked := m.drainCoalescedReq()
	if drained {
		return true
	}

	if blocked {
		return false
	}

	// Then, process new translation responses.
	return m.processTranslationRsp()
}

func (m *respondPipelineMW) processTranslationRsp() bool {
	rspI := m.translationPort().PeekIncoming()
	if rspI == nil {
		return false
	}

	rsp := rspI.(vmprotocol.TranslationRsp)
	nextState := &m.comp.State
	transIdx := findTransactionByReqID(nextState.Transactions, rsp.RspTo)

	if transIdx < 0 {
		m.translationPort().RetrieveIncoming()
		return true
	}

	nextTrans := &nextState.Transactions[transIdx]
	reqState := nextTrans.IncomingReqs[0]
	spec := m.comp.Spec()
	translatedReq := createTranslatedReq(reqState, rsp.Page,
		spec.Log2PageSize, m.bottomPort().AsRemote(),
		m.comp.Resources().MemProviderMapper)

	if !m.bottomPort().CanSend() {
		return false
	}

	m.bottomPort().Send(translatedReq)

	nextTrans = &nextState.Transactions[transIdx]
	nextTrans.TranslationDone = true
	nextTrans.Page = rsp.Page

	reqToBot := buildReqToBottom(reqState, translatedReq)
	nextState.InflightReqToBottom = append(
		nextState.InflightReqToBottom, reqToBot)
	nextTrans.IncomingReqs = nextTrans.IncomingReqs[1:]

	transReqTrace := vmprotocol.TranslationReq{
		MsgMeta: messaging.MsgMeta{
			ID:  nextTrans.TranslationReqID,
			Src: nextTrans.TranslationReqSrc,
			Dst: nextTrans.TranslationReqDst,
		},
	}

	if len(nextTrans.IncomingReqs) == 0 {
		removeTransaction(nextState, transIdx)
	}

	m.traceForwardedReq(reqState, translatedReq)
	tracing.TraceReqFinalize(m.comp, transReqTrace)

	m.translationPort().RetrieveIncoming()

	return true
}

// drainCoalescedReq forwards one queued request from a transaction whose
// translation already completed (the extra requests that were coalesced onto
// a single translation). It returns (drained, blocked): drained reports that
// a request was forwarded; blocked reports that a candidate exists but the
// Bottom port cannot accept it this cycle.
func (m *respondPipelineMW) drainCoalescedReq() (drained, blocked bool) {
	nextState := &m.comp.State
	spec := m.comp.Spec()

	for idx := range nextState.Transactions {
		trans := &nextState.Transactions[idx]
		if !trans.TranslationDone || len(trans.IncomingReqs) == 0 {
			continue
		}

		reqState := trans.IncomingReqs[0]
		translatedReq := createTranslatedReq(reqState, trans.Page,
			spec.Log2PageSize, m.bottomPort().AsRemote(),
			m.comp.Resources().MemProviderMapper)

		if !m.bottomPort().CanSend() {
			return false, true
		}

		m.bottomPort().Send(translatedReq)

		nextState.InflightReqToBottom = append(
			nextState.InflightReqToBottom,
			buildReqToBottom(reqState, translatedReq))
		trans.IncomingReqs = trans.IncomingReqs[1:]

		if len(trans.IncomingReqs) == 0 {
			removeTransaction(nextState, idx)
		}

		m.traceForwardedReq(reqState, translatedReq)

		return true, false
	}

	return false, false
}

// traceForwardedReq records tracing milestones for a request forwarded to the
// Bottom port and initiates the downstream request trace.
func (m *respondPipelineMW) traceForwardedReq(
	reqState incomingReqState,
	translatedReq messaging.Msg,
) {
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(translatedReq, m.comp),
		Kind:   tracing.MilestoneKindNetworkBusy,
		What:   m.bottomPort().Name(),
	})

	fakeFromTop := restoreMemMsg(reqState.ID, reqState.Src, reqState.Dst,
		reqState.RspTo, reqState.Type)

	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(fakeFromTop, m.comp),
		Kind:   tracing.MilestoneKindTranslation,
		What:   "translation",
	})

	tracing.TraceReqInitiate(m.comp, translatedReq,
		tracing.MsgIDAtReceiver(fakeFromTop, m.comp))
}

//nolint:funlen,gocyclo
func (m *respondPipelineMW) respond() bool {
	rspI := m.bottomPort().PeekIncoming()
	if rspI == nil {
		return false
	}

	nextState := &m.comp.State

	var (
		reqFromTopState reqToBottomState
		rspToTop        messaging.Msg
	)

	reqInBottom := false

	switch rsp := rspI.(type) {
	case memprotocol.DataReadyRsp:
		reqInBottom = isReqInBottomByID(nextState.InflightReqToBottom, rsp.RspTo)
		if reqInBottom {
			reqFromTopState = findReqToBottomByID(nextState.InflightReqToBottom, rsp.RspTo)
			rspToTop = memprotocol.DataReadyRsp{
				MsgMeta: messaging.MsgMeta{
					ID:           timing.GetIDGenerator().Generate(),
					Src:          m.topPort().AsRemote(),
					Dst:          reqFromTopState.ReqFromTopSrc,
					RspTo:        reqFromTopState.ReqFromTopID,
					TrafficBytes: len(rsp.Data) + 4,
					TrafficClass: "memprotocol.DataReadyRsp",
				},
				Data: rsp.Data,
			}

			fakeFromTop := restoreMemMsg(
				reqFromTopState.ReqFromTopID,
				reqFromTopState.ReqFromTopSrc,
				reqFromTopState.ReqFromTopDst,
				0, reqFromTopState.ReqFromTopType)
			tracing.AddMilestone(m.comp, tracing.Milestone{
				TaskID: tracing.MsgIDAtReceiver(fakeFromTop, m.comp),
				Kind:   tracing.MilestoneKindData,
				What:   "data",
			})
		}
	case memprotocol.WriteDoneRsp:
		reqInBottom = isReqInBottomByID(nextState.InflightReqToBottom, rsp.RspTo)
		if reqInBottom {
			reqFromTopState = findReqToBottomByID(nextState.InflightReqToBottom, rsp.RspTo)
			rspToTop = memprotocol.WriteDoneRsp{
				MsgMeta: messaging.MsgMeta{
					ID:           timing.GetIDGenerator().Generate(),
					Src:          m.topPort().AsRemote(),
					Dst:          reqFromTopState.ReqFromTopSrc,
					RspTo:        reqFromTopState.ReqFromTopID,
					TrafficBytes: 4,
					TrafficClass: "memprotocol.WriteDoneRsp",
				},
			}

			fakeFromTop := restoreMemMsg(
				reqFromTopState.ReqFromTopID,
				reqFromTopState.ReqFromTopSrc,
				reqFromTopState.ReqFromTopDst,
				0, reqFromTopState.ReqFromTopType)
			tracing.AddMilestone(m.comp, tracing.Milestone{
				TaskID: tracing.MsgIDAtReceiver(fakeFromTop, m.comp),
				Kind:   tracing.MilestoneKindSubTask,
				What:   "subtask",
			})
		}
	default:
		log.Panicf("cannot handle respond of type %s", fmt.Sprintf("%T", rspI))
	}

	if reqInBottom {
		if !m.topPort().CanSend() {
			return false
		}

		m.topPort().Send(rspToTop)

		fakeFromTop := restoreMemMsg(
			reqFromTopState.ReqFromTopID,
			reqFromTopState.ReqFromTopSrc,
			reqFromTopState.ReqFromTopDst,
			0, reqFromTopState.ReqFromTopType)

		tracing.AddMilestone(m.comp, tracing.Milestone{
			TaskID: tracing.MsgIDAtReceiver(fakeFromTop, m.comp),
			Kind:   tracing.MilestoneKindNetworkBusy,
			What:   m.topPort().Name(),
		})

		rspMeta := rspI.Meta()
		removeReqToBottomByID(nextState, rspMeta.RspTo)

		fakeReqToBottom := restoreMemMsg(
			reqFromTopState.ReqToBottomID,
			reqFromTopState.ReqToBottomSrc,
			reqFromTopState.ReqToBottomDst,
			0, reqFromTopState.ReqToBottomType)
		tracing.TraceReqFinalize(m.comp, fakeReqToBottom)
		tracing.TraceReqComplete(m.comp, fakeFromTop)
	}

	m.bottomPort().RetrieveIncoming()

	return true
}
