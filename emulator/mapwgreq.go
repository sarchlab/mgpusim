package emulator

import "gitlab.com/yaotsu/core/conn"

// A MapWgReq is a request sent from a dispatcher to a compute unit
// to request the compute unit to execute a workgroup.
type MapWgReq struct {
	*conn.BasicRequest

	WG      *WorkGroup
	IsReply bool
	Succeed bool
}

// NewMapWGReq returns a new MapWorkGroupReq
func NewMapWGReq() *MapWgReq {
	r := new(MapWgReq)
	r.BasicRequest = conn.NewBasicRequest()

	return r
}

// MapWGReqFactory is the factory that creates MapWorkGroupReq
type MapWGReqFactory interface {
	Create() *MapWgReq
}

type mapWGReqFactoryImpl struct {
}

func (f *mapWGReqFactoryImpl) Create() *MapWgReq {
	return NewMapWGReq()
}

// NewMapWGReqFactory returns the default factory for the
// MapWorkGroupReq
func NewMapWGReqFactory() MapWGReqFactory {
	return &mapWGReqFactoryImpl{}
}
