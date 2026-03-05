package emu

import (
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// ScratchpadPreparer is the unit that sets the instruction scratchpad
// before the instruction can be emulated.
type ScratchpadPreparer interface {
	// Prepare reads from the register file and write into the instruction
	// scratchpad
	Prepare(instEmuState InstEmuState, wf *Wavefront)

	// Commit write to the register file to reflect the change in the scratchpad
	Commit(instEmuState InstEmuState, wf *Wavefront)
}

// ScratchpadPreparerImpl reads and write registers for the emulator
type ScratchpadPreparerImpl struct{}

// NewScratchpadPreparerImpl returns a newly created ScratchpadPreparerImpl,
// injecting the dependency of the RegInterface.
func NewScratchpadPreparerImpl(isCDNA3 bool) *ScratchpadPreparerImpl {
	p := new(ScratchpadPreparerImpl)
	return p
}

// Prepare read from the register file and sets the scratchpad layout
//
//nolint:gocyclo
func (p *ScratchpadPreparerImpl) Prepare(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	p.clear(instEmuState.Scratchpad())
	inst := instEmuState.Inst()
	switch inst.FormatType {
	case insts.SOP1:
		p.prepareSOP1(instEmuState, wf)
	case insts.SOP2:
		p.prepareSOP2(instEmuState, wf)
	case insts.SOPC:
		p.prepareSOPC(instEmuState, wf)
	case insts.VOP1:
		p.prepareVOP1(instEmuState, wf)
	case insts.VOP2:
		p.prepareVOP2(instEmuState, wf)
	case insts.VOP3a:
		p.prepareVOP3a(instEmuState, wf)
	case insts.VOP3b:
		p.prepareVOP3b(instEmuState, wf)
	case insts.VOPC:
		p.prepareVOPC(instEmuState, wf)
	case insts.FLAT:
		p.prepareFlat(instEmuState, wf)
	case insts.SMEM:
		p.prepareSMEM(instEmuState, wf)
	case insts.SOPP:
		p.prepareSOPP(instEmuState, wf)
	case insts.SOPK:
		p.prepareSOPK(instEmuState, wf)
	case insts.DS:
		p.prepareDS(instEmuState, wf)
	default:
		log.Panicf("Inst format %s is not supported", inst.Format.FormatName)
	}
}

func (p *ScratchpadPreparerImpl) prepareSOP1(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	// SOP1 instructions now read directly via ReadOperand/SCC/EXEC/PC.
	// No scratchpad preparation needed.
}

func (p *ScratchpadPreparerImpl) prepareSOP2(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	// SOP2 instructions now read directly via ReadOperand/SCC.
	// No scratchpad preparation needed.
}

func (p *ScratchpadPreparerImpl) prepareVOP1(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	sp := instEmuState.Scratchpad()

	copy(sp[0:8], wf.ReadReg(insts.Regs[insts.EXEC], 1, 0))
	copy(sp[520:528], wf.ReadReg(insts.Regs[insts.VCC], 1, 0))

	offset := 528
	for i := 0; i < 64; i++ {
		p.readOperand(inst.Src0, wf, i, sp[offset:offset+8])
		offset += 8
	}
}

func (p *ScratchpadPreparerImpl) prepareVOP2(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	sp := instEmuState.Scratchpad()
	copy(sp[0:8], wf.ReadReg(insts.Regs[insts.EXEC], 1, 0))
	copy(sp[520:528], wf.ReadReg(insts.Regs[insts.VCC], 1, 0))

	if inst.Src2 != nil {
		p.readOperand(inst.Src2, wf, 0, sp[1552:1560])
	}

	dstOffset := 8
	src0Offset := 528
	src1Offset := 1040
	for i := 0; i < 64; i++ {
		p.readOperand(inst.Dst, wf, i, sp[dstOffset:dstOffset+8])
		dstOffset += 8
		p.readOperand(inst.Src0, wf, i, sp[src0Offset:src0Offset+8])
		src0Offset += 8
		p.readOperand(inst.Src1, wf, i, sp[src1Offset:src1Offset+8])
		src1Offset += 8
	}
}

func (p *ScratchpadPreparerImpl) prepareVOP3a(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	sp := instEmuState.Scratchpad()

	copy(sp[0:8], wf.ReadReg(insts.Regs[insts.EXEC], 1, 0))
	copy(sp[520:528], wf.ReadReg(insts.Regs[insts.VCC], 1, 0))

	src0Offset := 528
	src1Offset := 1040
	src2Offset := 1552
	for i := 0; i < 64; i++ {
		p.readOperand(inst.Src0, wf, i, sp[src0Offset:src0Offset+8])
		src0Offset += 8
		p.readOperand(inst.Src1, wf, i, sp[src1Offset:src1Offset+8])
		src1Offset += 8
		if inst.Src2 != nil {
			p.readOperand(inst.Src2, wf, i, sp[src2Offset:src2Offset+8])
			src2Offset += 8
		}
	}
}

func (p *ScratchpadPreparerImpl) prepareVOP3b(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	sp := instEmuState.Scratchpad()

	copy(sp[0:8], wf.ReadReg(insts.Regs[insts.EXEC], 1, 0))
	copy(sp[520:528], wf.ReadReg(insts.Regs[insts.VCC], 1, 0))

	src0Offset := 528
	src1Offset := 1040
	src2Offset := 1552
	for i := 0; i < 64; i++ {
		p.readOperand(inst.Src0, wf, i, sp[src0Offset:src0Offset+8])
		src0Offset += 8
		p.readOperand(inst.Src1, wf, i, sp[src1Offset:src1Offset+8])
		src1Offset += 8
		if inst.Src2 != nil {
			p.readOperand(inst.Src2, wf, i, sp[src2Offset:src2Offset+8])
			src2Offset += 8
		}
	}
}

func (p *ScratchpadPreparerImpl) prepareVOPC(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	sp := instEmuState.Scratchpad()

	src0Offset := 16
	src1Offset := 16 + 64*8
	for i := 0; i < 64; i++ {
		p.readOperand(inst.Src0, wf, i, sp[src0Offset:src0Offset+8])
		src0Offset += 8
		p.readOperand(inst.Src1, wf, i, sp[src1Offset:src1Offset+8])
		src1Offset += 8
	}

	layout := sp.AsVOPC()
	layout.EXEC = wf.EXEC()
}

func (p *ScratchpadPreparerImpl) prepareFlat(
	instEmuState InstEmuState, wf *Wavefront,
) {
	// Flat instructions now read operands directly via ReadOperand.
	// No scratchpad preparation needed.
}

func (p *ScratchpadPreparerImpl) prepareSMEM(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	// SMEM instructions now read directly via ReadOperand/ReadOperandBytes.
	// No scratchpad preparation needed.
}

func (p *ScratchpadPreparerImpl) prepareSOPP(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	// SOPP instructions now read directly via ReadOperand/SCC/EXEC/VCC/PC.
	// No scratchpad preparation needed.
}

func (p *ScratchpadPreparerImpl) prepareSOPK(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	// SOPK instructions now read directly via ReadOperand/SCC.
	// No scratchpad preparation needed.
}

func (p *ScratchpadPreparerImpl) prepareSOPC(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	// SOPC instructions now read directly via ReadOperand.
	// No scratchpad preparation needed.
}

func (p *ScratchpadPreparerImpl) prepareDS(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	// DS instructions now read operands directly via ReadOperand.
	// No scratchpad preparation needed.
}

// Commit write to the register file according to the scratchpad layout
//
//nolint:gocyclo,funlen
func (p *ScratchpadPreparerImpl) Commit(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	switch inst.FormatType {
	case insts.SOP1:
		p.commitSOP1(instEmuState, wf)
	case insts.SOP2:
		p.commitSOP2(instEmuState, wf)
	case insts.VOP1:
		p.commitVOP1(instEmuState, wf)
	case insts.VOP2:
		p.commitVOP2(instEmuState, wf)
	case insts.VOP3a:
		p.commitVOP3a(instEmuState, wf)
	case insts.VOP3b:
		p.commitVOP3b(instEmuState, wf)
	case insts.VOPC:
		p.commitVOPC(instEmuState, wf)
	case insts.FLAT:
		p.commitFlat(instEmuState, wf)
	case insts.SMEM:
		p.commitSMEM(instEmuState, wf)
	case insts.SOPP:
		p.commitSOPP(instEmuState, wf)
	case insts.SOPC:
		p.commitSOPC(instEmuState, wf)
	case insts.SOPK:
		p.commitSOPK(instEmuState, wf)
	case insts.DS:
		p.commitDS(instEmuState, wf)
	default:
		log.Panicf("Inst format %s is not supported", inst.Format.FormatName)
	}
}

func (p *ScratchpadPreparerImpl) commitSOP1(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	// SOP1 instructions now write directly via WriteOperand/SetSCC/SetEXEC/SetPC.
	// No scratchpad commit needed.
}

func (p *ScratchpadPreparerImpl) commitSOP2(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	// SOP2 instructions now write directly via WriteOperand/SetSCC.
	// No scratchpad commit needed.
}

func (p *ScratchpadPreparerImpl) commitVOP1(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	scratchpad := instEmuState.Scratchpad()
	exec := scratchpad.AsVOP1().EXEC

	wf.WriteReg(insts.Regs[insts.VCC], 1, 0, scratchpad[520:528])

	for i := 63; i >= 0; i-- {
		if !laneMasked(exec, uint(i)) {
			continue
		}
		offset := 8 + i*8
		p.writeOperand(inst.Dst, wf, i, scratchpad[offset:offset+8])
	}
}

func (p *ScratchpadPreparerImpl) commitVOP2(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	scratchpad := instEmuState.Scratchpad()
	exec := scratchpad.AsVOP2().EXEC

	wf.WriteReg(insts.Regs[insts.VCC], 1, 0, scratchpad[520:528])

	for i := 0; i < 64; i++ {
		if !laneMasked(exec, uint(i)) {
			continue
		}

		offset := 8 + i*8
		p.writeOperand(inst.Dst, wf, i, scratchpad[offset:offset+8])
	}
}

func (p *ScratchpadPreparerImpl) commitVOP3a(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	sp := instEmuState.Scratchpad()

	if inst.Opcode <= 255 {
		p.commitVOP3aCmp(instEmuState, wf)
		return
	}

	wf.WriteReg(insts.Regs[insts.VCC], 1, 0, sp[520:528])

	exec := sp.AsVOP3A().EXEC

	for i := 63; i >= 0; i-- {
		if !laneMasked(exec, uint(i)) {
			continue
		}

		offset := 8 + i*8
		p.writeOperand(inst.Dst, wf, i, sp[offset:offset+8])
	}
}

func (p *ScratchpadPreparerImpl) commitVOP3aCmp(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	sp := instEmuState.Scratchpad()

	p.writeOperand(inst.Dst, wf, 0, sp[8:16])
}

func (p *ScratchpadPreparerImpl) commitVOP3b(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	sp := instEmuState.Scratchpad()
	layout := sp.AsVOP3B()

	wf.WriteReg(insts.Regs[insts.VCC], 1, 0, sp[520:528])
	exec := layout.EXEC

	for i := 63; i >= 0; i-- {
		if !laneMasked(exec, uint(i)) {
			continue
		}
		offset := 8 + i*8
		p.writeOperand(inst.Dst, wf, i, sp[offset:offset+8])
	}
	p.writeOperand(inst.SDst, wf, 0, insts.Uint64ToBytes(layout.SDST))
}

func (p *ScratchpadPreparerImpl) commitVOPC(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	sp := instEmuState.Scratchpad().AsVOPC()
	wf.SetVCC(sp.VCC)
	wf.SetEXEC(sp.EXEC)
}

func (p *ScratchpadPreparerImpl) commitFlat(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	// Flat instructions now write results directly via WriteOperand.
	// No scratchpad commit needed.
}

func (p *ScratchpadPreparerImpl) commitSMEM(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	// SMEM instructions now write directly via WriteOperandBytes.
	// No scratchpad commit needed.
}

func (p *ScratchpadPreparerImpl) commitSOPC(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	// SOPC instructions now write directly via SetSCC.
	// No scratchpad commit needed.
}

func (p *ScratchpadPreparerImpl) commitSOPP(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	// SOPP instructions now write directly via SetPC.
	// No scratchpad commit needed.
}

func (p *ScratchpadPreparerImpl) commitSOPK(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	// SOPK instructions now write directly via WriteOperand/SetSCC.
	// No scratchpad commit needed.
}

func (p *ScratchpadPreparerImpl) commitDS(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	// DS instructions now write results directly via WriteOperand.
	// No scratchpad commit needed.
}

func (p *ScratchpadPreparerImpl) readOperand(
	operand *insts.Operand,
	wf *Wavefront,
	laneID int,
	buf []byte,
) {
	switch operand.OperandType {
	case insts.RegOperand:
		copy(buf, wf.ReadReg(operand.Register, operand.RegCount, laneID))
	case insts.IntOperand:
		copy(buf, insts.Uint64ToBytes(uint64(operand.IntValue)))
	case insts.FloatOperand:
		copy(buf, insts.Uint64ToBytes(uint64(math.Float32bits(float32(operand.FloatValue)))))
	case insts.LiteralConstant:
		copy(buf, insts.Uint32ToBytes(operand.LiteralConstant))
	default:
		log.Panicf("Operand %s is not supported", operand.String())
	}
}

func (p *ScratchpadPreparerImpl) writeOperand(
	operand *insts.Operand,
	wf *Wavefront,
	laneID int,
	buf []byte,
) {
	if operand.OperandType != insts.RegOperand {
		log.Panic("Can only write into reg operand")
	}

	wf.WriteReg(operand.Register, operand.RegCount, laneID, buf)
}

func (p *ScratchpadPreparerImpl) clear(buf []byte) {
	for i := 0; i < len(buf); i++ {
		buf[i] = 0
	}
}
