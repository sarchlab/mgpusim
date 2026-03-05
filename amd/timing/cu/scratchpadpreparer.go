package cu

import (
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v4/amd/emu"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/timing/wavefront"
)

// ScratchpadPreparer does its jobs
type ScratchpadPreparer interface {
	Prepare(instEmuState emu.InstEmuState, wf *wavefront.Wavefront)
	Commit(instEmuState emu.InstEmuState, wf *wavefront.Wavefront)
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
//
//nolint:gocyclo
func (p *ScratchpadPreparerImpl) Prepare(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	p.clear(wf.Scratchpad())
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
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// SOP1 instructions now read directly via ReadOperand.
}

func (p *ScratchpadPreparerImpl) prepareSOP2(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// SOP2 instructions now read directly via ReadOperand.
}

func (p *ScratchpadPreparerImpl) prepareVOP1(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// VOP1 instructions now read operands directly via ReadOperand.
}

func (p *ScratchpadPreparerImpl) prepareVOP2(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// VOP2 instructions now read operands directly via ReadOperand.
}

func (p *ScratchpadPreparerImpl) prepareVOP3a(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// VOP3a instructions now read operands directly via ReadOperand.
}

func (p *ScratchpadPreparerImpl) prepareVOP3b(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// VOP3b instructions now read operands directly via ReadOperand.
}

func (p *ScratchpadPreparerImpl) prepareVOPC(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// VOPC instructions now read operands directly via ReadOperand.
}

func (p *ScratchpadPreparerImpl) prepareFlat(
	instEmuState emu.InstEmuState, wf *wavefront.Wavefront,
) {
	// In timing mode, the coalescer reads EXEC, ADDR, DATA from the scratchpad
	// to generate memory transactions. Keep scratchpad preparation.
	inst := instEmuState.Inst()
	sp := wf.Scratchpad()
	layout := sp.AsFlat()

	layout.EXEC = wf.EXEC()

	for i := 0; i < 64; i++ {
		p.readOperand(inst.Addr, wf, i, sp[8+i*8:8+i*8+8])
		p.readOperand(inst.Data, wf, i, sp[520+i*16:520+i*16+16])
	}
}

func (p *ScratchpadPreparerImpl) prepareSMEM(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// In timing mode, SMEM uses the scratchpad for Base/Offset to create
	// memory read requests (not through alu.Run). Keep scratchpad preparation.
	inst := instEmuState.Inst()
	scratchpad := wf.Scratchpad()

	if inst.Opcode >= 16 && inst.Opcode <= 26 { // Store instructions
		p.readOperand(inst.Data, wf, 0, scratchpad[0:16])
	}

	p.readOperand(inst.Offset, wf, 0, scratchpad[16:24])
	p.readOperand(inst.Base, wf, 0, scratchpad[24:32])
}

func (p *ScratchpadPreparerImpl) prepareSOPP(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// SOPP instructions now read directly via ReadOperand.
}

func (p *ScratchpadPreparerImpl) prepareSOPK(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// SOPK instructions now read directly via ReadOperand.
}

func (p *ScratchpadPreparerImpl) prepareSOPC(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// SOPC instructions now read directly via ReadOperand.
}

func (p *ScratchpadPreparerImpl) prepareDS(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// DS instructions now read operands directly via ReadOperand.
	// No scratchpad preparation needed.
}

// Commit write to the register file according to the scratchpad layout
//
//nolint:gocyclo
func (p *ScratchpadPreparerImpl) Commit(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
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
	case insts.SOPK:
		p.commitSOPK(instEmuState, wf)
	case insts.SOPC:
		p.commitSOPC(instEmuState, wf)
	case insts.DS:
		p.commitDS(instEmuState, wf)
	default:
		log.Panicf("Inst format %s is not supported", inst.Format.FormatName)
	}
}

func (p *ScratchpadPreparerImpl) commitSOP1(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// SOP1 instructions now write directly via WriteOperand/SetSCC/SetEXEC.
}

func (p *ScratchpadPreparerImpl) commitSOP2(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// SOP2 instructions now write directly via WriteOperand/SetSCC.
}

func (p *ScratchpadPreparerImpl) commitVOP1(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// VOP1 instructions now write results directly via WriteOperand.
}

func (p *ScratchpadPreparerImpl) commitVOP2(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// VOP2 instructions now write results directly via WriteOperand.
}

func (p *ScratchpadPreparerImpl) commitVOP3a(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// VOP3a instructions now write results directly via WriteOperand.
}

func (p *ScratchpadPreparerImpl) commitVOP3b(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// VOP3b instructions now write results directly via WriteOperand.
}

func (p *ScratchpadPreparerImpl) commitVOPC(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// VOPC instructions now write results directly via WriteOperand.
}

func (p *ScratchpadPreparerImpl) commitFlat(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// In timing mode, memory responses write data into the scratchpad DST area.
	// We must commit that data back to registers.
	inst := instEmuState.Inst()
	scratchpad := wf.Scratchpad()
	exec := scratchpad.AsFlat().EXEC

	if inst.Opcode < 24 || inst.Opcode > 31 { // Skip store instructions
		for i := 0; i < 64; i++ {
			if !laneMasked(exec, uint(i)) {
				continue
			}
			p.writeOperand(inst.Dst, wf, i, scratchpad[1544+i*16:1544+i*16+16])
		}
	}
}

func (p *ScratchpadPreparerImpl) commitSMEM(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// SMEM in timing mode is handled by memory response handlers,
	// not through scratchpad commit.
}

func (p *ScratchpadPreparerImpl) commitSOPK(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// SOPK instructions now write directly via WriteOperand/SetSCC.
}

