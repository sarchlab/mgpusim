package caches

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
)

type addressTranslator struct {
	preTranslationBuf  []*cacheTransaction
	pendingTranslation []*cacheTransaction
}

func (t *addressTranslator) acceptReq(req mem.AccessReq) bool {
	panic("implement me")
}

func (t *addressTranslator) tick(now akita.VTimeInSec) bool {
	panic("implement me")
	return false
}

func (t *addressTranslator) parseFromTLB(
	now akita.VTimeInSec,
	toTLB akita.Port,
) bool {
	panic("implement me")
	return false
}
