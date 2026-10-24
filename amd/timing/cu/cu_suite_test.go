package cu

import (
	"log"
	"sync/atomic"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
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
		wg := kernels.NewWorkGroup(testIDs.NewID())
		wg.CodeObject = co
		grid.WorkGroups = append(grid.WorkGroups, wg)
		for j := 0; j < 10; j++ {
			wf := kernels.NewWavefront(testIDs.NewID())
			wf.WG = wg
			wg.Wavefronts = append(wg.Wavefronts, wf)
		}
	}
	return grid
}

// fakeEngine is a controllable engine for tests. It records scheduled events
// and lets tests set the current time.
type fakeEngine struct {
	timing.Engine

	now       timing.VTimeInPicoSec
	scheduled []timing.Event
	handlers  map[string]timing.Handler
}

func newFakeEngine() *fakeEngine {
	return &fakeEngine{Engine: timing.NewSerialEngine(), handlers: make(map[string]timing.Handler)}
}

func (e *fakeEngine) CurrentTime() timing.VTimeInPicoSec { return e.now }
func (e *fakeEngine) Schedule(evt timing.Event) {
	e.scheduled = append(e.scheduled, evt)
}
func (e *fakeEngine) Run() error { return nil }
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
func (p *fakePort) BindConnection(messaging.Connection)          {}
func (p *fakePort) Connection() messaging.Connection             { return nil }
func (p *fakePort) Owner() messaging.PortOwner                   { return nil }
func (p *fakePort) BindOwner(_ messaging.PortOwner, name string) { p.name = name }
func (p *fakePort) IncomingBuf() messaging.BufferView            { return nil }
func (p *fakePort) OutgoingBuf() messaging.BufferView            { return nil }
func (p *fakePort) AddIncomingMilestone(messaging.WaitReason)    {}
func (p *fakePort) AddOutgoingMilestone(messaging.WaitReason)    {}
func (p *fakePort) CanDeliver() bool                             { return true }
func (p *fakePort) Deliver(msg messaging.Msg) {
	p.incoming = append(p.incoming, msg)
}
func (p *fakePort) NotifyAvailable()                        {}
func (p *fakePort) RetrieveOutgoing() (messaging.Msg, bool) { return messaging.Msg{}, false }
func (p *fakePort) PeekOutgoing() (messaging.Msg, bool)     { return messaging.Msg{}, false }
func (p *fakePort) CanSend() bool                           { return !p.full }
func (p *fakePort) NumIncoming() int                        { return len(p.incoming) }
func (p *fakePort) NumOutgoing() int                        { return len(p.sent) }

func (p *fakePort) Send(msg messaging.Msg) {
	if p.full {
		panic("Send called on a full fakePort; check CanSend first")
	}
	p.sent = append(p.sent, msg)
}

func (p *fakePort) PeekIncoming() (messaging.Msg, bool) {
	if len(p.incoming) == 0 {
		return messaging.Msg{}, false
	}
	return p.incoming[0], true
}

func (p *fakePort) RetrieveIncoming() (messaging.Msg, bool) {
	if len(p.incoming) == 0 {
		return messaging.Msg{}, false
	}
	msg := p.incoming[0]
	p.incoming = p.incoming[1:]
	return msg, true
}

// newTestComputeUnit builds a bare ComputeUnit middleware around a freshly
// built component. Sub-units, register files, and ports are set by each test
// as needed.
func newTestComputeUnit(
	name string,
	engine timing.EventScheduler,
) *ComputeUnit {
	if engine == nil {
		engine = newFakeEngine()
	}
	s := &cuTestSimulation{Simulation: sim.MakeBuilder().Build(), engine: engine.(timing.Engine)}
	s.tracker = tasks.NewTracker(engine, s.NewID, func(n naming.Named) bool { return true })
	comp := modeling.MakeBuilder[*Comp](DefaultSpec()).WithSimulation(s).Build(name)
	comp.State = State{
		InstTasks:    make(map[uint64]uint64),
		WFTasks:      make(map[uint64]uint64),
		WGTasks:      make(map[uint64]uint64),
		RequestTasks: make(map[uint64]uint64),
	}

	cuMW := &ComputeUnit{
		comp:                  comp,
		engine:                engine,
		wfCompletionHandlerID: name + ".WfCompletion",
		wfDispatchHandlerID:   name + ".WfDispatch",
		wftime:                make(map[uint64]timing.VTimeInPicoSec),
		workGroups:            make(map[uint64]*wavefront.WorkGroup),
	}
	cuMW.InFlightVectorMemAccessLimit = 512

	return cuMW
}

type cuTestSimulation struct {
	*sim.Simulation
	engine  timing.Engine
	tracker *tasks.Tracker
}

func (s *cuTestSimulation) Engine() timing.Engine       { return s.engine }
func (s *cuTestSimulation) TaskTracker() *tasks.Tracker { return s.tracker }

var testIDs testIDSource

type testIDSource struct{ value atomic.Uint64 }

func (s *testIDSource) NewID() uint64 { return (uint64(1) << 60) + s.value.Add(1) }

func registerTestWave(c *ComputeUnit, w *wavefront.Wavefront) {
	if w.WG == nil {
		w.WG = wavefront.NewWorkGroup(kernels.NewWorkGroup(testIDs.NewID()), messaging.Msg{})
	}
	w.WG.Wfs = append(w.WG.Wfs, w)
	c.workGroups[w.WG.UID] = w.WG
}
