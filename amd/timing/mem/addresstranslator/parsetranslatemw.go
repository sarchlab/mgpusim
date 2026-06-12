package addresstranslator

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// parseTranslateMW handles incoming requests from topPort and initiates
// address translation. Control-port handling lives in ctrlMiddleware.
//
// Unlike akita's address translator, this variant coalesces TLB requests:
// when a request targets a virtual page for which a translation is already
// pending (same page ID and PID), the request is appended to the existing
// transaction instead of issuing a second translation request.
type parseTranslateMW struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *parseTranslateMW) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *parseTranslateMW) translationPort() messaging.Port {
	return m.comp.GetPortByName("Translation")
}

// Tick runs translate while the component is enabled. Pause, Drain, and
// Reset all stop new translation work; in-flight transactions continue
// to drain through respondPipelineMW until the component is fully
// paused.
func (m *parseTranslateMW) Tick() bool {
	madeProgress := false

	if m.comp.State.ControlState == memcontrolprotocol.StateEnabled {
		spec := m.comp.Spec()
		for range spec.NumReqPerCycle {
			madeProgress = m.translate() || madeProgress
		}
	}

	return madeProgress
}

func (m *parseTranslateMW) translate() bool {
	itemI := m.topPort().PeekIncoming()
	if itemI == nil {
		return false
	}

	item := itemI.(memprotocol.AccessReq)
	vAddr := item.GetAddress()
	spec := m.comp.Spec()
	vPageID := addrToPageID(vAddr, spec.Log2PageSize)

	if m.coalesce(itemI, vPageID, item.GetPID()) {
		return true
	}

	transReq := vmprotocol.TranslationReq{}
	transReq.ID = timing.GetIDGenerator().Generate()
	transReq.Src = m.translationPort().AsRemote()
	transReq.Dst = m.comp.Resources().TranslationProviderMapper.Find(vAddr)
	transReq.PID = item.GetPID()
	transReq.VAddr = vPageID
	transReq.DeviceID = spec.DeviceID
	transReq.TrafficClass = "vmprotocol.TranslationReq"

	if !m.translationPort().CanSend() {
		return false
	}

	m.translationPort().Send(transReq)

	incoming := msgToIncomingReqState(itemI)

	nextState := &m.comp.State

	tracing.TraceReqReceive(m.comp, itemI)
	tracing.TraceReqInitiate(
		m.comp,
		transReq,
		tracing.MsgIDAtReceiver(itemI, m.comp),
	)

	trans := transactionState{
		IncomingReqs:      []incomingReqState{incoming},
		TranslationReqID:  transReq.ID,
		TranslationReqSrc: transReq.Src,
		TranslationReqDst: transReq.Dst,
		TranslationVAddr:  vPageID,
		TranslationPID:    transReq.PID,
	}
	nextState.Transactions = append(nextState.Transactions, trans)

	m.topPort().RetrieveIncoming()

	return true
}

// coalesce checks whether a translation for the same virtual page and PID is
// already pending. If so, the incoming request is appended to that
// transaction and no new translation request is sent.
func (m *parseTranslateMW) coalesce(
	msg messaging.Msg,
	vPageID uint64,
	pid vm.PID,
) bool {
	nextState := &m.comp.State

	for i := range nextState.Transactions {
		t := &nextState.Transactions[i]
		if !t.TranslationDone &&
			t.TranslationVAddr == vPageID &&
			t.TranslationPID == pid {
			t.IncomingReqs = append(
				t.IncomingReqs, msgToIncomingReqState(msg))

			tracing.TraceReqReceive(m.comp, msg)
			m.topPort().RetrieveIncoming()

			return true
		}
	}

	return false
}
