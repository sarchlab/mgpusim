package gcn3

import (
	"fmt"

	"gitlab.com/yaotsu/core"
)

// A GPU is the unit that one kernel can run on.
//
// A GPU is a Yaotsu component and it defines the port "ToDriver". Driver is
// a piece of software that conceptually runs in the Cpu. Therefore, all the
// CPU-GPU communication happens on the connection connecting the "ToDriver"
// port.
type GPU struct {
	*core.ComponentBase

	Freq core.Freq

	Driver           core.Component // The DriverComponent
	CommandProcessor core.Component // The CommandProcessor
	Dispatchers      []core.Component
	CUs              []core.Component
}

// NewGPU returns a newly created GPU
func NewGPU(name string) *GPU {
	g := new(GPU)
	g.ComponentBase = core.NewComponentBase(name)
	g.AddPort("ToDriver")
	g.AddPort("ToCommandProcessor")
	return g
}

// Handle defines how a GPU handles core.
//
// A GPU should not handle any event by itself.
func (g *GPU) Handle(e core.Event) error {
	return nil
}

// Recv processes incoming request to the GPU.
//
// The GPU itself does not respond to requests, but it always forward to the
// CommandProcessor.
func (g *GPU) Recv(req core.Req) *core.Error {
	if req.Src() == g.CommandProcessor { // From the CommandProcessor
		req.SetSrc(g)
		req.SetDst(g.Driver)
		g.GetConnection("ToDriver").Send(req)
		return nil
	} else if req.Src() == g.Driver { // From the Driver
		req.SetSrc(g)
		req.SetDst(g.CommandProcessor)
		g.GetConnection("ToCommandProcessor").Send(req)
		return nil
	}

	return core.NewError(
		fmt.Sprintf("Unrecognized source %s", req.Src().Name()), false, 0)
}
