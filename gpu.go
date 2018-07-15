package gcn3

import (
	"log"

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

	engine core.Engine
	Freq   core.Freq

	Driver           *core.Port // The DriverComponent
	CommandProcessor *core.Port // The CommandProcessor
	Dispatchers      []core.Component
	CUs              []core.Component

	ToDriver           *core.Port
	ToCommandProcessor *core.Port
}

func (g *GPU) NotifyPortFree(now core.VTimeInSec, port *core.Port) {
}

func (g *GPU) NotifyRecv(now core.VTimeInSec, port *core.Port) {
	req := port.Retrieve(now)
	core.ProcessReqAsEvent(req, g.engine, g.Freq)
}

// Handle defines how a GPU handles core.
//
// A GPU should not handle any event by itself.
func (g *GPU) Handle(e core.Event) error {
	now := e.Time()
	req := e.(core.Req)

	if req.Src() == g.CommandProcessor { // From the CommandProcessor
		req.SetSrc(g.ToDriver)
		req.SetDst(g.Driver)
		req.SetSendTime(now)
		g.ToDriver.Send(req)
		return nil
	} else if req.Src() == g.Driver { // From the Driver
		req.SetSrc(g.ToCommandProcessor)
		req.SetDst(g.CommandProcessor)
		req.SetSendTime(now)
		g.ToCommandProcessor.Send(req)
		return nil
	}

	log.Panic("Unknown source")

	return nil
}

// NewGPU returns a newly created GPU
func NewGPU(name string, engine core.Engine) *GPU {
	g := new(GPU)
	g.ComponentBase = core.NewComponentBase(name)

	g.engine = engine
	g.Freq = 1 * core.GHz

	g.ToDriver = core.NewPort(g)
	g.ToCommandProcessor = core.NewPort(g)

	return g
}
