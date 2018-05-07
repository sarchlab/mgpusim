package timing

import (
	"log"
	"math"

	"gitlab.com/yaotsu/gcn3/emu"
	"gitlab.com/yaotsu/gcn3/insts"
)

type ScratchpadPreparer interface {
	Prepare(instEmuState emu.InstEmuState, wf *Wavefront)
	Commit(instEmuState emu.InstEmuState, wf *Wavefront)
}

// ScratchpadPreparerImpl reads and write registers for the emulator
type ScratchpadPreparerImpl struct {
	cu *ComputeUnit
}

// NewScratchpadPreparerImpl returns a newly created ScratchpadPreparerImpl,
// injecting the dependency of the RegInterface.
func NewScratchpadPreparerImpl(cu *ComputeUnit) *ScratchpadPreparerImpl {
	p := new(ScratchpadPreparerImpl)
	p.cu = cu
	return p
}

// Prepare read from the register file and sets the scratchpad layout
func (p *ScratchpadPreparerImpl) Prepare(
	instEmuState emu.InstEmuState,
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
		p.prepareVOP3(instEmuState, wf)
	case insts.VOPC:
		p.prepareVOPC(instEmuState, wf)
	case insts.FLAT:
		p.prepareFlat(instEmuState, wf)
	case insts.SMEM:
		p.prepareSMEM(instEmuState, wf)
	case insts.SOPP:
		p.prepareSOPP(instEmuState, wf)
	default:
		log.Panicf("Inst format %s is not supported", inst.Format.FormatName)
	}
}

func (p *ScratchpadPreparerImpl) prepareSOP1(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	scratchPad := instEmuState.Scratchpad()
	layout := scratchPad.AsSOP1()

	p.readOperand(inst.Src0, wf, 0, scratchPad[0:8])
	layout.SCC = wf.SCC
	layout.EXEC = wf.EXEC
}

func (p *ScratchpadPreparerImpl) prepareSOP2(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	scratchPad := instEmuState.Scratchpad()
	layout := scratchPad.AsSOP2()

	p.readOperand(inst.Src0, wf, 0, scratchPad[0:8])
	p.readOperand(inst.Src1, wf, 0, scratchPad[8:16])

	layout.SCC = wf.SCC
}

func (p *ScratchpadPreparerImpl) prepareVOP1(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	sp := instEmuState.Scratchpad()
	layout := sp.AsVOP1()

	layout.EXEC = wf.EXEC
	layout.VCC = wf.VCC

	offset := 528
	for i := 0; i < 64; i++ {
		p.readOperand(inst.Src0, wf, i, sp[offset:offset+8])
		offset += 8
	}
}

func (p *ScratchpadPreparerImpl) prepareVOP2(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	sp := instEmuState.Scratchpad()
	layout := sp.AsVOP2()

	layout.EXEC = wf.EXEC
	layout.VCC = wf.VCC

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

func (p *ScratchpadPreparerImpl) prepareVOP3(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	sp := instEmuState.Scratchpad()
	layout := sp.AsVOP3A()

	layout.EXEC = wf.EXEC
	layout.VCC = wf.VCC

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
	instEmuState emu.InstEmuState,
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
	layout.EXEC = wf.EXEC
}

func (p *ScratchpadPreparerImpl) prepareFlat(
	instEmuState emu.InstEmuState, wf *Wavefront,
) {
	inst := instEmuState.Inst()
	sp := instEmuState.Scratchpad()
	layout := sp.AsFlat()

	layout.EXEC = wf.EXEC

	for i := 0; i < 64; i++ {
		p.readOperand(inst.Addr, wf, i, sp[8+i*8:8+i*8+8])
		p.readOperand(inst.Data, wf, i, sp[520+i*16:520+i*16+16])
	}
}

func (p *ScratchpadPreparerImpl) prepareSMEM(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	scratchpad := instEmuState.Scratchpad()

	if inst.Opcode >= 16 && inst.Opcode <= 26 { // Store instructions
		p.readOperand(inst.Data, wf, 0, scratchpad[0:16])
	}

	p.readOperand(inst.Offset, wf, 0, scratchpad[16:24])
	p.readOperand(inst.Base, wf, 0, scratchpad[24:32])
}

func (p *ScratchpadPreparerImpl) prepareSOPP(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	scratchPad := instEmuState.Scratchpad()
	layout := scratchPad.AsSOPP()

	layout.PC = wf.PC
	layout.SCC = wf.SCC
	layout.EXEC = wf.EXEC
	layout.VCC = wf.VCC
	p.readOperand(inst.SImm16, wf, 0, scratchPad[16:24])
}

func (p *ScratchpadPreparerImpl) prepareSOPC(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	scratchPad := instEmuState.Scratchpad()

	p.readOperand(inst.Src0, wf, 0, scratchPad[0:8])
	p.readOperand(inst.Src1, wf, 0, scratchPad[8:16])
}

// Commit write to the register file according to the scratchpad layout
func (p *ScratchpadPreparerImpl) Commit(
	instEmuState emu.InstEmuState,
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
		p.commitVOP3A(instEmuState, wf)
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
	default:
		log.Panicf("Inst format %s is not supported", inst.Format.FormatName)
	}
}

