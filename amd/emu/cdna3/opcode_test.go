package cdna3

import (
	"testing"

	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/mgpusim/v4/amd/emu"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

type mockInstState struct {
	inst       *insts.Inst
	scratchpad emu.Scratchpad
	exec       uint64
	vcc        uint64
	scc        byte
	pc         uint64
	operands   map[*insts.Operand]uint64
}

func newMockInstState() *mockInstState {
	return &mockInstState{
		inst:       insts.NewInst(),
		scratchpad: make([]byte, 4096),
		operands:   make(map[*insts.Operand]uint64),
	}
}

func (s *mockInstState) PID() vm.PID { return 1 }

func (s *mockInstState) Inst() *insts.Inst { return s.inst }

func (s *mockInstState) Scratchpad() emu.Scratchpad { return s.scratchpad }

func (s *mockInstState) ReadOperand(operand *insts.Operand, laneID int) uint64 {
	if v, ok := s.operands[operand]; ok {
		return v
	}
	return 0
}

func (s *mockInstState) WriteOperand(operand *insts.Operand, laneID int, value uint64) {
	s.operands[operand] = value
}

func (s *mockInstState) ReadOperandBytes(operand *insts.Operand, laneID int, byteCount int) []byte {
	panic("not implemented")
}

func (s *mockInstState) WriteOperandBytes(operand *insts.Operand, laneID int, data []byte) {
	panic("not implemented")
}

func (s *mockInstState) EXEC() uint64     { return s.exec }
func (s *mockInstState) SetEXEC(v uint64) { s.exec = v }
func (s *mockInstState) VCC() uint64      { return s.vcc }
func (s *mockInstState) SetVCC(v uint64)  { s.vcc = v }
func (s *mockInstState) SCC() byte        { return s.scc }
func (s *mockInstState) SetSCC(v byte)    { s.scc = v }
func (s *mockInstState) PC() uint64       { return s.pc }
func (s *mockInstState) SetPC(v uint64)   { s.pc = v }

func TestSOP1Opcode48SABSI32(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.SOP1
	state.inst.Opcode = 48
	state.inst.Src0 = &insts.Operand{}
	state.inst.Dst = &insts.Operand{}

	// Test negative input
	state.operands[state.inst.Src0] = uint64(emu.Int32ToBits(-7))
	alu.Run(state)

	if got := emu.AsInt32(uint32(state.operands[state.inst.Dst])); got != 7 {
		t.Fatalf("expected abs(-7)=7, got %d", got)
	}
	if state.scc != 1 {
		t.Fatalf("expected SCC=1 for negative input, got %d", state.scc)
	}

	// Test non-negative input
	state.operands[state.inst.Src0] = uint64(emu.Int32ToBits(7))
	alu.Run(state)
	if got := emu.AsInt32(uint32(state.operands[state.inst.Dst])); got != 7 {
		t.Fatalf("expected abs(7)=7, got %d", got)
	}
	if state.scc != 0 {
		t.Fatalf("expected SCC=0 for non-negative input, got %d", state.scc)
	}
}

func TestSOP2Opcode33SASHRI64(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.SOP2
	state.inst.Opcode = 33
	state.inst.Src0 = &insts.Operand{}
	state.inst.Src1 = &insts.Operand{}
	state.inst.Dst = &insts.Operand{}

	state.operands[state.inst.Src0] = emu.Int64ToBits(-8)
	state.operands[state.inst.Src1] = 2

	alu.Run(state)

	if got := int64(state.operands[state.inst.Dst]); got != -2 {
		t.Fatalf("expected -8 >> 2 = -2, got %d", got)
	}
	if state.scc != 1 {
		t.Fatalf("expected SCC=1 for non-zero result, got %d", state.scc)
	}
}

func TestSOP2Opcode34SBFMB32(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.SOP2
	state.inst.Opcode = 34
	state.inst.Src0 = &insts.Operand{}
	state.inst.Src1 = &insts.Operand{}
	state.inst.Dst = &insts.Operand{}

	state.operands[state.inst.Src0] = 8
	state.operands[state.inst.Src1] = 4

	alu.Run(state)

	if state.operands[state.inst.Dst] != 0x00000FF0 {
		t.Fatalf("expected bitmask 0x00000FF0, got 0x%08x", uint32(state.operands[state.inst.Dst]))
	}
}

func TestSOP2Opcode37SBFEU32(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.SOP2
	state.inst.Opcode = 37
	state.inst.Src0 = &insts.Operand{}
	state.inst.Src1 = &insts.Operand{}
	state.inst.Dst = &insts.Operand{}

	state.operands[state.inst.Src0] = 0xF0
	state.operands[state.inst.Src1] = (4 << 16) | 4 // width=4, offset=4

	alu.Run(state)

	if state.operands[state.inst.Dst] != 0xF {
		t.Fatalf("expected unsigned extracted value 0xF, got 0x%x", state.operands[state.inst.Dst])
	}
	if state.scc != 1 {
		t.Fatalf("expected SCC=1 for non-zero result, got %d", state.scc)
	}
}

func TestSOP2Opcode38SBFEI32(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.SOP2
	state.inst.Opcode = 38
	state.inst.Src0 = &insts.Operand{}
	state.inst.Src1 = &insts.Operand{}
	state.inst.Dst = &insts.Operand{}

	state.operands[state.inst.Src0] = 0xF0
	state.operands[state.inst.Src1] = (4 << 16) | 4 // width=4, offset=4 -> 0b1111 => -1 after sign extension

	alu.Run(state)

	if got := emu.AsInt32(uint32(state.operands[state.inst.Dst])); got != -1 {
		t.Fatalf("expected signed extracted value -1, got %d", got)
	}
	if state.scc != 1 {
		t.Fatalf("expected SCC=1 for non-zero result, got %d", state.scc)
	}
}

func TestVOP1Opcode56VMOVRELSDB32(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.VOP1
	state.inst.Opcode = 56

	sp := state.scratchpad.AsVOP1()
	sp.EXEC = 0xF
	sp.SRC0[0], sp.SRC0[1], sp.SRC0[2], sp.SRC0[3] = 11, 22, 33, 44

	alu.Run(state)

	if sp.DST[0] != 11 || sp.DST[1] != 22 || sp.DST[2] != 33 || sp.DST[3] != 44 {
		t.Fatalf("unexpected movrelsd result: [%d %d %d %d]",
			sp.DST[0], sp.DST[1], sp.DST[2], sp.DST[3])
	}
}
