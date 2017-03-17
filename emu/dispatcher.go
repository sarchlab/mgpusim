package emu

import (
	"fmt"
	"reflect"

	"gitlab.com/yaotsu/core"
)

// A Dispatcher is a Yaotsu component that is responsible for distributing
// the Work-groups to compute units.
//
//   ToCommandProcessor <=> Receives kernel launching requests and send
//                          kernel compeletion signal
//   ToComputeUnits <=> Send MapWorkGroupReq to compute units and
//                      receives from the compute units about the completion
//                      of the workgroups.
type Dispatcher struct {
	*core.BasicComponent

	GridBuilder     GridBuilder
	MapWGReqFactory MapWGReqFactory

	ComputeUnits        []core.Component // All the CUs
	ComputeUnitsRunning []bool           // A mask for which cu is running
	PendingGrids        []*Grid          // The Grid that had not been started
	PendingWGs          []*WorkGroup     // A Queue for all the work-groups
}

// NewDispatcher creates a new dispatcher
func NewDispatcher(
	name string,
	gridBuilder GridBuilder,
	mapWGReqFactory MapWGReqFactory,
) *Dispatcher {
	d := new(Dispatcher)
	d.BasicComponent = core.NewBasicComponent(name)

	d.GridBuilder = gridBuilder
	d.MapWGReqFactory = mapWGReqFactory

	d.ComputeUnits = make([]core.Component, 0)
	d.ComputeUnitsRunning = make([]bool, 0)
	d.PendingGrids = make([]*Grid, 0)

	d.AddPort("ToCommandProcessor")
	d.AddPort("ToComputeUnits")

	return d
}

// RegisterCU allows the dispatcher to dispatch workgroups to the
// ComputeUnit
func (d *Dispatcher) RegisterCU(cu core.Component) {
	d.ComputeUnits = append(d.ComputeUnits, cu)
	d.ComputeUnitsRunning = append(d.ComputeUnitsRunning, false)
}

// Receive processes the incomming requests
func (d *Dispatcher) Receive(req core.Request) *core.Error {
	switch req := req.(type) {
	case *LaunchKernelReq:
		return d.processLaunchKernelReq(req)
	default:
		return core.NewError(
			fmt.Sprintf("cannot process request %s", reflect.TypeOf(req)), false, 0)
	}
}

// Dispatch function search for idle compute units and send not-started
// work-groups to them.
func (d *Dispatcher) Dispatch(now core.VTimeInSec) {

	for d.numPendingWG() < d.numIdleCU() && len(d.PendingGrids) > 0 {
		g := d.PendingGrids[0]
		d.PendingGrids = d.PendingGrids[1:]
		d.PendingWGs = append(d.PendingWGs, g.WorkGroups...)
	}

	for d.numPendingWG() != 0 && d.numIdleCU() != 0 {
		wg := d.PendingWGs[0]
		d.PendingWGs = d.PendingWGs[1:]
		for i, cu := range d.ComputeUnits {
			if !d.ComputeUnitsRunning[i] {
				d.doDispatch(cu, wg, now)
				d.ComputeUnitsRunning[i] = true
				break
			}

		}
	}
}

func (d *Dispatcher) doDispatch(
	cu core.Component,
	wg *WorkGroup,
	time core.VTimeInSec,
) {
	req := d.MapWGReqFactory.Create()
	req.SetSource(d)
	req.SetDestination(cu)
	req.SetSendTime(time)
	req.WG = wg

	d.GetConnection("ToComputeUnits").Send(req)
}

func (d *Dispatcher) numIdleCU() int {
	count := 0
	for _, running := range d.ComputeUnitsRunning {
		if !running {
			count++
		}
	}
	return count
}

func (d *Dispatcher) numPendingWG() int {
	return len(d.PendingWGs)
}

func (d *Dispatcher) processLaunchKernelReq(req *LaunchKernelReq) *core.Error {
	grid := d.GridBuilder.Build(req)
	d.PendingGrids = append(d.PendingGrids, grid)

	d.Dispatch(req.RecvTime())

	return nil
}

// Handle processes the events that is scheduled for the CommandProcessor
func (d *Dispatcher) Handle(e core.Event) error {
	return nil
}
