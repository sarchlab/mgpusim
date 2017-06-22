package emu

import (
	"log"

	"gitlab.com/yaotsu/gcn3/insts"
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
type ScratchpadPreparerImpl struct {
}

// NewScratchpadPreparerImpl returns a newly created ScratchpadPreparerImpl,
// injecting the dependency of the RegInterface.
func NewScratchpadPreparerImpl() *ScratchpadPreparerImpl {
	p := new(ScratchpadPreparerImpl)
	return p
}

// Prepare read from the register file and sets the scratchpad layout
func (p *ScratchpadPreparerImpl) Prepare(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	p.clear(instEmuState.Scratchpad())
	inst := instEmuState.Inst()
	switch inst.FormatType {
	case insts.Sop2:
		p.prepareSOP2(instEmuState, wf)
	case insts.Vop1:
		p.prepareVOP1(instEmuState, wf)
	default:
		log.Panicf("Inst format %s is not supported", inst.Format.FormatName)
	}
}

func (p *ScratchpadPreparerImpl) prepareSOP2(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	scratchPad := instEmuState.Scratchpad()
	p.readOperand(inst.Src0, wf, 0, scratchPad[0:8])
	p.readOperand(inst.Src1, wf, 0, scratchPad[8:16])
	copy(scratchPad[24:25], wf.ReadReg(insts.Regs[insts.Scc], 1, 0))
}

func (p *ScratchpadPreparerImpl) prepareVOP1(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	scratchPad := instEmuState.Scratchpad()

	for i := 0; i < 64; i++ {
		p.readOperand(inst.Src0, wf, i, scratchPad[i*8:i*8+8])
	}

}

// Commit write to the register file according to the scratchpad layout
func (p *ScratchpadPreparerImpl) Commit(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	switch inst.FormatType {
	case insts.Sop2:
		p.commitSOP2(instEmuState, wf)
	case insts.Vop1:
		p.commitVOP1(instEmuState, wf)
	default:
		log.Panicf("Inst format %s is not supported", inst.Format.FormatName)
	}
}
func (p *ScratchpadPreparerImpl) commitSOP2(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	scratchpad := instEmuState.Scratchpad()
	p.writeOperand(inst.Dst, wf, 0, scratchpad[16:24])
	wf.WriteReg(insts.Regs[insts.Scc], 1, 0, scratchpad[24:25])
}

func (p *ScratchpadPreparerImpl) commitVOP1(
	instEmuState InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	scratchpad := instEmuState.Scratchpad()

	offset := 512
	for i := 0; i < 64; i++ {
		p.writeOperand(inst.Dst, wf, i, scratchpad[offset:offset+8])
		offset += 8
	}
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
