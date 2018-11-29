package caches

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/vm"
)

type addressTranslator struct {
	l1vCache *L1VCache

	pendingTranslation *cacheTransaction
	toSendToTLB        []*vm.TranslationReq

	madeProgress bool
}

func (t *addressTranslator) tick(now akita.VTimeInSec) bool {
	t.madeProgress = false

	t.sendToTLB(now)
	t.parseFromTLB(now)
	t.generateTranslationReq(now)

	return t.madeProgress
}

func (t *addressTranslator) sendToTLB(now akita.VTimeInSec) {
	if len(t.toSendToTLB) == 0 {
		return
	}

	req := t.toSendToTLB[0]
	req.SetSendTime(now)
	err := t.l1vCache.ToTLB.Send(req)
	if err == nil {
		t.madeProgress = true
		t.toSendToTLB = t.toSendToTLB[1:]
	}
}

func (t *addressTranslator) generateTranslationReq(now akita.VTimeInSec) {
	if len(t.l1vCache.preAddrTranslationBuf) == 0 {
		return
	}

	if t.pendingTranslation != nil {
		return
	}

	t.madeProgress = true

	trans := t.l1vCache.preAddrTranslationBuf[0]
	t.l1vCache.preAddrTranslationBuf =
		t.l1vCache.preAddrTranslationBuf[1:]
	t.pendingTranslation = trans
	req := trans.Req.(mem.AccessReq)
	switch req := req.(type) {
	case *mem.ReadReq:
		translationReq := vm.NewTranslateReq(10,
			t.l1vCache.ToTLB, t.l1vCache.TLB,
			req.PID, req.Address)
		t.toSendToTLB = append(t.toSendToTLB, translationReq)

	case *mem.WriteReq:
		translationReq := vm.NewTranslateReq(10,
			t.l1vCache.ToTLB, t.l1vCache.TLB,
			req.PID, req.Address)
		t.toSendToTLB = append(t.toSendToTLB, translationReq)

	default:
		panic("cannot process request")
	}

}

func (t *addressTranslator) parseFromTLB(
	now akita.VTimeInSec,
) {
	req := t.l1vCache.ToTLB.Retrieve(now)
	if req == nil {
		return
	}

	translationRsp := req.(*vm.TranslateReadyRsp)
	t.pendingTranslation.Page = translationRsp.Page
	t.l1vCache.postAddrTranslationBuf =
		append(t.l1vCache.postAddrTranslationBuf, t.pendingTranslation)
	t.pendingTranslation = nil

	t.madeProgress = true

}
