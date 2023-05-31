package addresstranslator

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/mem"

	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/mem/vm"
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

// AddressTranslator is a component that forwards the read/write requests with
// the address translated from virtual to physical.
type AddressTranslator struct {
	*sim.TickingComponent

	topPort         sim.Port
	bottomPort      sim.Port
	translationPort sim.Port
	ctrlPort        sim.Port

	lowModuleFinder     mem.LowModuleFinder
	translationProvider sim.Port
	log2PageSize        uint64
	deviceID            uint64
	numReqPerCycle      int

	isFlushing bool

	transactions        []*transaction
	inflightReqToBottom []reqToBottom

	isWaitingForGL0InvCompletion   bool
	isWaitingOnGL0InvalidateRsp    bool
	currentGL0InvReq               *mem.GL0InvalidateReq
	totalRequestsUponGL0InvArrival int
}

// SetTranslationProvider sets the remote port that can translate addresses.
func (t *AddressTranslator) SetTranslationProvider(p sim.Port) {
	t.translationProvider = p
}

// SetLowModuleFinder sets the table recording where to find an address.
func (t *AddressTranslator) SetLowModuleFinder(lmf mem.LowModuleFinder) {
	t.lowModuleFinder = lmf
}

// Tick updates state at each cycle.
func (t *AddressTranslator) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	if !t.isFlushing {
		madeProgress = t.runPipeline(now)
	}

	madeProgress = t.handleCtrlRequest(now) || madeProgress

	return madeProgress
}

func (t *AddressTranslator) runPipeline(now sim.VTimeInSec) bool {
	madeProgress := false

	for i := 0; i < t.numReqPerCycle; i++ {
		madeProgress = t.respond(now) || madeProgress
	}

	for i := 0; i < t.numReqPerCycle; i++ {
		madeProgress = t.parseTranslation(now) || madeProgress
	}

	for i := 0; i < t.numReqPerCycle; i++ {
		madeProgress = t.translate(now) || madeProgress
	}

	madeProgress = t.doGL0Invalidate(now) || madeProgress

	return madeProgress
}

func (t *AddressTranslator) doGL0Invalidate(now sim.VTimeInSec) bool {
	if t.currentGL0InvReq == nil {
		return false
	}

	if t.isWaitingOnGL0InvalidateRsp == true {
		return false
	}

	if t.totalRequestsUponGL0InvArrival == 0 {
		req := mem.GL0InvalidateReqBuilder{}.
			WithPID(t.currentGL0InvReq.PID).
			WithSrc(t.bottomPort).
			WithDst(t.lowModuleFinder.Find(0)).
			WithSendTime(now).
			Build()

		err := t.bottomPort.Send(req)
		if err == nil {
			t.isWaitingOnGL0InvalidateRsp = true
			return true
		}
	}

	return true
}

func (t *AddressTranslator) translate(now sim.VTimeInSec) bool {
	if t.currentGL0InvReq != nil {
		return false
	}

	item := t.topPort.Peek()
	if item == nil {
		return false
	}

	switch req := item.(type) {
	case *mem.GL0InvalidateReq:
		return t.handleGL0InvalidateReq(now, req)
	}

	req := item.(mem.AccessReq)
	vAddr := req.GetAddress()
	vPageID := t.addrToPageID(vAddr)

	transReq := vm.TranslationReqBuilder{}.
		WithSendTime(now).
		WithSrc(t.translationPort).
		WithDst(t.translationProvider).
		WithPID(req.GetPID()).
		WithVAddr(vPageID).
		WithDeviceID(t.deviceID).
		Build()
	err := t.translationPort.Send(transReq)
	if err != nil {
		return false
	}

	translation := &transaction{
		incomingReqs:   []mem.AccessReq{req},
		translationReq: transReq,
	}
	t.transactions = append(t.transactions, translation)

	tracing.TraceReqReceive(req, t)
	tracing.TraceReqInitiate(transReq, t, tracing.MsgIDAtReceiver(req, t))

	t.topPort.Retrieve(now)

	return true
}

func (t *AddressTranslator) handleGL0InvalidateReq(
	now sim.VTimeInSec,
	req *mem.GL0InvalidateReq,
) bool {
	if t.currentGL0InvReq != nil {
		return false
	}

	t.currentGL0InvReq = req
	t.totalRequestsUponGL0InvArrival =
		len(t.transactions) + len(t.inflightReqToBottom)
	t.topPort.Retrieve(now)

	return true
}

