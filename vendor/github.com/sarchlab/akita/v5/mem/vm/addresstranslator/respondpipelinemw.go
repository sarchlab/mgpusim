package addresstranslator

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"

	// respondPipelineMW handles translation responses and bottom-port responses.
	"github.com/sarchlab/akita/v5/messaging"
)

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

	if len(nextTrans.IncomingReqs) == 0 {
		removeTransaction(nextState, transIdx)
	}

	m.traceTranslationComplete(nextTrans, reqState, translatedReq)

	m.translationPort().RetrieveIncoming()

	return true
}

// traceTranslationComplete records tracing milestones for a completed
// translation and initiates the downstream request trace.
func (m *respondPipelineMW) traceTranslationComplete(
	trans *transactionState,
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

	fakeTransReq := vmprotocol.TranslationReq{
		MsgMeta: messaging.MsgMeta{
			ID:  trans.TranslationReqID,
			Src: trans.TranslationReqSrc,
			Dst: trans.TranslationReqDst,
		},
	}
	tracing.TraceReqFinalize(m.comp, fakeTransReq)
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
