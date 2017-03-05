package emulator

import (
	"fmt"
	"log"
	"reflect"

	"gitlab.com/yaotsu/core/conn"
	"gitlab.com/yaotsu/core/event"
)

// A MapWorkGroupReq is a request sent from a dispatcher to a compute unit
// to request the compute unit to execute a workgroup.
type MapWorkGroupReq struct {
	*conn.BasicRequest

	WG      *WorkGroup
	IsReply bool
	Succeed bool
}

// NewMapWorkGroupReq returns a new MapWorkGroupReq
func NewMapWorkGroupReq() *MapWorkGroupReq {
	r := new(MapWorkGroupReq)
	r.BasicRequest = conn.NewBasicRequest()

	return r
}

// MapWorkGroupReqFactory is the factory that creates MapWorkGroupReq
type MapWorkGroupReqFactory interface {
	Create() *MapWorkGroupReq
}

type mapWorkGroupReqFactoryImpl struct {
}

func (f *mapWorkGroupReqFactoryImpl) Create() *MapWorkGroupReq {
	return NewMapWorkGroupReq()
}

// NewMapWorkGroupReqFactory returns the default factory for the
// MapWorkGroupReq
func NewMapWorkGroupReqFactory() MapWorkGroupReqFactory {
	return &mapWorkGroupReqFactoryImpl{}
}

// A ComputeUnit is the unit that can execute workgroups.
//
// A ComputeUnit is a Yaotsu component. It defines port "ToDispatcher" to
// receive the dispatching request
type ComputeUnit struct {
	*conn.BasicComponent

	MaxNumWGs  int
	WorkGroups []*WorkGroup
}

// NewComputeUnit creates a ComputeUnit
func NewComputeUnit(name string) *ComputeUnit {
	cu := new(ComputeUnit)
	cu.BasicComponent = conn.NewBasicComponent(name)
	cu.WorkGroups = make([]*WorkGroup, 0)
	cu.MaxNumWGs = 1

	cu.AddPort("ToDispatcher")
	return cu
}

func (cu *ComputeUnit) handleMapWorkGroupReq(req *MapWorkGroupReq) *conn.Error {
	if len(cu.WorkGroups) >= cu.MaxNumWGs {
		req.SwapSrcAndDst()
		req.IsReply = true
		req.Succeed = false
		cu.GetConnection("ToDispatcher").Send(req)
		return nil
	}

	log.Printf("CU starts\n")

	return nil
}

// Receive processes the incomming requests
func (cu *ComputeUnit) Receive(req conn.Request) *conn.Error {
	switch req := req.(type) {
	case *MapWorkGroupReq:
		return cu.handleMapWorkGroupReq(req)
	default:
		return conn.NewError(
			fmt.Sprintf("cannot process request %s", reflect.TypeOf(req)), false, 0)
	}
}

// Handle processes the events that is scheduled for the CommandProcessor
func (cu *ComputeUnit) Handle(e event.Event) error {
	return nil
}
