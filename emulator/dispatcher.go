package emulator

import (
	"fmt"
	"log"
	"reflect"

	"gitlab.com/yaotsu/core/conn"
	"gitlab.com/yaotsu/core/event"
)

// A Dispatcher is a Yaotsu component that is responsible for distributing
// the Work-groups to compute units.
//
//   ToCommandProcessor <=> Receives kernel launching requests and send
//                          kernel compeletion signal
//
//   ToComputeUnits <=> Send MapWorkGroupReq to compute units and
//                      receives from the compute units about the completion
//                      of the workgroups.
type Dispatcher struct {
	*conn.BasicComponent
}

// NewDispatcher creates a new dispatcher
func NewDispatcher(name string) *Dispatcher {
	d := new(Dispatcher)
	d.BasicComponent = conn.NewBasicComponent(name)

	d.AddPort("ToCommandProcessor")

	return d
}

func (d *Dispatcher) handleLaunchKernelReq(req *LaunchKernelReq) *conn.Error {
	log.Println("Dispatching")
	return nil
}

// Receive processes the incomming requests
func (d *Dispatcher) Receive(req conn.Request) *conn.Error {
	switch req := req.(type) {
	case *LaunchKernelReq:
		return d.handleLaunchKernelReq(req)
	default:
		return conn.NewError(
			fmt.Sprintf("cannot process request %s", reflect.TypeOf(req)), false, 0)
	}
}

// Handle processes the events that is scheduled for the CommandProcessor
func (d *Dispatcher) Handle(e event.Event) error {
	return nil
}