func (p *ScratchpadPreparerImpl) commitSOP1(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	scratchpad := instEmuState.Scratchpad()
	layout := scratchpad.AsSOP1()

	p.writeOperand(inst.Dst, wf, 0, scratchpad[8:16])
	wf.EXEC = layout.EXEC
	wf.SCC = layout.SCC
}

func (p *ScratchpadPreparerImpl) commitSOP2(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	scratchpad := instEmuState.Scratchpad()
	layout := scratchpad.AsSOP2()
	p.writeOperand(inst.Dst, wf, 0, scratchpad[16:24])
	wf.SCC = layout.SCC
}

func (p *ScratchpadPreparerImpl) commitVOP1(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	scratchpad := instEmuState.Scratchpad()
	layout := scratchpad.AsVOP1()

	wf.VCC = layout.VCC

	for i := 63; i >= 0; i-- {
		offset := 8 + i*8
		p.writeOperand(inst.Dst, wf, i, scratchpad[offset:offset+8])
	}
}

func (p *ScratchpadPreparerImpl) commitVOP2(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	scratchpad := instEmuState.Scratchpad()
	layout := scratchpad.AsVOP2()

	wf.VCC = layout.VCC

	for i := 63; i >= 0; i-- {
		offset := 8 + i*8
		p.writeOperand(inst.Dst, wf, i, scratchpad[offset:offset+8])
	}
}

func (p *ScratchpadPreparerImpl) commitVOP3A(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	sp := instEmuState.Scratchpad()
	layout := sp.AsVOP3A()

	wf.VCC = layout.VCC

	for i := 63; i >= 0; i-- {
		offset := 8 + i*8
		p.writeOperand(inst.Dst, wf, i, sp[offset:offset+8])
	}
}

func (p *ScratchpadPreparerImpl) commitVOPC(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	sp := instEmuState.Scratchpad().AsVOPC()
	wf.VCC = sp.VCC
	wf.EXEC = sp.EXEC
}

func (p *ScratchpadPreparerImpl) commitFlat(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	scratchpad := instEmuState.Scratchpad()

	if inst.Opcode < 24 || inst.Opcode > 31 { // Skip store instructions
		for i := 0; i < 64; i++ {
			p.writeOperand(inst.Dst, wf, i, scratchpad[1544+i*16:1544+i*16+16])
		}
	}
}

func (p *ScratchpadPreparerImpl) commitSMEM(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	inst := instEmuState.Inst()
	scratchpad := instEmuState.Scratchpad()

	if inst.Opcode <= 12 { // Load instructions
		p.writeOperand(inst.Data, wf, 0, scratchpad[32:96])
	}
}

func (p *ScratchpadPreparerImpl) commitSOPC(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	scratchpad := instEmuState.Scratchpad()
	wf.SCC = scratchpad.AsSOPC().SCC
}

func (p *ScratchpadPreparerImpl) commitSOPP(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	scratchpad := instEmuState.Scratchpad()
	wf.PC = scratchpad.AsSOPP().PC
}

