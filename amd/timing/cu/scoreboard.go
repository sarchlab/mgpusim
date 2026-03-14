package cu

import (
	"strings"

	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// Scoreboard tracks per-wavefront register availability for hazard detection.
// Each entry stores the number of cycles until the register becomes available.
type Scoreboard struct {
	VGPRBusyUntil [256]int // cycle counter for each VGPR
	SGPRBusyUntil [102]int // cycle counter for each SGPR
	SCCBusyUntil  int
	VCCBusyUntil  int
	EXECBusyUntil int
}

// NewScoreboard creates a new Scoreboard with all counters at zero.
func NewScoreboard() *Scoreboard {
	return &Scoreboard{}
}

// Tick decrements all non-zero counters by 1 each cycle.
func (s *Scoreboard) Tick() {
	for i := range s.VGPRBusyUntil {
		if s.VGPRBusyUntil[i] > 0 {
			s.VGPRBusyUntil[i]--
		}
	}

	for i := range s.SGPRBusyUntil {
		if s.SGPRBusyUntil[i] > 0 {
			s.SGPRBusyUntil[i]--
		}
	}

	if s.SCCBusyUntil > 0 {
		s.SCCBusyUntil--
	}

	if s.VCCBusyUntil > 0 {
		s.VCCBusyUntil--
	}

	if s.EXECBusyUntil > 0 {
		s.EXECBusyUntil--
	}
}

// MarkBusy marks destination registers of the instruction as busy for the
// given number of cycles. It examines Dst (VGPR/SGPR), SDst, and implicit
// SCC/VCC writes.
func (s *Scoreboard) MarkBusy(inst *insts.Inst, latency int) {
	if latency <= 0 {
		return
	}

	s.markOperandBusy(inst.Dst, latency)
	s.markOperandBusy(inst.SDst, latency)

	// Scalar ALU instructions implicitly write SCC.
	if inst.ExeUnit == insts.ExeUnitScalar {
		s.SCCBusyUntil = max(s.SCCBusyUntil, latency)
	}

	// VOPC comparison instructions implicitly write VCC.
	if inst.Format != nil && inst.FormatType == insts.VOPC {
		s.VCCBusyUntil = max(s.VCCBusyUntil, latency)
	}
}

func (s *Scoreboard) markOperandBusy(op *insts.Operand, latency int) {
	if op == nil || op.OperandType != insts.RegOperand || op.Register == nil {
		return
	}

	reg := op.Register
	regCount := op.RegCount
	if regCount < 1 {
		regCount = 1
	}

	if reg.IsVReg() {
		base := reg.RegIndex()
		for i := 0; i < regCount && base+i < 256; i++ {
			s.VGPRBusyUntil[base+i] = max(s.VGPRBusyUntil[base+i], latency)
		}
		return
	}

	if reg.IsSReg() {
		base := reg.RegIndex()
		for i := 0; i < regCount && base+i < 102; i++ {
			s.SGPRBusyUntil[base+i] = max(s.SGPRBusyUntil[base+i], latency)
		}
		return
	}

	switch reg.RegType {
	case insts.SCC:
		s.SCCBusyUntil = max(s.SCCBusyUntil, latency)
	case insts.VCC, insts.VCCLO, insts.VCCHI:
		s.VCCBusyUntil = max(s.VCCBusyUntil, latency)
	case insts.EXEC, insts.EXECLO, insts.EXECHI:
		s.EXECBusyUntil = max(s.EXECBusyUntil, latency)
	}
}

// HasHazard checks if any source operand reads a register that is still busy.
func (s *Scoreboard) HasHazard(inst *insts.Inst) bool {
	operands := []*insts.Operand{
		inst.Src0, inst.Src1, inst.Src2,
		inst.Addr, inst.Data, inst.Base, inst.Offset,
	}

	for _, op := range operands {
		if s.operandHasHazard(op) {
			return true
		}
	}

	return false
}

func (s *Scoreboard) operandHasHazard(op *insts.Operand) bool {
	if op == nil || op.OperandType != insts.RegOperand || op.Register == nil {
		return false
	}

	reg := op.Register
	regCount := op.RegCount
	if regCount < 1 {
		regCount = 1
	}

	if reg.IsVReg() {
		base := reg.RegIndex()
		for i := 0; i < regCount && base+i < 256; i++ {
			if s.VGPRBusyUntil[base+i] > 0 {
				return true
			}
		}
		return false
	}

	if reg.IsSReg() {
		base := reg.RegIndex()
		for i := 0; i < regCount && base+i < 102; i++ {
			if s.SGPRBusyUntil[base+i] > 0 {
				return true
			}
		}
		return false
	}

	switch reg.RegType {
	case insts.SCC:
		return s.SCCBusyUntil > 0
	case insts.VCC, insts.VCCLO, insts.VCCHI:
		return s.VCCBusyUntil > 0
	case insts.EXEC, insts.EXECLO, insts.EXECHI:
		return s.EXECBusyUntil > 0
	}

	return false
}

// AnyBusy returns true if any register counter is still > 0.
func (s *Scoreboard) AnyBusy() bool {
	for _, v := range s.VGPRBusyUntil {
		if v > 0 {
			return true
		}
	}
	for _, v := range s.SGPRBusyUntil {
		if v > 0 {
			return true
		}
	}
	return s.SCCBusyUntil > 0 || s.VCCBusyUntil > 0 || s.EXECBusyUntil > 0
}

// Clear resets all counters to 0.
func (s *Scoreboard) Clear() {
	s.VGPRBusyUntil = [256]int{}
	s.SGPRBusyUntil = [102]int{}
	s.SCCBusyUntil = 0
	s.VCCBusyUntil = 0
	s.EXECBusyUntil = 0
}

// GetScoreboardLatency returns the scoreboard latency for an instruction based
// on its execution unit. LDS/VMem instructions return 0 (not tracked by
// scoreboard; handled by s_waitcnt).
func GetScoreboardLatency(inst *insts.Inst) int {
	switch inst.ExeUnit {
	case insts.ExeUnitVALU:
		if strings.Contains(inst.InstName, "f64") {
			return 8
		}
		return 4
	case insts.ExeUnitScalar:
		return 2
	case insts.ExeUnitBranch:
		return 3
	default:
		return 0 // LDS/VMem/Special - don't track
	}
}
