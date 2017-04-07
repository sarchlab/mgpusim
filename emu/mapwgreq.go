package emu

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/kernels"
)

// A MapWgReq is a request sent from a dispatcher to a compute unit
// to request the compute unit to execute a workgroup.
type MapWgReq struct {
	*core.ReqBase

	WG      *kernels.WorkGroup
	IsReply bool
	Succeed bool
}

// NewMapWGReq returns a new MapWorkGroupReq
func NewMapWGReq() *MapWgReq {
	r := new(MapWgReq)
	r.ReqBase = core.NewReqBase()

	return r
}