func (t *AddressTranslator) parseTranslation(now sim.VTimeInSec) bool {
	rsp := t.translationPort.Peek()
	if rsp == nil {
		return false
	}

	transRsp := rsp.(*vm.TranslationRsp)
	transaction := t.findTranslationByReqID(transRsp.RespondTo)
	if transaction == nil {
		t.translationPort.Retrieve(now)
		return true
	}

	transaction.translationRsp = transRsp
	transaction.translationDone = true
	reqFromTop := transaction.incomingReqs[0]
	translatedReq := t.createTranslatedReq(
		reqFromTop,
		transaction.translationRsp.Page)
	translatedReq.Meta().SendTime = now
	err := t.bottomPort.Send(translatedReq)
	if err != nil {
		return false
	}

	t.inflightReqToBottom = append(t.inflightReqToBottom,
		reqToBottom{
			reqFromTop:  reqFromTop,
			reqToBottom: translatedReq,
		})
	transaction.incomingReqs = transaction.incomingReqs[1:]
	if len(transaction.incomingReqs) == 0 {
		t.removeExistingTranslation(transaction)
	}

	t.translationPort.Retrieve(now)

	tracing.TraceReqFinalize(transaction.translationReq, t)
	tracing.TraceReqInitiate(translatedReq, t,
		tracing.MsgIDAtReceiver(reqFromTop, t))

	return true
}

//nolint:funlen,gocyclo
func (t *AddressTranslator) respond(now sim.VTimeInSec) bool {
	rsp := t.bottomPort.Peek()
	if rsp == nil {
		return false
	}

	reqInBottom := false
	gl0InvalidateRsp := false

	var reqFromTop mem.AccessReq
	var reqToBottomCombo reqToBottom
	var rspToTop mem.AccessRsp
	switch rsp := rsp.(type) {
	case *mem.DataReadyRsp:
		reqInBottom = t.isReqInBottomByID(rsp.RespondTo)
		if reqInBottom {
			reqToBottomCombo = t.findReqToBottomByID(rsp.RespondTo)
			reqFromTop = reqToBottomCombo.reqFromTop
			drToTop := mem.DataReadyRspBuilder{}.
				WithSendTime(now).
				WithSrc(t.topPort).
				WithDst(reqFromTop.Meta().Src).
				WithRspTo(reqFromTop.Meta().ID).
				WithData(rsp.Data).
				Build()
			rspToTop = drToTop
		}
	case *mem.WriteDoneRsp:
		reqInBottom = t.isReqInBottomByID(rsp.RespondTo)
		if reqInBottom {
			reqToBottomCombo = t.findReqToBottomByID(rsp.RespondTo)
			reqFromTop = reqToBottomCombo.reqFromTop
			rspToTop = mem.WriteDoneRspBuilder{}.
				WithSendTime(now).
				WithSrc(t.topPort).
				WithDst(reqFromTop.Meta().Src).
				WithRspTo(reqFromTop.Meta().ID).
				Build()
		}
	case *mem.GL0InvalidateRsp:
		gl0InvalidateReq := t.currentGL0InvReq
		if gl0InvalidateReq == nil {
			log.Panicf("Cannot have rsp without req")
		}
		rspToTop = mem.GL0InvalidateRspBuilder{}.
			WithSendTime(now).
			WithSrc(t.topPort).
			WithDst(gl0InvalidateReq.Src).
			WithRspTo(gl0InvalidateReq.Meta().ID).
			Build()
		gl0InvalidateRsp = true
	default:
		log.Panicf("cannot handle respond of type %s", reflect.TypeOf(rsp))
	}

	if reqInBottom {
		err := t.topPort.Send(rspToTop)
		if err != nil {
			return false
		}

		t.removeReqToBottomByID(rsp.(mem.AccessRsp).GetRspTo())

		tracing.TraceReqFinalize(reqToBottomCombo.reqToBottom, t)
		tracing.TraceReqComplete(reqToBottomCombo.reqFromTop, t)
	}

	if gl0InvalidateRsp {
		err := t.topPort.Send(rspToTop)
		if err != nil {
			return false
		}
		t.currentGL0InvReq = nil
		t.isWaitingOnGL0InvalidateRsp = false
		if t.totalRequestsUponGL0InvArrival != 0 {
			log.Panicf("Something went wrong \n")
		}
	}

	if t.currentGL0InvReq != nil {
		t.totalRequestsUponGL0InvArrival--

		if t.totalRequestsUponGL0InvArrival < 0 {
			log.Panicf("Not possible")
		}
	}

	t.bottomPort.Retrieve(now)
	return true
}

func (t *AddressTranslator) createTranslatedReq(
	req mem.AccessReq,
	page vm.Page,
) mem.AccessReq {
	switch req := req.(type) {
	case *mem.ReadReq:
		return t.createTranslatedReadReq(req, page)
	case *mem.WriteReq:
		return t.createTranslatedWriteReq(req, page)
	default:
		log.Panicf("cannot translate request of type %s", reflect.TypeOf(req))
		return nil
	}
}

