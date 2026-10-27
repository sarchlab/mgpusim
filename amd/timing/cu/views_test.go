package cu

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/sim/tasks"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// Reuse the decoder backing array deliberately: State replacement must work
// independently of allocator behavior, load hooks, middleware access, and ticks.
//
//nolint:musttag // State uses the default field names of Akita's checkpoint codec.
func replaceCUState(t *testing.T, c *ComputeUnit) State {
	t.Helper()
	old := c.comp.State
	data, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	var next State
	if err := json.Unmarshal(data, &next); err != nil {
		t.Fatal(err)
	}
	next.Decoders = old.Decoders
	c.comp.State = next
	return old
}

func TestViewsResolveReplacedStateWithoutLifecycleHook(t *testing.T) {
	spec := DefaultSpec()
	spec.RegisterScoreboard = true
	c := checkpointCU(t, spec)
	g := checkpointGroup(c)
	w := g.waves()[0]
	i := checkpointInstruction(t, c)
	w.setTestInst(i)
	accessor := w.regAccessor()
	issue := NewIssueArbiter(c)
	old := replaceCUState(t, c)
	s := &c.comp.State
	s.Waves[slotIndex(g.state().Waves[0])].Execution.PC = 128
	s.Waves[slotIndex(g.state().Waves[0])].Execution.SRegOffset = 16
	s.Waves[slotIndex(g.state().Waves[0])].Execution.VRegOffset = 32
	s.Waves[slotIndex(g.state().Waves[0])].Scoreboard.VGPRBusyUntil[3] = 17
	s.SRegisters[16] = 43
	s.VRegisters[0][32] = 47
	s.Groups[g.slot].LDS = []byte{53}
	changed := s.Instructions[i.ID]
	changed.Raw = []byte{1, 0, 0x80, 0xbf}
	s.Instructions[i.ID] = changed
	s.Scheduler.LastSIMD = 3
	if w.PC() != 128 || w.scoreboard().VGPRBusyUntil[3] != 17 || g.state().LDS[0] != 53 || issue.state().LastSIMD != 3 {
		t.Fatal("view retained pre-replacement state")
	}
	if w.ReadOperand(insts.NewSRegOperand(0, 0, 1), 0) != 43 || w.ReadOperand(insts.NewVRegOperand(0, 0, 1), 0) != 47 {
		t.Fatal("operand read retained old register storage/offset")
	}
	if w.Inst().SImm16.IntValue != 1 {
		t.Fatal("instruction lookup retained old bytes for the same dynamic ID")
	}
	accessor.WriteReg(insts.Regs[insts.M0], 1, 0, 0, insts.Uint32ToBytes(59))
	w.WriteOperand(insts.NewSRegOperand(0, 0, 1), 0, 61)
	w.WriteOperandBytes(insts.NewVRegOperand(0, 0, 1), 0, insts.Uint32ToBytes(67))
	w.scoreboard().VGPRBusyUntil[3] = 71
	g.state().LDS[0] = 73
	if w.execution().M0 != 59 ||
		s.SRegisters[16] != 61 ||
		s.VRegisters[0][32] != 67 ||
		s.Waves[slotIndex(g.state().Waves[0])].Scoreboard.VGPRBusyUntil[3] != 71 {
		t.Fatal("view writes did not reach canonical replacement state")
	}
	if old.Waves[slotIndex(w.handle)].Execution.M0 != 0 ||
		old.SRegisters[16] != 0 ||
		old.VRegisters[0][32] != 0 ||
		old.Waves[slotIndex(w.handle)].Scoreboard.VGPRBusyUntil[3] != 0 {
		t.Fatal("view write mutated detached state")
	}
	// Another view must resolve the same canonical LDS through its group ID.
	g2 := w.group()
	if &g2.state().LDS[0] != &g.state().LDS[0] {
		t.Fatal("group views do not share canonical LDS")
	}
}

