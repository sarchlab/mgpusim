package wavefront

import (
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/kernels"
)

// A WorkGroup is a wrapper for the kernels.WorkGroup
type WorkGroup struct {
	*kernels.WorkGroup

	Wfs    []*Wavefront
	MapReq *gcn3.MapWGReq
	LDS    []byte
}

// NewWorkGroup returns a newly constructed WorkGroup
func NewWorkGroup(raw *kernels.WorkGroup, req *gcn3.MapWGReq) *WorkGroup {
	wg := new(WorkGroup)
	wg.WorkGroup = raw
	wg.MapReq = req
	wg.Wfs = make([]*Wavefront, 0)
	return wg
}
