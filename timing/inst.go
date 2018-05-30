package timing

import (
	"github.com/rs/xid"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
)

//var _NextInstIDMutex sync.Mutex
//var _NextInstID uint64

// Inst in the timing package is a wrapper of the insts.Inst.
type Inst struct {
	*insts.Inst

	ID string
}

// NewInst creates a newly created Inst
func NewInst(raw *insts.Inst) *Inst {
	i := new(Inst)
	i.Inst = raw

	//_NextInstIDMutex.Lock()
	//i.ID = _NextInstID
	//_NextInstID++
	//_NextInstIDMutex.Unlock()

	i.ID = xid.New().String()

	return i
}

// InstHookInfo is the information that is to be provided to the hooks that
// hooks to Inst
type InstHookInfo struct {
	Now   core.VTimeInSec
	Inst  *Inst
	Stage string
}