func TestCustomCompletionEventResolvesReplacedState(t *testing.T) {
	c := checkpointCU(t)
	g := checkpointGroup(c)
	w := g.waves()[0]
	w.execution().State = wavefront.WfSampledWaiting
	c.WfPools[0].AddWf(w)
	c.comp.State.WFTasks[w.UID] = 0
	c.comp.State.WGTasks[g.state().Request.ID] = 0
	old := replaceCUState(t, c)
	port := newFakePort("CU.Top")
	c.ToACE = port
	c.Handle(wavefront.NewWfCompletionEvent(c.comp.NewID(), 200, c.wfCompletionHandlerID, handleOf(w)))
	if len(port.sent) != 1 || liveTestGroups(&c.comp.State) != 0 || liveTestWaves(&c.comp.State) != 0 {
		t.Fatal("custom completion did not retire replacement state")
	}
	if old.Waves[slotIndex(w.handle)].Execution.State != wavefront.WfSampledWaiting {
		t.Fatal("custom completion mutated detached state")
	}
	if c.findWavefront(w.UID) != nil {
		t.Fatal("retired wave still resolves")
	}
}

func TestLDSExecutionDoesNotRetainStateStorage(t *testing.T) {
	c := checkpointCU(t)
	g := checkpointGroup(c)
	w := g.waves()[0]
	group := c.comp.State.Groups[g.slot]
	group.LDS = []byte{79}
	c.comp.State.Groups[g.slot] = group
	alu := &ldsViewProbe{}
	u := NewLDSUnit(c, alu)
	c.comp.State.LDS.Exec = handleOf(w)
	old := replaceCUState(t, c)
	c.comp.State.Groups[g.slot].LDS[0] = 83
	if !u.Run() || alu.seen != 83 || alu.LDS() != nil {
		t.Fatal("LDS operation retained or read stale State storage")
	}
	if old.Groups[g.slot].LDS[0] != 79 {
		t.Fatal("LDS operation mutated detached state")
	}
}

type ldsViewProbe struct {
	mockALU
	seen byte
	lds  []byte
}

func (a *ldsViewProbe) SetLDS(v []byte)        { a.lds = v }
func (a *ldsViewProbe) LDS() []byte            { return a.lds }
func (a *ldsViewProbe) Run(_ emu.InstEmuState) { a.seen = a.lds[0]; a.lds[0]++ }

func TestMemoryResponsesResolveReplacedState(t *testing.T) {
	c := checkpointCU(t)
	g := checkpointGroup(c)
	w := g.waves()[0]
	i := checkpointInstruction(t, c)
	w.setTestInst(i)
	w.execution().OutstandingScalarMemAccess = 1
	w.execution().OutstandingVectorMemAccess = 1
	w.execution().InFlightInsts = 2
	scalarReq := messaging.Msg{ID: c.comp.NewID(), Payload: memprotocol.ReadReq{}}
	vectorReq := messaging.Msg{ID: c.comp.NewID(), Payload: memprotocol.ReadReq{}}
	c.comp.State.Scalar = []ScalarAccessState{
		{Req: scalarReq, WF: handleOf(w), Inst: i.ID, Register: insts.SReg(1).RegType},
	}
	c.comp.State.Vector = []VectorAccessState{{Read: vectorReq, WF: handleOf(w), Inst: i.ID,
		Lanes: []LaneState{{Lane: 0, Register: insts.VReg(1).RegType, Count: 1}}}}
	old := replaceCUState(t, c)
	w.execution().SRegOffset = 16
	w.execution().VRegOffset = 32
	c.handleScalarDataLoadReturn(messaging.Msg{RspTo: scalarReq.ID,
		Payload: memprotocol.DataReadyRsp{Data: insts.Uint32ToBytes(89)}})
	c.handleVectorDataLoadReturn(messaging.Msg{RspTo: vectorReq.ID,
		Payload: memprotocol.DataReadyRsp{Data: insts.Uint32ToBytes(97)}})
	if c.comp.State.SRegisters[20] != 89 ||
		c.comp.State.VRegisters[0][36] != 97 ||
		w.execution().OutstandingScalarMemAccess != 0 ||
		w.execution().OutstandingVectorMemAccess != 0 ||
		w.execution().InFlightInsts != 0 {
		t.Fatal("memory completion did not use replacement wave/register state")
	}
	if old.SRegisters[20] != 0 ||
		old.VRegisters[0][36] != 0 ||
		old.Waves[slotIndex(w.handle)].Execution.OutstandingScalarMemAccess != 1 ||
		old.Waves[slotIndex(w.handle)].Execution.OutstandingVectorMemAccess != 1 {
		t.Fatal("memory completion mutated detached state")
	}
}