func (p *ScratchpadPreparerImpl) commitSOPC(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// SOPC instructions now write directly via SetSCC.
}

func (p *ScratchpadPreparerImpl) commitSOPP(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// SOPP instructions now write directly via SetPC.
}

func (p *ScratchpadPreparerImpl) commitDS(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	// DS instructions now write results directly via WriteOperand/WriteOperandBytes.
	// No scratchpad commit needed.
}

func (p *ScratchpadPreparerImpl) readOperand(
	operand *insts.Operand,
	wf *wavefront.Wavefront,
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
	wf *wavefront.Wavefront,
	laneID int,
) uint32 {
	buf := make([]byte, 4)
	p.readReg(reg, 1, wf, laneID, buf)
	return insts.BytesToUint32(buf)
}

//nolint:gocyclo
func (p *ScratchpadPreparerImpl) readReg(
	reg *insts.Reg,
	regCount int,
	wf *wavefront.Wavefront,
	laneID int,
	buf []byte,
) {
	if reg.IsSReg() {
		regFile := p.cu.SRegFile
		regRead := RegisterAccess{}
		regRead.Reg = reg
		regRead.RegCount = regCount
		regRead.LaneID = laneID
		regRead.WaveOffset = wf.SRegOffset
		regRead.Data = buf
		regFile.Read(regRead)
	} else if reg.IsVReg() {
		regFile := p.cu.VRegFile[wf.SIMDID]
		regRead := RegisterAccess{}
		regRead.Reg = reg
		regRead.RegCount = regCount
		regRead.LaneID = laneID
		regRead.WaveOffset = wf.VRegOffset
		regRead.Data = buf
		regFile.Read(regRead)
	} else if reg.RegType == insts.SCC {
		buf[0] = wf.SCC()
	} else if reg.RegType == insts.VCC {
		copy(buf, insts.Uint64ToBytes(wf.VCC()))
	} else if reg.RegType == insts.VCCLO && regCount == 1 {
		copy(buf, insts.Uint32ToBytes(uint32(wf.VCC())))
	} else if reg.RegType == insts.VCCHI && regCount == 1 {
		copy(buf, insts.Uint32ToBytes(uint32(wf.VCC()>>32)))
	} else if reg.RegType == insts.VCCLO && regCount == 2 {
		copy(buf, insts.Uint64ToBytes(wf.VCC()))
	} else if reg.RegType == insts.EXEC {
		copy(buf, insts.Uint64ToBytes(wf.EXEC()))
	} else if reg.RegType == insts.EXECLO && regCount == 2 {
		copy(buf, insts.Uint64ToBytes(wf.EXEC()))
	} else if reg.RegType == insts.M0 {
		copy(buf, insts.Uint32ToBytes(wf.M0))
	} else {
		log.Panicf("Unsupported register read %s\n", reg.Name)
	}
}

func (p *ScratchpadPreparerImpl) writeOperand(
	operand *insts.Operand,
	wf *wavefront.Wavefront,
	laneID int,
	buf []byte,
) {
	if operand.OperandType != insts.RegOperand {
		log.Panic("Can only write into reg operand")
	}

	p.writeReg(operand.Register, operand.RegCount, wf, laneID, buf)
}

//nolint:gocyclo
func (p *ScratchpadPreparerImpl) writeReg(
	reg *insts.Reg,
	regCount int,
	wf *wavefront.Wavefront,
	laneID int,
	buf []byte,
) {
	if reg.IsSReg() {
		regFile := p.cu.SRegFile
		regWrite := RegisterAccess{}
		regWrite.Reg = reg
		regWrite.RegCount = regCount
		regWrite.LaneID = laneID
		regWrite.WaveOffset = wf.SRegOffset
		regWrite.Data = buf
		regFile.Write(regWrite)
	} else if reg.IsVReg() {
		regFile := p.cu.VRegFile[wf.SIMDID]
		regWrite := RegisterAccess{}
		regWrite.Reg = reg
		regWrite.RegCount = regCount
		regWrite.LaneID = laneID
		regWrite.WaveOffset = wf.VRegOffset
		regWrite.Data = buf
		regFile.Write(regWrite)
	} else if reg.RegType == insts.SCC {
		wf.SetSCC(buf[0])
	} else if reg.RegType == insts.VCC {
		wf.SetVCC(insts.BytesToUint64(buf))
	} else if reg.RegType == insts.VCCLO && regCount == 2 {
		wf.SetVCC(insts.BytesToUint64(buf))
	} else if reg.RegType == insts.VCCLO && regCount == 1 {
		wf.SetVCC(wf.VCC() & uint64(0x00000000ffffffff))
		wf.SetVCC(wf.VCC() | uint64(insts.BytesToUint32(buf)))
	} else if reg.RegType == insts.VCCHI && regCount == 1 {
		wf.SetVCC(wf.VCC() & uint64(0xffffffff00000000))
		wf.SetVCC(wf.VCC() | uint64(insts.BytesToUint32(buf)) << 32)
	} else if reg.RegType == insts.EXEC {
		wf.SetEXEC(insts.BytesToUint64(buf))
	} else if reg.RegType == insts.EXECLO && regCount == 2 {
		wf.SetEXEC(insts.BytesToUint64(buf))
	} else if reg.RegType == insts.M0 {
		wf.M0 = insts.BytesToUint32(buf)
	} else {
		log.Panicf("Unsupported register write %s\n", reg.Name)
	}
}

func (p *ScratchpadPreparerImpl) clear(buf []byte) {
	for i := 0; i < len(buf); i++ {
		buf[i] = 0
	}
}

func laneMasked(Exec uint64, laneID uint) bool {
	return Exec&(1<<laneID) > 0
}
