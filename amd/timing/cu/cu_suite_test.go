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
	comp.State.Initialize(comp)

	cuMW := &ComputeUnit{
		comp:                  comp,
		engine:                engine,
		wfCompletionHandlerID: name + ".WfCompletion",
		wfDispatchHandlerID:   name + ".WfDispatch",
		waves:                 make(map[uint64]*wavefront.Wavefront),
		instructions:          make(map[uint64]*wavefront.Inst),
		boundDecoder:          &comp.State.Decoders[0],
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

func newTestWave() *wavefront.Wavefront {
	return wavefront.NewWavefront(kernels.NewWavefront(testIDs.NewID()))
}
func registerTestWave(c *ComputeUnit, w *wavefront.Wavefront) {
	if w == nil {
		return
	}
	if w.Wavefront == nil {
		w.Wavefront = kernels.NewWavefront(testIDs.NewID())
	}
	if w.UID == 0 {
		w.UID = testIDs.NewID()
	}
	if c.waves[w.UID] == w {
		return
	}
	if w.WG == nil {
		w.WG = wavefront.NewWorkGroup(kernels.NewWorkGroup(testIDs.NewID()), messaging.Msg{})
	}
	if w.WG.WorkGroup == nil {
		w.WG.WorkGroup = kernels.NewWorkGroup(testIDs.NewID())
	}
	current, next := w.DynamicInst(), w.NextInst()
	g := c.comp.State.Groups[w.WG.UID]
	g.Waves = append(g.Waves, WaveState{ID: w.UID, Execution: *w.ExecutionState})
	c.comp.State.Groups[w.WG.UID] = g
	c.waves[w.UID] = w
	c.workGroups[w.WG.UID] = w.WG
	found := false
	for _, other := range w.WG.Wfs {
		if other == w {
			found = true
		}
	}
	if !found {
		w.WG.Wfs = append(w.WG.Wfs, w)
	}
	for i := range g.Waves {
		view := c.waves[g.Waves[i].ID]
		view.BindExecution(&g.Waves[i].Execution, c.instruction, c.rememberInstruction, c.releaseInstruction)
	}
	c.rememberInstruction(current)
	c.rememberInstruction(next)
}
func testWaveID(c *ComputeUnit, w *wavefront.Wavefront) uint64 {
	registerTestWave(c, w)
	return wfID(w)
}
func testAddWave(p *WavefrontPool, w *wavefront.Wavefront) { registerTestWave(p.cu, w); p.AddWf(w) }

func testFetchState(c *ComputeUnit, v *InstFetchReqInfo) FetchState {
	registerTestWave(c, v.Wavefront)
	return c.fetchState(v)
}
func testScalarState(c *ComputeUnit, v *ScalarMemAccessInfo) ScalarAccessState {
	registerTestWave(c, v.Wavefront)
	c.rememberInstruction(v.Inst)
	s := ScalarAccessState{Req: v.Req, WF: wfID(v.Wavefront), Inst: instID(v.Inst)}
	if v.DstSGPR != nil {
		s.Register = v.DstSGPR.RegType
	}
	return s
}
func testVectorState(c *ComputeUnit, v VectorMemAccessInfo) VectorAccessState {
	registerTestWave(c, v.Wavefront)
	return c.vectorState(v)
}
func testFetchStates(c *ComputeUnit, vs []*InstFetchReqInfo) []FetchState {
	var out []FetchState
	for _, v := range vs {
		out = append(out, testFetchState(c, v))
	}
	return out
}
func testVectorStates(c *ComputeUnit, vs []VectorMemAccessInfo) []VectorAccessState {
	var out []VectorAccessState
	for _, v := range vs {
		out = append(out, testVectorState(c, v))
	}
	return out
}
