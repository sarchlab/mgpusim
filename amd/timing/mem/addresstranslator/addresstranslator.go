package addresstranslator

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

type transaction struct {
	incomingReqs    []mem.AccessReq
	translationReq  *vm.TranslationReq
	translationRsp  *vm.TranslationRsp
	translationDone bool
}

type reqToBottom struct {
	reqFromTop  mem.AccessReq
	reqToBottom mem.AccessReq
}

// Comp is an AddressTranslator that forwards the read/write requests with
// the address translated from virtual to physical.
type Comp struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	topPort         sim.Port
	bottomPort      sim.Port
	translationPort sim.Port
	ctrlPort        sim.Port

	log2PageSize          uint64
	deviceID              uint64
	numReqPerCycle        int
	memoryPortMapper      mem.AddressToPortMapper
	translationPortMapper mem.AddressToPortMapper

	isFlushing bool

	transactions        []*transaction
	inflightReqToBottom []reqToBottom
}

func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

type middleware struct {
	*Comp
}

// Tick updates state at each cycle.
func (m *middleware) Tick() bool {
	madeProgress := false

	if !m.isFlushing {
		madeProgress = m.runPipeline()
	} else {
		for i := 0; i < m.numReqPerCycle; i++ {
			madeProgress = m.parseTranslation() || madeProgress
		}
	}

	madeProgress = m.handleCtrlRequest() || madeProgress

	return madeProgress
}

func (m *middleware) runPipeline() bool {
	madeProgress := false

	for i := 0; i < m.numReqPerCycle; i++ {
		madeProgress = m.respond() || madeProgress
	}

	for i := 0; i < m.numReqPerCycle; i++ {
		madeProgress = m.parseTranslation() || madeProgress
	}

	for i := 0; i < m.numReqPerCycle; i++ {
		madeProgress = m.translate() || madeProgress
	}

	return madeProgress
}

func (m *middleware) translate() bool {
	item := m.topPort.PeekIncoming()
	if item == nil {
		return false
	}

	req := item.(mem.AccessReq)
	vAddr := req.GetAddress()
	vPageID := m.addrToPageID(vAddr)

	// TLB request coalescing: check if there's already a pending
	// translation for the same virtual page and PID
	for _, t := range m.transactions {
		if !t.translationDone &&
			t.translationReq != nil &&
			m.addrToPageID(t.translationReq.VAddr) == vPageID &&
			t.incomingReqs[0].GetPID() == req.GetPID() {
			// Coalesce: append to existing transaction
			t.incomingReqs = append(t.incomingReqs, req)
			tracing.TraceReqReceive(req, m.Comp)
			m.topPort.RetrieveIncoming()
			return true
		}
	}

	// No coalescing opportunity — create new translation
	transReq := vm.TranslationReqBuilder{}.
		WithSrc(m.translationPort.AsRemote()).
		WithDst(m.translationPortMapper.Find(vAddr)).
		WithPID(req.GetPID()).
		WithVAddr(vPageID).
		WithDeviceID(m.deviceID).
		Build()

	err := m.translationPort.Send(transReq)
	if err != nil {
		return false
	}

	translation := &transaction{
		incomingReqs:   []mem.AccessReq{req},
		translationReq: transReq,
	}
	m.transactions = append(m.transactions, translation)

	tracing.TraceReqReceive(req, m.Comp)
	tracing.TraceReqInitiate(
		transReq,
		m.Comp,
		tracing.MsgIDAtReceiver(req, m.Comp),
	)

	m.topPort.RetrieveIncoming()

	return true
}

