package l1v

import (
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
)

type bankActionType int

const (
	bankActionInvalid bankActionType = iota
	bankActionReadHit
	bankActionWrite
	bankActionWriteFetched
)

type transaction struct {
	read                *mem.ReadReq
	readToBottom        *mem.ReadReq
	dataReadyFromBottom *mem.DataReadyRsp
	dataReadyToTop      *mem.DataReadyRsp

	write          *mem.WriteReq
	writeToBottom  *mem.WriteReq
	doneFromBottom *mem.DoneRsp
	doneToTop      *mem.DoneRsp

	preCoalesceTransactions []*transaction

	bankAction            bankActionType
	block                 *cache.Block
	data                  []byte
	writeFetchedDirtyMask []bool

	done bool
}

func (t *transaction) Address() uint64 {
	if t.read != nil {
		return t.read.Address
	}
	return t.write.Address
}