func (p *ScratchpadPreparerImpl) readOperand(
	operand *insts.Operand,
	wf *Wavefront,
	laneID int,
	buf []byte,
) {
	switch operand.OperandType {
	case insts.RegOperand:
		p.readReg(operand.Register, operand.RegCount, wf, laneID, buf)
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

func (p *ScratchpadPreparerImpl) readRegAsUint32(
	reg *insts.Reg,
	wf *Wavefront,
	laneID int,
) uint32 {
	buf := make([]byte, 4)
	p.readReg(reg, 1, wf, laneID, buf)
	return insts.BytesToUint32(buf)
}

func (p *ScratchpadPreparerImpl) readReg(
	reg *insts.Reg,
	regCount int,
	wf *Wavefront,
	laneID int,
	buf []byte,
) {
	if reg.IsSReg() {
		regFile := p.cu.SRegFile
		regRead := new(RegisterAccess)
		regRead.Reg = reg
		regRead.RegCount = regCount
		regRead.LaneID = laneID
		regRead.WaveOffset = wf.SRegOffset

		regFile.Read(regRead)

		copy(buf, regRead.Data)
	} else if reg.IsVReg() {
		regFile := p.cu.VRegFile[wf.SIMDID]
		regRead := new(RegisterAccess)
		regRead.Reg = reg
		regRead.RegCount = regCount
		regRead.LaneID = laneID
		regRead.WaveOffset = wf.VRegOffset

		regFile.Read(regRead)

		copy(buf, regRead.Data)
	} else if reg.RegType == insts.SCC {
		buf[0] = wf.SCC
	} else if reg.RegType == insts.VCC {
		copy(buf, insts.Uint64ToBytes(wf.VCC))
	} else if reg.RegType == insts.VCCLO && regCount == 1 {
		copy(buf, insts.Uint32ToBytes(uint32(wf.VCC)))
	} else if reg.RegType == insts.VCCHI && regCount == 1 {
		copy(buf, insts.Uint32ToBytes(uint32(wf.VCC>>32)))
	} else if reg.RegType == insts.VCCLO && regCount == 2 {
		copy(buf, insts.Uint64ToBytes(wf.VCC))
	} else if reg.RegType == insts.EXEC {
		copy(buf, insts.Uint64ToBytes(wf.EXEC))
	} else if reg.RegType == insts.EXECLO && regCount == 2 {
		copy(buf, insts.Uint64ToBytes(wf.EXEC))
	} else {
		log.Panicf("Unsupported register read %s\n", reg.Name)
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

	p.writeReg(operand.Register, operand.RegCount, wf, laneID, buf)
}

func (p *ScratchpadPreparerImpl) writeReg(
	reg *insts.Reg,
	regCount int,
	wf *Wavefront,
	laneID int,
	buf []byte,
) {
	if reg.IsSReg() {
		regFile := p.cu.SRegFile
		regWrite := new(RegisterAccess)
		regWrite.Reg = reg
		regWrite.RegCount = regCount
		regWrite.LaneID = laneID
		regWrite.WaveOffset = wf.SRegOffset
		regWrite.Data = buf

		regFile.Write(regWrite)

	} else if reg.IsVReg() {
		regFile := p.cu.VRegFile[wf.SIMDID]
		regWrite := new(RegisterAccess)
		regWrite.Reg = reg
		regWrite.RegCount = regCount
		regWrite.LaneID = laneID
		regWrite.WaveOffset = wf.VRegOffset
		regWrite.Data = buf

		regFile.Write(regWrite)
	} else if reg.RegType == insts.SCC {
		wf.SCC = buf[0]
	} else if reg.RegType == insts.VCC {
		wf.VCC = insts.BytesToUint64(buf)
	} else if reg.RegType == insts.VCCLO && regCount == 2 {
		wf.VCC = insts.BytesToUint64(buf)
	} else if reg.RegType == insts.VCCLO && regCount == 1 {
		wf.VCC &= uint64(0x00000000ffffffff)
		wf.VCC |= uint64(insts.BytesToUint32(buf))
	} else if reg.RegType == insts.VCCHI && regCount == 1 {
		wf.VCC &= uint64(0xffffffff00000000)
		wf.VCC |= uint64(insts.BytesToUint32(buf)) << 32
	} else if reg.RegType == insts.EXEC {
		wf.EXEC = insts.BytesToUint64(buf)
	} else if reg.RegType == insts.EXECLO && regCount == 2 {
		wf.EXEC = insts.BytesToUint64(buf)
	} else {
		log.Panicf("Unsupported register write %s\n", reg.Name)
	}
}

func (p *ScratchpadPreparerImpl) clear(buf []byte) {
	for i := 0; i < len(buf); i++ {
		buf[i] = 0
	}
}
