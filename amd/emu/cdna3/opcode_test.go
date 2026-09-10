package cdna3

import (
	"math"
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

type mockInstState struct {
	inst     *insts.Inst
	exec     uint64
	vcc      uint64
	scc      byte
	pc       uint64
	operands map[*insts.Operand]map[int]uint64
}

func newMockInstState() *mockInstState {
	return &mockInstState{
		inst:     insts.NewInst(),
		operands: make(map[*insts.Operand]map[int]uint64),
	}
}

func (s *mockInstState) PID() vm.PID { return 1 }

func (s *mockInstState) Inst() *insts.Inst { return s.inst }

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

func TestVOP3aDivFixupF32SpecialValues(t *testing.T) {
	alu := NewALU(nil)
	state := newMockInstState()
	state.inst.FormatType = insts.VOP3a
	state.inst.Opcode = 478
	state.inst.InstName = "v_div_fixup_f32"
	state.inst.Src0 = &insts.Operand{}
	state.inst.Src1 = &insts.Operand{}
	state.inst.Src2 = &insts.Operand{}
	state.inst.Dst = &insts.Operand{}

	testCases := []struct {
		name               string
		quotient           uint32
		denominator        uint32
		numerator          uint32
		expectedResultBits uint32
	}{
		{
			name:     "normal quotient keeps the numerator-over-denominator sign",
			quotient: math.Float32bits(-1.5), denominator: math.Float32bits(2),
			numerator: math.Float32bits(3), expectedResultBits: math.Float32bits(1.5),
		},
		{
			name:     "negative normal quotient",
			quotient: math.Float32bits(1.5), denominator: math.Float32bits(-2),
			numerator: math.Float32bits(3), expectedResultBits: math.Float32bits(-1.5),
		},
		{
			name:     "positive zero numerator",
			quotient: math.Float32bits(9), denominator: math.Float32bits(2),
			numerator: 0x00000000, expectedResultBits: 0x00000000,
		},
		{
			name:     "negative zero numerator",
			quotient: math.Float32bits(9), denominator: math.Float32bits(2),
			numerator: 0x80000000, expectedResultBits: 0x80000000,
		},
		{
			name:     "finite numerator divided by negative zero",
			quotient: math.Float32bits(9), denominator: 0x80000000,
			numerator: math.Float32bits(3), expectedResultBits: 0xff800000,
		},
		{
			name:     "zero divided by zero is indeterminate",
			quotient: math.Float32bits(9), denominator: 0x00000000,
			numerator: 0x80000000, expectedResultBits: 0xffc00000,
		},
		{
			name:     "infinity divided by infinity is indeterminate",
			quotient: math.Float32bits(9), denominator: 0xff800000,
			numerator: 0x7f800000, expectedResultBits: 0xffc00000,
		},
		{
			name:     "finite numerator divided by negative infinity",
			quotient: math.Float32bits(9), denominator: 0xff800000,
			numerator: math.Float32bits(3), expectedResultBits: 0x80000000,
		},
		{
			name:     "negative infinity divided by finite denominator",
			quotient: math.Float32bits(9), denominator: math.Float32bits(2),
			numerator: 0xff800000, expectedResultBits: 0xff800000,
		},
		{
			name:     "numerator signaling NaN takes precedence and is quieted",
			quotient: math.Float32bits(9), denominator: 0x7f800321,
			numerator: 0xff800123, expectedResultBits: 0xffc00123,
		},
		{
			name:     "denominator signaling NaN is quieted",
			quotient: math.Float32bits(9), denominator: 0x7f800321,
			numerator: math.Float32bits(3), expectedResultBits: 0x7fc00321,
		},
		{
			name:     "result below the representable range underflows to signed zero",
			quotient: math.Float32bits(9), denominator: 0x7f7fffff,
			numerator: 0x80800000, expectedResultBits: 0x80000000,
		},
	}

	state.exec = (uint64(1) << uint(len(testCases))) - 1
	for lane, tc := range testCases {
		state.setOperand(state.inst.Src0, lane, uint64(tc.quotient))
		state.setOperand(state.inst.Src1, lane, uint64(tc.denominator))
		state.setOperand(state.inst.Src2, lane, uint64(tc.numerator))
	}

	disabledLane := len(testCases)
	const disabledLaneSentinel = uint64(0x12345678)
	state.setOperand(state.inst.Src0, disabledLane, f32bits(1))
	state.setOperand(state.inst.Src1, disabledLane, f32bits(1))
	state.setOperand(state.inst.Src2, disabledLane, f32bits(1))
	state.setOperand(state.inst.Dst, disabledLane, disabledLaneSentinel)

	alu.Run(state)

	for lane, tc := range testCases {
		if got := uint32(state.operands[state.inst.Dst][lane]); got != tc.expectedResultBits {
			t.Errorf("%s: expected %#08x, got %#08x",
				tc.name, tc.expectedResultBits, got)
		}
	}
	if got := state.operands[state.inst.Dst][disabledLane]; got != disabledLaneSentinel {
		t.Errorf("disabled EXEC lane changed: expected %#08x, got %#08x",
			disabledLaneSentinel, got)
	}
}
