// Package wavefront defines concepts related to a wavefront.
package wavefront

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mgpusim/insts"
)

// Inst in the timing package is a wrapper of the insts.Inst.
type Inst struct {
	*insts.Inst

	ID string
}

// NewInst creates a newly created Inst
func NewInst(raw *insts.Inst) *Inst {
	i := new(Inst)
	i.Inst = raw

	i.ID = akita.GetIDGenerator().Generate()

	return i
}
