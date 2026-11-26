package cu

import (
	"math/bits"
	"slices"

	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// BankedLDSState owns the outstanding gfx942 bank reservations and returns.
type BankedLDSState struct {
	Waiting  []waveHandle
	BankFree timing.VTimeInPicoSec
	PortFree []timing.VTimeInPicoSec
	Returns  []LDSReturn
}

// LDSReturn saves the issued instruction and active lanes until writeback.
// Data is the concatenation of the active lanes' results in lane order.
type LDSReturn struct {
	Wave       waveHandle
	Inst, Exec uint64
	Ready      timing.VTimeInPicoSec
	Data       []byte
}

type cdna3LDSUnit struct {
	cu  *ComputeUnit
	alu emu.ALU
}

func (u *cdna3LDSUnit) state() *BankedLDSState { return &u.cu.comp.State.BankedLDS }
func (u *cdna3LDSUnit) CanAcceptWave() bool {
	return len(u.state().Waiting) < u.cu.comp.Spec.SIMDCount
}
func (u *cdna3LDSUnit) AcceptWave(w *Wavefront) {
	u.state().Waiting = append(u.state().Waiting, u.cu.useWave(w))
}
func (u *cdna3LDSUnit) IsIdle() bool {
	return len(u.state().Waiting) == 0 && len(u.state().Returns) == 0
}

func (u *cdna3LDSUnit) Run() bool {
	s := u.state()
	now := u.cu.CurrentTime()
	madeProgress := false
	for i := 0; i < len(s.Returns); {
		r := &s.Returns[i]
		if r.Ready > now {
			i++
			continue
		}
		u.complete(r)
		s.Returns = slices.Delete(s.Returns, i, i+1)
		madeProgress = true
	}

	if len(s.Waiting) == 0 {
		return madeProgress
	}
	if s.BankFree > now {
		u.cu.comp.TickAt(s.BankFree)
		return madeProgress
	}
	if s.PortFree == nil {
		s.PortFree = make([]timing.VTimeInPicoSec, (u.cu.comp.Spec.SIMDCount+1)/2)
	}
	for i, wave := range s.Waiting {
		w := u.cu.wave(wave)
		port := w.execution().SIMDID / 2
		if s.PortFree[port] > now {
			u.cu.comp.TickAt(s.PortFree[port])
			continue
		}
		u.issue(w, port)
		s.Waiting = slices.Delete(s.Waiting, i, i+1)
		return true
	}
	return madeProgress
}

type ldsCapture struct {
	*operationWave
	width int
	data  []byte
}

func (r *ldsCapture) WriteOperandBytes(_ *insts.Operand, lane int, data []byte) {
	index := bits.OnesCount64(r.EXEC() & (uint64(1)<<lane - 1))
	copy(r.data[index*r.width:][:r.width], data)
}

func (r *ldsCapture) WriteOperand(dst *insts.Operand, lane int, value uint64) {
	r.WriteOperandBytes(dst, lane, insts.Uint64ToBytes(value))
}

func (u *cdna3LDSUnit) issue(w *Wavefront, port int) {
	op := w.emulation()
	inst := op.Inst()
	access := inst.LDSAccess()
	var bases [64]uint32
	for active := op.EXEC(); active != 0; active &= active - 1 {
		lane := bits.TrailingZeros64(active)
		bases[lane] = uint32(op.ReadOperand(inst.Addr, lane))
	}
	rounds, replays := ldsBankRounds(access, op.EXEC(), &bases)
	portCycles := ldsPortCycles(inst.FormatType, access)
	now, freq := u.cu.CurrentTime(), u.cu.comp.Spec.Freq
	s := u.state()
	s.BankFree = freq.NCyclesLater(max(1, rounds), now)
	s.PortFree[port] = freq.NCyclesLater(portCycles, now)
	// Conflict-free bank phases overlap transport. Each replay adds one
	// bank cycle on the critical path. The remaining completion latency is
	// calibrated against the dependent MI300X shared FLAT loop.
	ready := freq.NCyclesLater(u.cu.comp.Spec.LDSReturnCycles+portCycles+replays, now)
	// Preserve each wave's issue order even across different transfer widths.
	for _, r := range s.Returns {
		if r.Wave == handleOf(w) {
			ready = max(ready, r.Ready)
		}
	}

	data := u.execute(w, op, access)
	s.Returns = append(s.Returns, LDSReturn{
		Wave: handleOf(w), Inst: op.InstID(), Exec: op.EXEC(), Ready: ready, Data: data,
	})
	w.execution().OutstandingScalarMemAccess++
	if inst.FormatType == insts.FLAT {
		w.execution().OutstandingVectorMemAccess++
	}
	u.cu.UpdatePCAndSetReady(w)
	u.cu.comp.TickAt(ready)
}

// Perform the memory operation in bank-grant order. Loads capture the
// value when granted and publish it to the original lanes on return.
func (u *cdna3LDSUnit) execute(w *Wavefront, op *operationWave, access insts.CDNA3LDSAccess) []byte {
	capture := &ldsCapture{operationWave: op}
	if !access.Write {
		capture.width = max(4, access.Bytes) * access.Count
		capture.data = make([]byte, bits.OnesCount64(op.EXEC())*capture.width)
	}
	u.alu.SetLDS(w.group().state().LDS)
	defer u.alu.SetLDS(nil)
	u.alu.Run(capture)
	return capture.data
}

// Model each SIMD pair's 64 B/cycle path. The address is a dword for DS,
// and a full 64-bit pointer for generic FLAT. Loads overlap address and data
// traffic; stores share the outgoing path. FLAT transport is an approximation.
func ldsPortCycles(format insts.FormatType, access insts.CDNA3LDSAccess) int {
	addressCycles := 4
	if format == insts.FLAT {
		addressCycles = 8
	}
	dataCycles := 4 * ((access.Bytes + 3) / 4) * access.Count
	if access.Write {
		return addressCycles + dataCycles
	}
	return max(addressCycles, dataCycles)
}

func (u *cdna3LDSUnit) complete(r *LDSReturn) {
	w := u.cu.wave(r.Wave)
	inst := u.cu.instruction(r.Inst)
	op := w.operation()
	if len(r.Data) > 0 {
		width := len(r.Data) / bits.OnesCount64(r.Exec)
		data := r.Data
		for active := r.Exec; active != 0; active &= active - 1 {
			op.WriteOperandBytes(inst.Dst, bits.TrailingZeros64(active), data[:width])
			data = data[width:]
		}
	}
	w.execution().OutstandingScalarMemAccess--
	if inst.FormatType == insts.FLAT {
		w.execution().OutstandingVectorMemAccess--
	}
	u.cu.markInstDataReturned(r.Inst, "lds")
	u.cu.completeInstruction(w, r.Inst)
}
