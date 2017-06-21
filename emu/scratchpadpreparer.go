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
	Prepare(instEmuState InstEmuState, wf interface{})

	// Commit write to the register file to reflect the change in the scratchpad
	Commit(instEmuState InstEmuState, wf interface{})
}

// ScratchpadPreparerImpl provides a standard implmentation of the
// ScratchpadPreparer
type ScratchpadPreparerImpl struct {
	regInterface RegInterface
}

// NewScratchpadPreparerImpl returns a newly created ScratchpadPreparerImpl,
// injecting the dependency of the RegInterface.
func NewScratchpadPreparerImpl(
	regInterface RegInterface,
) *ScratchpadPreparerImpl {
	p := new(ScratchpadPreparerImpl)
	p.regInterface = regInterface
	return p
}

// Prepare read from the register file and sets the scratchpad layout
func (p *ScratchpadPreparerImpl) Prepare(
	instEmuState InstEmuState,
	wf interface{},
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
	wf interface{},
) {
	inst := instEmuState.Inst()
	scratchPad := instEmuState.Scratchpad()
	p.readOperand(inst.Src0, wf, 0, scratchPad[0:8])
	p.readOperand(inst.Src1, wf, 0, scratchPad[8:16])
	p.readScc(wf, scratchPad[24:25])
}

func (p *ScratchpadPreparerImpl) prepareVOP1(
	instEmuState InstEmuState,
	wf interface{},
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
	wf interface{},
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
	wf interface{},
) {
	inst := instEmuState.Inst()
	scratchpad := instEmuState.Scratchpad()
	p.writeOperand(inst.Dst, wf, 0, scratchpad[16:24])
	p.writeScc(wf, scratchpad[24:25])
}

func (p *ScratchpadPreparerImpl) commitVOP1(
	instEmuState InstEmuState,
	wf interface{},
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
	wf interface{},
	laneID int,
	buf []byte,
) {
	switch operand.OperandType {
	case insts.RegOperand:
		if operand.RegCount == 0 || operand.RegCount == 1 {
			p.regInterface.ReadReg(wf, laneID, operand.Register, buf[0:4])
		} else if operand.RegCount == 2 {
			p.regInterface.ReadReg(wf, laneID, operand.Register, buf[0:8])
		} else {
			log.Panicf("Register count of %d is not supported", operand.RegCount)
		}
	case insts.IntOperand:
		copy(buf, insts.Uint64ToBytes(uint64(operand.IntValue)))
	default:
		log.Panicf("Operand %s is not supported", operand.String())
	}
}

func (p *ScratchpadPreparerImpl) readScc(
	wf interface{},
	buf []byte,
) {
	p.regInterface.ReadReg(wf, 0, insts.Regs[insts.Scc], buf)
}

func (p *ScratchpadPreparerImpl) writeOperand(
	operand *insts.Operand,
	wf interface{},
	laneID int,
	buf []byte,
) {
	if operand.OperandType != insts.RegOperand {
		log.Panic("Can only write into reg operand")
	}

	if operand.RegCount == 0 || operand.RegCount == 1 {
		p.regInterface.WriteReg(wf, laneID, operand.Register, buf[0:4])
	} else if operand.RegCount == 2 {
		p.regInterface.WriteReg(wf, laneID, operand.Register, buf[0:8])
	} else {
		log.Panicf("Register count of %d is not supported", operand.RegCount)
	}
}

func (p *ScratchpadPreparerImpl) writeScc(wf interface{}, buf []byte) {
	p.regInterface.WriteReg(wf, 0, insts.Regs[insts.Scc], buf)
}

func (p *ScratchpadPreparerImpl) clear(buf []byte) {
	for i := 0; i < len(buf); i++ {
		buf[i] = 0
	}
}
