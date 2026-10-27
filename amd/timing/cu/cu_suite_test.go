package cu

import (
	"encoding/binary"
	"fmt"
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

	cuMW := &ComputeUnit{comp: comp}
	cuMW.Decoder = fixtureDecoder{}
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

func newTestWave() *Wavefront {
	return newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
}
func registerTestWave(c *ComputeUnit, w *Wavefront) {
	if w == nil || w.cu == c {
		return
	}
	source := w.cu
	if source.SRegFile != nil && c.SRegFile == nil {
		c.SRegFile = source.SRegFile
	}
	if source.VRegFile != nil && c.VRegFile == nil {
		c.VRegFile = source.VRegFile
	}
	group := w.group()
	target := findTestGroup(c, group.UID)
	if target == nil {
		target = c.allocateGroup(group.state().Raw, group.state().Request)
		target.state().LDS = group.state().LDS
	}
	target.state().Raw = group.state().Raw
	for _, old := range group.waves() {
		if c.findWavefront(old.UID) != nil {
			continue
		}
		value := *old.state()
		allocated := c.allocateWave(target, value.Index, value)
		for _, id := range []uint64{value.Execution.InstID, value.Execution.ToIssueID} {
			if id != 0 {
				instruction := source.comp.State.Instructions[id]
				instruction.Owner = allocated.handle
				c.comp.State.Instructions[id] = instruction
			}
		}
	}
	if original := fixtureGroups[group.UID]; original != nil {
		*original = *target
	}
	*w = *c.wavefrontByID(w.UID)
}
func testWaveID(c *ComputeUnit, w *Wavefront) waveHandle {
	registerTestWave(c, w)
	return handleOf(w)
}
func testAddWave(p *WavefrontPool, w *Wavefront) { registerTestWave(p.cu, w); p.AddWf(w) }

func testFetchState(c *ComputeUnit, v *InstFetchReqInfo) FetchState {
	registerTestWave(c, v.Wavefront)
	return c.fetchState(v)
}
func testScalarState(c *ComputeUnit, v *ScalarMemAccessInfo) ScalarAccessState {
	registerTestWave(c, v.Wavefront)

	s := ScalarAccessState{Req: v.Req, WF: handleOf(v.Wavefront), Inst: v.InstID}
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

// Fixtures use synthetic instruction encodings to exercise units independently
// of the real ISA decoder. Production views still resolve the canonical encoding.
var fixtureGroups = map[uint64]*WorkGroup{}

var fixtureInstructions = map[string]*insts.Inst{}

type fixtureDecoder struct{}

func (fixtureDecoder) Decode(raw []byte) (*insts.Inst, error) {
	i, ok := fixtureInstructions[string(raw)]
	if !ok {
		return nil, fmt.Errorf("unknown fixture encoding %x", raw)
	}
	return i, nil
}
func testInst(id uint64, i *insts.Inst) *testInstruction {
	if i == nil {
		i = insts.NewInst()
	}
	raw := make([]byte, 8)
	binary.LittleEndian.PutUint64(raw, id)
	fixtureInstructions[string(raw)] = i
	v := newTestInstruction(id, i)
	v.Raw = raw
	return v
}
func newTestRawGroup(raw *kernels.WorkGroup, req messaging.Msg) *WorkGroup {
	c := newTestComputeUnit("FixtureCU", nil)
	if raw == nil {
		raw = kernels.NewWorkGroup(testIDs.NewID())
	}
	if raw.UID == 0 {
		raw.UID = testIDs.NewID()
	}
	value := kernels.WorkGroupSnapshot(raw)
	g := c.allocateGroup(value, req)
	fixtureGroups[g.UID] = g
	return g
}
func newTestRawWave(raw *kernels.Wavefront) *Wavefront {
	if raw == nil {
		raw = kernels.NewWavefront(testIDs.NewID())
	}
	if raw.UID == 0 {
		raw.UID = testIDs.NewID()
	}
	group := raw.WG
	if group == nil {
		group = kernels.NewWorkGroup(testIDs.NewID())
		group.CodeObject = raw.CodeObject
		group.Packet = raw.Packet
	}
	// Unit fixtures often provide an incomplete kernel graph; capture metadata
	// directly, without requiring the dispatch protocol's graph validation.
	groupCopy := *group
	groupCopy.Wavefronts = nil
	wg := newTestRawGroup(&groupCopy, messaging.Msg{})
	g := wg.state()
	g.Raw.Wavefronts = []kernels.WavefrontValue{{UID: raw.UID, PacketAddress: raw.PacketAddress,
		FirstWiFlatID: raw.FirstWiFlatID, InitExecMask: raw.InitExecMask}}
	return wg.cu.allocateWave(wg, 0, WaveState{ID: raw.UID,
		Execution: wavefront.ExecutionState{InstBuffer: make([]byte, 0, 256)}})
}
func setTestGroup(w *Wavefront, g *WorkGroup) {
	state := *w.state()
	raw := *w.raw()
	source := w.cu
	dst := g.state()
	if !dst.Raw.CodeObject.Present {
		dst.Raw.CodeObject = w.group().raw().CodeObject
	}
	if !dst.Raw.Packet.Present {
		dst.Raw.Packet = w.group().raw().Packet
	}
	index := len(dst.Waves)
	existing := g.cu.findWavefront(w.UID)
	if existing == nil {
		existing = g.cu.allocateWave(g, index, state)
		dst.Raw.Wavefronts = append(dst.Raw.Wavefronts, raw)
	} else {
		old := existing.state()
		if old.Group != g.slot {
			previous := existing.group().state()
			for i, h := range previous.Waves {
				if h == existing.handle {
					previous.Waves = append(previous.Waves[:i], previous.Waves[i+1:]...)
					break
				}
			}
			old.Group, old.Index = g.slot, index
			dst.Waves = append(dst.Waves, existing.handle)
			dst.Raw.Wavefronts = append(dst.Raw.Wavefronts, raw)
		}
	}
	for _, id := range []uint64{state.Execution.InstID, state.Execution.ToIssueID} {
		if id != 0 {
			instruction := source.comp.State.Instructions[id]
			instruction.Owner = existing.handle
			g.cu.comp.State.Instructions[id] = instruction
		}
	}
	*w = *existing
}
func setTestWaves(g *WorkGroup, ws []*Wavefront) {
	for _, w := range ws {
		setTestGroup(w, g)
	}
}
func setTestScoreboard(w *Wavefront, sb *Scoreboard) {
	s := w.state()
	s.HasScoreboard = sb != nil
	if sb != nil {
		s.Scoreboard = *sb
	}
}
func setTestAccessor(w *Wavefront, a wavefront.RegFileAccessor) {
	if c, ok := a.(*CURegFileAccessor); ok {
		registerTestWave(c.CU, w)
		return
	}
	f := fixtureRegisterFile{a}
	w.cu.SRegFile = f
	w.cu.VRegFile = []RegisterFile{f, f, f, f}
}

type fixtureRegisterFile struct{ a wavefront.RegFileAccessor }

func (f fixtureRegisterFile) Read(v RegisterAccess) {
	copy(v.Data, f.a.ReadReg(v.Reg, v.RegCount, v.LaneID, v.WaveOffset))
}
func (f fixtureRegisterFile) Write(v RegisterAccess) {
	f.a.WriteReg(v.Reg, v.RegCount, v.LaneID, v.WaveOffset, v.Data)
}

func setTestCodeObject(wg *WorkGroup, co *insts.KernelCodeObject) {
	g := wg.state()
	g.Raw.CodeObject = insts.CodeObjectSnapshot(co)
}
func setTestPacket(wg *WorkGroup, p *kernels.HsaKernelDispatchPacket) {
	g := wg.state()
	g.Raw.Packet = kernels.PacketSnapshot(p)
}
func setTestLDS(wg *WorkGroup, lds []byte) {
	g := wg.state()
	g.LDS = lds
}
func setTestRequest(wg *WorkGroup, r messaging.Msg) {
	g := wg.state()
	g.Request = r
}

func findTestGroup(c *ComputeUnit, id uint64) *WorkGroup {
	for i := range c.comp.State.Groups {
		g := &c.comp.State.Groups[i]
		if g.Occupied && g.Raw.UID == id {
			return &WorkGroup{cu: c, UID: id, slot: i}
		}
	}
	return nil
}
func liveTestGroups(s *State) int {
	n := 0
	for i := range s.Groups {
		if s.Groups[i].Occupied {
			n++
		}
	}
	return n
}
func liveTestWaves(s *State) int {
	n := 0
	for i := range s.Waves {
		if s.Waves[i].ID != 0 {
			n++
		}
	}
	return n
}

// testInstruction holds construction inputs only. Production retains just the
// canonical encoding/ID; fixture decoders own their mutable test descriptions.
type testInstruction struct {
	*insts.Inst
	ID  uint64
	Raw []byte
}

func newTestInstruction(id uint64, description *insts.Inst) *testInstruction {
	return &testInstruction{Inst: description, ID: id}
}
func (i *testInstruction) saved() InstructionState {
	if i == nil {
		return InstructionState{}
	}
	var pc uint64
	if i.Inst != nil {
		pc = i.PC
	}
	return InstructionState{ID: i.ID, PC: pc, Raw: i.Raw}
}
func (wf *Wavefront) setTestInst(i *testInstruction) {
	value := i.saved()
	wf.cu.rememberInstruction(wf.handle, value)
	wf.SetInstID(value.ID)
}
func (wf *Wavefront) setTestNextInst(i *testInstruction) {
	value := i.saved()
	wf.cu.rememberInstruction(wf.handle, value)
	wf.SetNextInstID(value.ID)
}
