// Package wavefront defines concepts related to a wavefront.
package wavefront

import (
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// Inst in the timing package is a wrapper of the insts.Inst.
type Inst struct {
	*insts.Inst

	ID uint64
	// Raw holds the decoded instruction bytes for portable checkpoint restoration.
	Raw []byte
}

// NewInst creates a newly created Inst
func NewInst(id uint64, raw *insts.Inst) *Inst {
	i := new(Inst)
	i.Inst = raw

	i.ID = id

	return i
}
