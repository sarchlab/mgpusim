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
type ScratchpadPreparerImpl struct {
	IsCDNA3 bool
}

// NewScratchpadPreparerImpl returns a newly created ScratchpadPreparerImpl,
// injecting the dependency of the RegInterface.
func NewScratchpadPreparerImpl(isCDNA3 bool) *ScratchpadPreparerImpl {
	p := new(ScratchpadPreparerImpl)
	p.IsCDNA3 = isCDNA3
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
	inst := instEmuState.Inst()
	sp := instEmuState.Scratchpad()

	copy(sp[0:8], wf.ReadReg(insts.Regs[insts.EXEC], 1, 0))

	// Check if this is a global instruction with scalar base address (SAddr)
	// SAddr handling is architecture-dependent:
	// - CDNA3: SAddr=0x7F means OFF mode, any other value (including 0) is valid.
	// - GCN3: SAddr=0x7F or SAddr=0 means OFF mode.
	var useSAddr bool
	if p.IsCDNA3 {
		useSAddr = inst.SAddr != nil && inst.SAddr.IntValue != 0x7F
	} else {
		useSAddr = inst.SAddr != nil && inst.SAddr.IntValue != 0x7F && inst.SAddr.IntValue != 0
	}

	var scalarBase uint64
	if useSAddr {
		// Read the scalar base address from SGPR pair
		sAddrReg := int(inst.SAddr.IntValue)
		sAddrOperand := insts.NewSRegOperand(sAddrReg, sAddrReg, 2)
		buf := wf.ReadReg(sAddrOperand.Register, sAddrOperand.RegCount, 0)
		scalarBase = insts.BytesToUint64(buf)
	}

	// Compute signed FLAT offset from the instruction encoding (bits [12:0]).
	// In GFX9+/CDNA3, FLAT/GLOBAL instructions support a 13-bit signed
	// immediate offset, e.g. "global_load_dword v11, v[64:65], off offset:4".
	var flatOffset int64
	if inst.Offset0 != 0 {
		flatOffset = int64(int32(inst.Offset0)) // already sign-extended in decode
	}

	for i := 0; i < 64; i++ {
		if useSAddr {
			// For global with SAddr: addr = SAddr + zero_extend(VGPR) + offset
			// The VGPR is a single 32-bit register (not a pair)
			vAddrOperand := insts.NewVRegOperand(
				inst.Addr.Register.RegIndex(),
				inst.Addr.Register.RegIndex(), 1)
			vBuf := wf.ReadReg(vAddrOperand.Register, 1, i)
			vOffset := uint64(insts.BytesToUint32(vBuf))
			addr := scalarBase + vOffset + uint64(flatOffset)
			copy(sp[8+i*8:8+i*8+8], insts.Uint64ToBytes(addr))
		} else {
			// For flat/global with off: addr = VGPR pair (64-bit) + offset
			p.readOperand(inst.Addr, wf, i, sp[8+i*8:8+i*8+8])
			if flatOffset != 0 {
				addr := insts.BytesToUint64(sp[8+i*8:8+i*8+8]) + uint64(flatOffset)
				copy(sp[8+i*8:8+i*8+8], insts.Uint64ToBytes(addr))
			}
		}
		p.readOperand(inst.Data, wf, i, sp[520+i*16:520+i*16+16])
	}
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
	inst := instEmuState.Inst()
	sp := instEmuState.Scratchpad()
	layout := sp.AsDS()

	layout.EXEC = wf.EXEC()

	offset := 8
	for i := 0; i < 64; i++ {
		p.readOperand(inst.Addr, wf, i, sp[offset+i*4:offset+i*4+4])
	}

	if inst.Data != nil {
		offset = 8 + 64*4
		for i := 0; i < 64; i++ {
			p.readOperand(inst.Data, wf, i, sp[offset+i*16:offset+i*16+16])
		}
	}

	if inst.Data1 != nil {
		offset = 8 + 64*4 + 256*4
		for i := 0; i < 64; i++ {
			p.readOperand(inst.Data1, wf, i, sp[offset+i*16:offset+i*16+16])
		}
	}
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
	inst := instEmuState.Inst()
	scratchpad := instEmuState.Scratchpad()
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
	inst := instEmuState.Inst()
	sp := instEmuState.Scratchpad()
	exec := sp.AsDS().EXEC

	if inst.Dst != nil {
		offset := 8 + 64*4 + 256*4*2
		for i := 0; i < 64; i++ {
			if !laneMasked(exec, uint(i)) {
				continue
			}
			p.writeOperand(inst.Dst, wf, i, sp[offset+i*16:offset+i*16+16])
		}
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
