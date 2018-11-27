package gcn3

import (
	"log"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/rdma"
	"gitlab.com/akita/mem/cache"
)

// A GPU is the unit that one kernel can run on.
//
// A GPU is a Akita component and it defines the port "ToDriver". Driver is
// a piece of software that conceptually runs in the CPU. Therefore, all the
// CPU-GPU communication happens on the connection connecting the "ToDriver"
// port.
type GPU struct {
	*akita.ComponentBase

	engine akita.Engine
	Freq   akita.Freq

	Driver             akita.Port
	CommandProcessor   akita.Port
	RDMAEngine         *rdma.Engine
	Dispatchers        []akita.Component
	CUs                []akita.Component
	L1VCaches          []akita.Component
	L1ICaches          []akita.Component
	L1KCaches          []akita.Component
	L2Caches           []akita.Component
	L2CacheFinder      cache.LowModuleFinder
	MemoryControllers  []akita.Component
	InternalConnection akita.Connection

	ToDriver           akita.Port
	ToCommandProcessor akita.Port
}

// NotifyPortFree of a GPU does not do anything.
func (g *GPU) NotifyPortFree(now akita.VTimeInSec, port akita.Port) {
}

// NotifyRecv of a GPU retrieves the request from the port and process requests
// as Events.
func (g *GPU) NotifyRecv(now akita.VTimeInSec, port akita.Port) {
	req := port.Retrieve(now)
	akita.ProcessReqAsEvent(req, g.engine, g.Freq)
}

// Handle defines how a GPU handles akita.
//
// A GPU should not handle any event by itself.
func (g *GPU) Handle(e akita.Event) error {
	now := e.Time()
	req := e.(akita.Req)

	if req.Src() == g.CommandProcessor { // From the CommandProcessor
		req.SetSrc(g.ToDriver)
		req.SetDst(g.Driver)
		req.SetSendTime(now)
		err := g.ToDriver.Send(req)
		if err != nil {
			panic(err)
		}
		return nil
	} else if req.Src() == g.Driver { // From the Driver
		req.SetSrc(g.ToCommandProcessor)
		req.SetDst(g.CommandProcessor)
		req.SetSendTime(now)
		err := g.ToCommandProcessor.Send(req)
		if err != nil {
			panic(err)
		}
		return nil
	}

	log.Panic("Unknown source")

	return nil
}

// NewGPU returns a newly created GPU
func NewGPU(name string, engine akita.Engine) *GPU {
	g := new(GPU)
	g.ComponentBase = akita.NewComponentBase(name)

	g.engine = engine
	g.Freq = 1 * akita.GHz

	g.ToDriver = akita.NewLimitNumReqPort(g, 1)
	g.ToCommandProcessor = akita.NewLimitNumReqPort(g, 1)

	return g
}
