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
	operands   map[*insts.Operand]map[int]uint64
}

func newMockInstState() *mockInstState {
	return &mockInstState{
		inst:       insts.NewInst(),
		scratchpad: make([]byte, 4096),
		operands:   make(map[*insts.Operand]map[int]uint64),
	}
}

func (s *mockInstState) PID() vm.PID { return 1 }

func (s *mockInstState) Inst() *insts.Inst { return s.inst }

func (s *mockInstState) Scratchpad() emu.Scratchpad { return s.scratchpad }

func (s *mockInstState) ReadOperand(operand *insts.Operand, laneID int) uint64 {
	if lanes, ok := s.operands[operand]; ok {
		if v, ok := lanes[laneID]; ok {
			return v
		}
	}
	return 0
}

func (s *mockInstState) WriteOperand(operand *insts.Operand, laneID int, value uint64) {
	if s.operands[operand] == nil {
		s.operands[operand] = make(map[int]uint64)
	}
	s.operands[operand][laneID] = value
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

// setOperand sets an operand value for a specific lane.
func (s *mockInstState) setOperand(op *insts.Operand, lane int, value uint64) {
	if s.operands[op] == nil {
		s.operands[op] = make(map[int]uint64)
	}
	s.operands[op][lane] = value
}

func TestSOP1Opcode48SABSI32(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.SOP1
	state.inst.Opcode = 48
	state.inst.Src0 = &insts.Operand{}
	state.inst.Dst = &insts.Operand{}

	// Test negative input
	state.setOperand(state.inst.Src0, 0, uint64(emu.Int32ToBits(-7)))
	alu.Run(state)

	if got := emu.AsInt32(uint32(state.operands[state.inst.Dst][0])); got != 7 {
		t.Fatalf("expected abs(-7)=7, got %d", got)
	}
	if state.scc != 1 {
		t.Fatalf("expected SCC=1 for negative input, got %d", state.scc)
	}

	// Test non-negative input
	state.setOperand(state.inst.Src0, 0, uint64(emu.Int32ToBits(7)))
	alu.Run(state)
	if got := emu.AsInt32(uint32(state.operands[state.inst.Dst][0])); got != 7 {
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

	state.setOperand(state.inst.Src0, 0, emu.Int64ToBits(-8))
	state.setOperand(state.inst.Src1, 0, 2)

	alu.Run(state)

	if got := int64(state.operands[state.inst.Dst][0]); got != -2 {
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

	state.setOperand(state.inst.Src0, 0, 8)
	state.setOperand(state.inst.Src1, 0, 4)

	alu.Run(state)

	if state.operands[state.inst.Dst][0] != 0x00000FF0 {
		t.Fatalf("expected bitmask 0x00000FF0, got 0x%08x", uint32(state.operands[state.inst.Dst][0]))
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

	state.setOperand(state.inst.Src0, 0, 0xF0)
	state.setOperand(state.inst.Src1, 0, (4<<16)|4) // width=4, offset=4

	alu.Run(state)

	if state.operands[state.inst.Dst][0] != 0xF {
		t.Fatalf("expected unsigned extracted value 0xF, got 0x%x", state.operands[state.inst.Dst][0])
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

	state.setOperand(state.inst.Src0, 0, 0xF0)
	state.setOperand(state.inst.Src1, 0, (4<<16)|4) // width=4, offset=4 -> 0b1111 => -1 after sign extension

	alu.Run(state)

	if got := emu.AsInt32(uint32(state.operands[state.inst.Dst][0])); got != -1 {
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
	state.exec = 0xF

	state.setOperand(state.inst.Src0, 0, 11)
	state.setOperand(state.inst.Src0, 1, 22)
	state.setOperand(state.inst.Src0, 2, 33)
	state.setOperand(state.inst.Src0, 3, 44)

	alu.Run(state)

	if state.operands[state.inst.Dst][0] != 11 ||
		state.operands[state.inst.Dst][1] != 22 ||
		state.operands[state.inst.Dst][2] != 33 ||
		state.operands[state.inst.Dst][3] != 44 {
		t.Fatalf("unexpected movrelsd result: [%d %d %d %d]",
			state.operands[state.inst.Dst][0],
			state.operands[state.inst.Dst][1],
			state.operands[state.inst.Dst][2],
			state.operands[state.inst.Dst][3])
	}
}
