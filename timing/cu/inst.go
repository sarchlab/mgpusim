package cu

import "gitlab.com/yaotsu/gcn3/insts"

// Inst in the timing package is a wrapper of the insts.Inst.
type Inst struct {
	*insts.Inst

	ID int
}

// NewInst creates a newly created Inst
func NewInst(raw *insts.Inst) *Inst {
	i := new(Inst)
	i.Inst = raw
	return i
}