func (t *AddressTranslator) createTranslatedReadReq(
	req *mem.ReadReq,
	page vm.Page,
) *mem.ReadReq {
	offset := req.Address % (1 << t.log2PageSize)
	addr := page.PAddr + offset
	clone := mem.ReadReqBuilder{}.
		WithSrc(t.bottomPort).
		WithDst(t.lowModuleFinder.Find(addr)).
		WithAddress(addr).
		WithByteSize(req.AccessByteSize).
		WithPID(0).
		WithInfo(req.Info).
		Build()
	clone.CanWaitForCoalesce = req.CanWaitForCoalesce
	return clone
}

func (t *AddressTranslator) createTranslatedWriteReq(
	req *mem.WriteReq,
	page vm.Page,
) *mem.WriteReq {
	offset := req.Address % (1 << t.log2PageSize)
	addr := page.PAddr + offset
	clone := mem.WriteReqBuilder{}.
		WithSrc(t.bottomPort).
		WithDst(t.lowModuleFinder.Find(addr)).
		WithData(req.Data).
		WithDirtyMask(req.DirtyMask).
		WithAddress(addr).
		WithPID(0).
		WithInfo(req.Info).
		Build()
	clone.CanWaitForCoalesce = req.CanWaitForCoalesce
	return clone
}

func (t *AddressTranslator) addrToPageID(addr uint64) uint64 {
	return (addr >> t.log2PageSize) << t.log2PageSize
}

func (t *AddressTranslator) findTranslationByReqID(id string) *transaction {
	for _, t := range t.transactions {
		if t.translationReq.ID == id {
			return t
		}
	}
	return nil
}

func (t *AddressTranslator) removeExistingTranslation(trans *transaction) {
	for i, tr := range t.transactions {
		if tr == trans {
			t.transactions = append(t.transactions[:i], t.transactions[i+1:]...)
			return
		}
	}
	panic("translation not found")
}

func (t *AddressTranslator) isReqInBottomByID(id string) bool {
	for _, r := range t.inflightReqToBottom {
		if r.reqToBottom.Meta().ID == id {
			return true
		}
	}
	return false
}

func (t *AddressTranslator) findReqToBottomByID(id string) reqToBottom {
	for _, r := range t.inflightReqToBottom {
		if r.reqToBottom.Meta().ID == id {
			return r
		}
	}
	panic("req to bottom not found")
}

func (t *AddressTranslator) removeReqToBottomByID(id string) {
	for i, r := range t.inflightReqToBottom {
		if r.reqToBottom.Meta().ID == id {
			t.inflightReqToBottom = append(
				t.inflightReqToBottom[:i],
				t.inflightReqToBottom[i+1:]...)
			return
		}
	}
	panic("req to bottom not found")
}

func (t *AddressTranslator) handleCtrlRequest(now sim.VTimeInSec) bool {
	req := t.ctrlPort.Peek()
	if req == nil {
		return false
	}

	msg := req.(*mem.ControlMsg)

	if msg.DiscardTransations {
		return t.handleFlushReq(now, msg)
	} else if msg.Restart {
		return t.handleRestartReq(now, msg)
	}

	panic("never")
}

func (t *AddressTranslator) handleFlushReq(
	now sim.VTimeInSec,
	req *mem.ControlMsg,
) bool {
	rsp := mem.ControlMsgBuilder{}.
		WithSrc(t.ctrlPort).
		WithDst(req.Src).
		WithSendTime(now).
		ToNotifyDone().
		Build()

	err := t.ctrlPort.Send(rsp)
	if err != nil {
		return false
	}

	t.ctrlPort.Retrieve(now)

	t.transactions = nil
	t.inflightReqToBottom = nil
	t.isFlushing = true

	return true
}

func (t *AddressTranslator) handleRestartReq(
	now sim.VTimeInSec,
	req *mem.ControlMsg,
) bool {
	rsp := mem.ControlMsgBuilder{}.
		WithSrc(t.ctrlPort).
		WithDst(req.Src).
		WithSendTime(now).
		ToNotifyDone().
		Build()

	err := t.ctrlPort.Send(rsp)

	if err != nil {
		return false
	}

	for t.topPort.Retrieve(now) != nil {
	}

	for t.bottomPort.Retrieve(now) != nil {
	}

	for t.translationPort.Retrieve(now) != nil {
	}

	t.isFlushing = false

	t.ctrlPort.Retrieve(now)

	return true
}