func TestDecodeCacheUsesEncodingAndISAWithoutDynamicIdentity(t *testing.T) {
	c := checkpointCU(t)
	first := checkpointInstruction(t, c)
	first.PC = 128
	c.rememberInstruction(0, first.saved())
	second := checkpointInstruction(t, c)
	second.PC = 128
	c.rememberInstruction(0, second.saved())
	a := c.instruction(first.ID)
	if first.ID == second.ID || a != c.instruction(second.ID) || a.PC != 128 {
		t.Fatal("equal descriptions did not share a pointer independently of dynamic IDs")
	}
	if n := testing.AllocsPerRun(1000, func() { _ = c.instruction(first.ID) }); n != 0 {
		t.Fatalf("cached instruction lookup allocated %g times", n)
	}
	replaceCUState(t, c)
	record := c.comp.State.Instructions[first.ID]
	record.PC = 256
	c.comp.State.Instructions[first.ID] = record
	b := c.instruction(first.ID)
	if b == a || b.PC != 256 || a.PC != 128 || c.instruction(second.ID) != a {
		t.Fatal("canonical PC replacement changed a shared description")
	}
	record.Raw = []byte{1, 0, 0x80, 0xbf}
	c.comp.State.Instructions[first.ID] = record
	changed := c.instruction(first.ID)
	if changed == b || changed.SImm16.IntValue != 1 || b.SImm16.IntValue != 0 {
		t.Fatal("same dynamic ID did not resolve changed encoding")
	}
	c.Decoder.(*insts.Disassembler).IsCDNA3 = true
	mode := c.instruction(first.ID)
	if mode == changed || mode.SImm16.IntValue != 1 {
		t.Fatal("decode cache ignored ISA configuration")
	}
	c.Decoder = insts.NewDisassembler()
	replacement := c.instruction(first.ID)
	if replacement == mode || replacement.SImm16.IntValue != 1 {
		t.Fatal("decode cache ignored decoder replacement")
	}
}

func TestCustomDecoderBypassesContentCache(t *testing.T) {
	c := checkpointCU(t)
	i := checkpointInstruction(t, c)
	c.rememberInstruction(0, i.saved())
	decoder := &changingDecoder{}
	c.Decoder = decoder
	if c.instruction(i.ID).ByteSize != 1 || c.instruction(i.ID).ByteSize != 2 || len(c.decoded) != 0 {
		t.Fatal("custom decoder context was cached")
	}
}

type changingDecoder struct{ calls int }

func (d *changingDecoder) Decode(_ []byte) (*insts.Inst, error) {
	d.calls++
	i := insts.NewInst()
	i.ByteSize = d.calls
	return i, nil
}