//nolint:gocyclo,cyclop,funlen
func (m *middleware) parseTranslation() bool {
	// First, try to drain coalesced requests from completed transactions
	for _, t := range m.transactions {
		if t.translationDone && len(t.incomingReqs) > 0 {
			reqFromTop := t.incomingReqs[0]
			translatedReq := m.createTranslatedReq(
				reqFromTop, t.translationRsp.Page)

			err := m.bottomPort.Send(translatedReq)
			if err != nil {
				return false
			}

			tracing.AddMilestone(
				tracing.MsgIDAtReceiver(translatedReq, m.Comp),
				tracing.MilestoneKindNetworkBusy,
				m.bottomPort.Name(),
				m.Comp.Name(),
				m.Comp,
			)

			m.inflightReqToBottom = append(m.inflightReqToBottom,
				reqToBottom{
					reqFromTop:  reqFromTop,
					reqToBottom: translatedReq,
				})
			t.incomingReqs = t.incomingReqs[1:]

			if len(t.incomingReqs) == 0 {
				m.removeExistingTranslation(t)
			}

			tracing.AddMilestone(
				tracing.MsgIDAtReceiver(reqFromTop, m.Comp),
				tracing.MilestoneKindTranslation,
				"translation",
				m.Comp.Name(),
				m.Comp,
			)

			tracing.TraceReqInitiate(translatedReq, m.Comp,
				tracing.MsgIDAtReceiver(reqFromTop, m.Comp))

			return true
		}
	}

	// Then, process new translation responses
	rsp := m.translationPort.PeekIncoming()
	if rsp == nil {
		return false
	}

	transRsp := rsp.(*vm.TranslationRsp)
	transaction := m.findTranslationByReqID(transRsp.RespondTo)

	if transaction == nil {
		m.translationPort.RetrieveIncoming()
		return true
	}

	transaction.translationRsp = transRsp
	transaction.translationDone = true

	reqFromTop := transaction.incomingReqs[0]
	translatedReq := m.createTranslatedReq(
		reqFromTop, transaction.translationRsp.Page)

	err := m.bottomPort.Send(translatedReq)
	if err != nil {
		return false
	}

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(translatedReq, m.Comp),
		tracing.MilestoneKindNetworkBusy,
		m.bottomPort.Name(),
		m.Comp.Name(),
		m.Comp,
	)

	m.inflightReqToBottom = append(m.inflightReqToBottom,
		reqToBottom{
			reqFromTop:  reqFromTop,
			reqToBottom: translatedReq,
		})
	transaction.incomingReqs = transaction.incomingReqs[1:]

	if len(transaction.incomingReqs) == 0 {
		m.removeExistingTranslation(transaction)
	}

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(reqFromTop, m.Comp),
		tracing.MilestoneKindTranslation,
		"translation",
		m.Comp.Name(),
		m.Comp,
	)

	m.translationPort.RetrieveIncoming()

	tracing.TraceReqFinalize(transaction.translationReq, m.Comp)
	tracing.TraceReqInitiate(translatedReq, m.Comp,
		tracing.MsgIDAtReceiver(reqFromTop, m.Comp))

	return true
}

//nolint:funlen,gocyclo
func (m *middleware) respond() bool {
	rsp := m.bottomPort.PeekIncoming()
	if rsp == nil {
		return false
	}

	var (
		reqFromTop       mem.AccessReq
		reqToBottomCombo reqToBottom
		rspToTop         mem.AccessRsp
	)

	reqInBottom := false

	switch rsp := rsp.(type) {
	case *mem.DataReadyRsp:
		reqInBottom = m.isReqInBottomByID(rsp.RespondTo)
		if reqInBottom {
			reqToBottomCombo = m.findReqToBottomByID(rsp.RespondTo)
			reqFromTop = reqToBottomCombo.reqFromTop
			drToTop := mem.DataReadyRspBuilder{}.
				WithSrc(m.topPort.AsRemote()).
				WithDst(reqFromTop.Meta().Src).
				WithRspTo(reqFromTop.Meta().ID).
				WithData(rsp.Data).
				Build()
			rspToTop = drToTop
			tracing.AddMilestone(
				tracing.MsgIDAtReceiver(reqFromTop, m.Comp),
				tracing.MilestoneKindData,
				"data",
				m.Comp.Name(),
				m.Comp,
			)
		}
	case *mem.WriteDoneRsp:
		reqInBottom = m.isReqInBottomByID(rsp.RespondTo)
		if reqInBottom {
			reqToBottomCombo = m.findReqToBottomByID(rsp.RespondTo)
			reqFromTop = reqToBottomCombo.reqFromTop
			rspToTop = mem.WriteDoneRspBuilder{}.
				WithSrc(m.topPort.AsRemote()).
				WithDst(reqFromTop.Meta().Src).
				WithRspTo(reqFromTop.Meta().ID).
				Build()
			tracing.AddMilestone(
				tracing.MsgIDAtReceiver(reqFromTop, m.Comp),
				tracing.MilestoneKindSubTask,
				"subtask",
				m.Comp.Name(),
				m.Comp,
			)
		}
	default:
		log.Panicf("cannot handle respond of type %s", reflect.TypeOf(rsp))
	}

	if reqInBottom {
		err := m.topPort.Send(rspToTop)
		if err != nil {
			return false
		}

		tracing.AddMilestone(
			tracing.MsgIDAtReceiver(reqFromTop, m.Comp),
			tracing.MilestoneKindNetworkBusy,
			m.topPort.Name(),
			m.Comp.Name(),
			m.Comp,
		)

		m.removeReqToBottomByID(rsp.(mem.AccessRsp).GetRspTo())

		tracing.TraceReqFinalize(reqToBottomCombo.reqToBottom, m.Comp)
		tracing.TraceReqComplete(reqToBottomCombo.reqFromTop, m.Comp)
	}

	m.bottomPort.RetrieveIncoming()

	return true
}

