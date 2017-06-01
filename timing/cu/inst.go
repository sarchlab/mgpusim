package cu

import (
	"sync"

	"gitlab.com/yaotsu/gcn3/insts"
)

var _NextInstIDMutex sync.Mutex
var _NextInstID uint64

// Inst in the timing package is a wrapper of the insts.Inst.
type Inst struct {
	*insts.Inst

	ID uint64
}

// NewInst creates a newly created Inst
func NewInst(raw *insts.Inst) *Inst {
	i := new(Inst)
	i.Inst = raw

	_NextInstIDMutex.Lock()
	i.ID = _NextInstID
	_NextInstID++
	_NextInstIDMutex.Unlock()

	return i
}