func TestDebuggerRetainsIDsAndResolvesReplacementAtCompletion(t *testing.T) {
	c := newTestComputeUnit("CU", nil)
	c.Decoder = insts.NewDisassembler()
	g := checkpointGroup(c)
	w := g.waves()[0]
	group := c.comp.State.Groups[g.slot]
	group.Raw.IDX = 75
	group.Raw.IDY = 1
	c.comp.State.Groups[g.slot] = group
	i := checkpointInstruction(t, c)
	w.setTestInst(i)
	var output bytes.Buffer
	h := &ISADebugger{Logger: log.New(&output, "", 0), cu: c, isFirstEntry: true,
		executingInst: make(map[uint64]tasks.Task)}
	c.comp.TaskTracker().Subscribe(h, c.comp)
	c.beginInstruction(w, w.InstID())
	if len(h.executingInst) != 1 {
		t.Fatal("debugger did not track instruction")
	}
	for _, task := range h.executingInst {
		detail := task.Detail.(map[string]interface{})
		if detail["wf"] != w.UID || detail["inst"] != i.ID || detail["format"] != "sopp" {
			t.Fatal("tracing detail retains execution objects instead of IDs/metadata")
		}
	}
	old := replaceCUState(t, c)
	w.SetPC(256)
	c.completeInstruction(w, w.InstID())
	if !strings.Contains(output.String(), `"PCLo":256`) || len(h.executingInst) != 0 {
		t.Fatalf("debugger used stale State or retained completed task: %s", output.String())
	}
	if old.Waves[slotIndex(w.handle)].Execution.InFlightInsts != 1 {
		t.Fatal("completion mutated old state")
	}
}

func TestGroupRetirementPreservesFixedSlotsAndSurvivingViews(t *testing.T) {
	c := checkpointCU(t)
	c.ToACE = newFakePort("CU.ACE")
	groups := []*WorkGroup{checkpointGroup(c), checkpointGroup(c), checkpointGroup(c)}
	waves := []*Wavefront{groups[0].waves()[0], groups[1].waves()[0], groups[2].waves()[0]}
	for n, w := range waves {
		w.SetPC(uint64(n + 1))
		w.setTestInst(checkpointInstruction(t, c))
	}
	waves[1].execution().State = wavefront.WfCompleted
	c.retireGroups()
	if liveTestGroups(&c.comp.State) != 2 ||
		liveTestWaves(&c.comp.State) != 2 ||
		len(c.comp.State.Instructions) != 2 {
		t.Fatal("middle group retirement did not remove canonical values")
	}
	if c.findWavefront(waves[1].UID) != nil ||
		groups[2].slot != 2 ||
		waves[2].state().Group != 2 {
		t.Fatal("surviving group/wave slot moved")
	}
	if waves[0].PC() != 1 || waves[2].PC() != 3 || groups[2].waves()[0].UID != waves[2].UID {
		t.Fatal("surviving handles lost identity during retirement")
	}
	old := replaceCUState(t, c)
	waves[0].SetPC(101)
	waves[2].SetPC(303)
	if groups[0].waves()[0].PC() != 101 ||
		groups[2].waves()[0].PC() != 303 ||
		old.Waves[slotIndex(waves[0].handle)].Execution.PC != 1 ||
		old.Waves[slotIndex(waves[2].handle)].Execution.PC != 3 {
		t.Fatal("surviving handles did not resolve State replacement after retirement")
	}
	waves[0].execution().State = wavefront.WfCompleted
	waves[2].execution().State = wavefront.WfCompleted
	c.retireGroups()
	if liveTestGroups(&c.comp.State) != 0 ||
		liveTestWaves(&c.comp.State) != 0 ||
		len(c.comp.State.Instructions) != 0 {
		t.Fatal("retirement retained occupied slots")
	}
}

func TestInstructionSelectionRejectsMissingAndForeignOwners(t *testing.T) {
	c := checkpointCU(t)
	first, second := checkpointGroup(c).waves()[0], checkpointGroup(c).waves()[0]
	i := checkpointInstruction(t, c)
	first.setTestInst(i)
	requireSlotPanic(t, func() { second.SetInstID(i.ID) })
	requireSlotPanic(t, func() { second.SetNextInstID(i.ID) })
	requireSlotPanic(t, func() { first.SetInstID(c.comp.NewID()) })
	if first.InstID() != i.ID || second.InstID() != 0 {
		t.Fatal("rejected selection changed canonical state")
	}
}
