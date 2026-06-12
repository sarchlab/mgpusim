package cu

import (
	"log"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
)

func TestSimulator(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "GCN3 Timing Simulator")
}

func prepareGrid(co *insts.KernelCodeObject) *kernels.Grid {
	// Prepare a mock grid that is expanded
	grid := kernels.NewGrid()
	grid.CodeObject = co
	for i := 0; i < 5; i++ {
		wg := kernels.NewWorkGroup()
		wg.CodeObject = co
		grid.WorkGroups = append(grid.WorkGroups, wg)
		for j := 0; j < 10; j++ {
			wf := kernels.NewWavefront()
			wf.WG = wg
			wg.Wavefronts = append(wg.Wavefronts, wf)
		}
	}
	return grid
}

// fakeEngine is a controllable engine for tests. It records scheduled events
// and lets tests set the current time.
type fakeEngine struct {
	hooking.HookableBase

	now       timing.VTimeInPicoSec
	scheduled []timing.Event
	handlers  map[string]timing.Handler
}

func newFakeEngine() *fakeEngine {
	return &fakeEngine{handlers: make(map[string]timing.Handler)}
}

func (e *fakeEngine) CurrentTime() timing.VTimeInPicoSec { return e.now }
func (e *fakeEngine) Schedule(evt timing.Event) {
	e.scheduled = append(e.scheduled, evt)
}
func (e *fakeEngine) Run() error { return nil }
func (e *fakeEngine) Pause()     {}
func (e *fakeEngine) Continue()  {}
func (e *fakeEngine) RegisterHandler(name string, h timing.Handler) {
	e.handlers[name] = h
}

// fakePort is a controllable implementation of messaging.Port for tests.
// Tests fill the incoming slice to emulate received messages, and inspect
// the sent slice to verify outgoing messages. Setting full to true makes
// CanSend return false.
type fakePort struct {
	hooking.HookableBase

	name     string
	incoming []messaging.Msg
	sent     []messaging.Msg
	full     bool
}

func newFakePort(name string) *fakePort {
	return &fakePort{name: name}
}

func (p *fakePort) Name() string { return p.name }
func (p *fakePort) AsRemote() messaging.RemotePort {
	return messaging.RemotePort(p.name)
}
func (p *fakePort) SetConnection(conn messaging.Connection) {}
func (p *fakePort) Component() messaging.Component          { return nil }
func (p *fakePort) SetComponent(comp messaging.Component)   {}
func (p *fakePort) CanDeliver() bool                        { return true }
func (p *fakePort) Deliver(msg messaging.Msg) {
	p.incoming = append(p.incoming, msg)
}
func (p *fakePort) NotifyAvailable()                {}
func (p *fakePort) RetrieveOutgoing() messaging.Msg { return nil }
func (p *fakePort) PeekOutgoing() messaging.Msg     { return nil }
func (p *fakePort) CanSend() bool                   { return !p.full }
func (p *fakePort) NumIncoming() int                { return len(p.incoming) }
func (p *fakePort) NumOutgoing() int                { return len(p.sent) }

func (p *fakePort) Send(msg messaging.Msg) {
	if p.full {
		panic("Send called on a full fakePort; check CanSend first")
	}
	p.sent = append(p.sent, msg)
}

func (p *fakePort) PeekIncoming() messaging.Msg {
	if len(p.incoming) == 0 {
		return nil
	}
	return p.incoming[0]
}

func (p *fakePort) RetrieveIncoming() messaging.Msg {
	if len(p.incoming) == 0 {
		return nil
	}
	msg := p.incoming[0]
	p.incoming = p.incoming[1:]
	return msg
}

// newTestComputeUnit builds a bare ComputeUnit middleware around a freshly
// built component. Sub-units, register files, and ports are set by each test
// as needed.
func newTestComputeUnit(
	name string,
	engine timing.EventScheduler,
) *ComputeUnit {
	comp := modeling.NewBuilder[Spec, State, Resources]().
		WithEngine(engine).
		WithFreq(1 * timing.GHz).
		WithSpec(DefaultSpec()).
		Build(name)
	comp.State = State{}

	cuMW := &ComputeUnit{
		comp:                  comp,
		engine:                engine,
		wfCompletionHandlerID: name + ".WfCompletion",
		wfDispatchHandlerID:   name + ".WfDispatch",
		wftime:                make(map[uint64]timing.VTimeInPicoSec),
	}
	cuMW.InFlightVectorMemAccessLimit = 512

	return cuMW
}
