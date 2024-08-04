// Package wavefront defines concepts related to a wavefront.
package wavefront

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v3/insts"
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

	i.ID = sim.GetIDGenerator().Generate()

	return i
}
