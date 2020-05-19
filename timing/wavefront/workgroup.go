package wavefront

import (
	"gitlab.com/akita/mgpusim/kernels"
	"gitlab.com/akita/mgpusim/protocol"
)

// A WorkGroup is a wrapper for the kernels.WorkGroup
type WorkGroup struct {
	*kernels.WorkGroup

	Wfs    []*Wavefront
	MapReq *protocol.MapWGReq
	LDS    []byte
}

// NewWorkGroup returns a newly constructed WorkGroup
func NewWorkGroup(raw *kernels.WorkGroup, req *protocol.MapWGReq) *WorkGroup {
	wg := new(WorkGroup)
	wg.WorkGroup = raw
	wg.MapReq = req
	wg.Wfs = make([]*Wavefront, 0)
	return wg
}
