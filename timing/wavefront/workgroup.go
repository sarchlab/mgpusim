package wavefront

import (
	"gitlab.com/akita/mgpusim"
	"gitlab.com/akita/mgpusim/kernels"
)

// A WorkGroup is a wrapper for the kernels.WorkGroup
type WorkGroup struct {
	*kernels.WorkGroup

	Wfs    []*Wavefront
	MapReq *mgpusim.MapWGReq
	LDS    []byte
}

// NewWorkGroup returns a newly constructed WorkGroup
func NewWorkGroup(raw *kernels.WorkGroup, req *mgpusim.MapWGReq) *WorkGroup {
	wg := new(WorkGroup)
	wg.WorkGroup = raw
	wg.MapReq = req
	wg.Wfs = make([]*Wavefront, 0)
	return wg
}