func (m *middleware) createTranslatedReq(
	req mem.AccessReq,
	page vm.Page,
) mem.AccessReq {
	switch req := req.(type) {
	case *mem.ReadReq:
		return m.createTranslatedReadReq(req, page)
	case *mem.WriteReq:
		return m.createTranslatedWriteReq(req, page)
	default:
		log.Panicf("cannot translate request of type %s", reflect.TypeOf(req))
		return nil
	}
}

func (m *middleware) createTranslatedReadReq(
	req *mem.ReadReq,
	page vm.Page,
) *mem.ReadReq {
	offset := req.Address % (1 << m.log2PageSize)
	addr := page.PAddr + offset
	clone := mem.ReadReqBuilder{}.
		WithSrc(m.bottomPort.AsRemote()).
		WithDst(m.memoryPortMapper.Find(addr)).
		WithAddress(addr).
		WithByteSize(req.AccessByteSize).
		WithPID(0).
		WithInfo(req.Info).
		Build()
	clone.CanWaitForCoalesce = req.CanWaitForCoalesce

	return clone
}

func (m *middleware) createTranslatedWriteReq(
	req *mem.WriteReq,
	page vm.Page,
) *mem.WriteReq {
	offset := req.Address % (1 << m.log2PageSize)
	addr := page.PAddr + offset
	clone := mem.WriteReqBuilder{}.
		WithSrc(m.bottomPort.AsRemote()).
		WithDst(m.memoryPortMapper.Find(addr)).
		WithData(req.Data).
		WithDirtyMask(req.DirtyMask).
		WithAddress(addr).
		WithPID(0).
		WithInfo(req.Info).
		Build()
	clone.CanWaitForCoalesce = req.CanWaitForCoalesce

	return clone
}

func (m *middleware) addrToPageID(addr uint64) uint64 {
	return (addr >> m.log2PageSize) << m.log2PageSize
}

func (m *middleware) findTranslationByReqID(id string) *transaction {
	for _, t := range m.transactions {
		if t.translationReq.ID == id {
			return t
		}
	}

	return nil
}

func (m *middleware) removeExistingTranslation(trans *transaction) {
	for i, tr := range m.transactions {
		if tr == trans {
			m.transactions = append(m.transactions[:i], m.transactions[i+1:]...)
			return
		}
	}

	panic("translation not found")
}

func (m *middleware) isReqInBottomByID(id string) bool {
	for _, r := range m.inflightReqToBottom {
		if r.reqToBottom.Meta().ID == id {
			return true
		}
	}

	return false
}

func (m *middleware) findReqToBottomByID(id string) reqToBottom {
	for _, r := range m.inflightReqToBottom {
		if r.reqToBottom.Meta().ID == id {
			return r
		}
	}

	panic("req to bottom not found")
}

func (m *middleware) removeReqToBottomByID(id string) {
	for i, r := range m.inflightReqToBottom {
		if r.reqToBottom.Meta().ID == id {
			m.inflightReqToBottom = append(
				m.inflightReqToBottom[:i],
				m.inflightReqToBottom[i+1:]...)

			return
		}
	}

	panic("req to bottom not found")
}

func (m *middleware) handleCtrlRequest() bool {
	req := m.ctrlPort.PeekIncoming()
	if req == nil {
		return false
	}

	msg := req.(*mem.ControlMsg)

	if msg.DiscardTransations {
		return m.handleFlushReq(msg)
	} else if msg.Restart {
		return m.handleRestartReq(msg)
	}

	panic("never")
}

func (m *middleware) handleFlushReq(
	req *mem.ControlMsg,
) bool {
	rsp := mem.ControlMsgBuilder{}.
		WithSrc(m.ctrlPort.AsRemote()).
		WithDst(req.Src).
		ToNotifyDone().
		Build()

	err := m.ctrlPort.Send(rsp)
	if err != nil {
		return false
	}

	m.ctrlPort.RetrieveIncoming()

	m.transactions = nil
	m.inflightReqToBottom = nil
	m.isFlushing = true

	return true
}

func (m *middleware) handleRestartReq(
	req *mem.ControlMsg,
) bool {
	rsp := mem.ControlMsgBuilder{}.
		WithSrc(m.ctrlPort.AsRemote()).
		WithDst(req.Src).
		ToNotifyDone().
		Build()

	err := m.ctrlPort.Send(rsp)

	if err != nil {
		return false
	}

	for m.topPort.RetrieveIncoming() != nil {
	}

	for m.bottomPort.RetrieveIncoming() != nil {
	}

	for m.translationPort.RetrieveIncoming() != nil {
	}

	m.isFlushing = false

	m.ctrlPort.RetrieveIncoming()

	return true
}
