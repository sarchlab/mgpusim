package emu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

var _ = Describe("ScratchpadPreparer", func() {
	var (
		sp *ScratchpadPreparerImpl
		wf *Wavefront
	)

	BeforeEach(func() {
		sp = NewScratchpadPreparerImpl(false)
		wf = NewWavefront(nil)
	})

	It("should prepare for SOP1", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOP1
		inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		wf.inst = inst

		wf.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(517))
		wf.SetSCC(1)
		wf.SetEXEC(0xffffffff00000000)
		wf.SetPC(10)

		sp.Prepare(wf, wf)

		// SOP1 instructions now use ReadOperand directly; scratchpad is not used.
		layout := wf.Scratchpad().AsSOP1()
		Expect(layout.SRC0).To(Equal(uint64(0)))
		Expect(layout.EXEC).To(Equal(uint64(0)))
		Expect(layout.SCC).To(Equal(byte(0)))
		Expect(layout.PC).To(Equal(uint64(0)))
	})

	It("should prepare for SOP2", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOP2
		inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		inst.Src1 = insts.NewIntOperand(1, 1)
		wf.inst = inst

		wf.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(517))
		wf.SetSCC(1)

		sp.Prepare(wf, wf)

		// SOP2 instructions now use ReadOperand directly; scratchpad is not used.
		layout := wf.Scratchpad().AsSOP2()
		Expect(layout.SRC0).To(Equal(uint64(0)))
		Expect(layout.SRC1).To(Equal(uint64(0)))
		Expect(layout.SCC).To(Equal(byte(0)))
	})

	It("should prepare for VOP1", func() {
		// VOP1 prepare is now a no-op (instructions read directly via ReadOperand).
		inst := insts.NewInst()
		inst.FormatType = insts.VOP1
		inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		wf.inst = inst

		sp.Prepare(wf, wf)

		// Scratchpad should remain zeroed (no-op)
		layout := wf.Scratchpad().AsVOP1()
		Expect(layout.EXEC).To(Equal(uint64(0)))
	})

	It("should prepare for VOP2", func() {
		// VOP2 prepare is now a no-op (instructions read directly via ReadOperand).
		inst := insts.NewInst()
		inst.FormatType = insts.VOP2
		inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		inst.Src1 = insts.NewVRegOperand(2, 2, 2)
		inst.Dst = insts.NewVRegOperand(6, 6, 2)
		wf.inst = inst

		sp.Prepare(wf, wf)

		// Scratchpad should remain zeroed (no-op)
		layout := wf.Scratchpad().AsVOP2()
		Expect(layout.EXEC).To(Equal(uint64(0)))
	})

	It("should prepare for VOP3a", func() {
		// VOP3A prepare is now a no-op (instructions read directly via ReadOperand).
		inst := insts.NewInst()
		inst.FormatType = insts.VOP3a
		inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		inst.Src1 = insts.NewVRegOperand(2, 2, 2)
		inst.Src2 = insts.NewIntOperand(1, 1)
		wf.inst = inst

		sp.Prepare(wf, wf)

		// Scratchpad should remain zeroed (no-op)
		layout := wf.Scratchpad().AsVOP3A()
		Expect(layout.EXEC).To(Equal(uint64(0)))
	})

	It("should prepare for VOP3b", func() {
		// VOP3B prepare is now a no-op (instructions read directly via ReadOperand).
		inst := insts.NewInst()
		inst.FormatType = insts.VOP3b
		inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		inst.Src1 = insts.NewVRegOperand(2, 2, 2)
		inst.Src2 = insts.NewIntOperand(1, 1)
		wf.inst = inst

		sp.Prepare(wf, wf)

		// Scratchpad should remain zeroed (no-op)
		layout := wf.Scratchpad().AsVOP3B()
		Expect(layout.EXEC).To(Equal(uint64(0)))
	})

	It("should prepare for VOPC", func() {
		// VOPC prepare is now a no-op (instructions read directly via ReadOperand).
		inst := insts.NewInst()
		inst.FormatType = insts.VOPC
		inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		wf.inst = inst

		sp.Prepare(wf, wf)

		// Scratchpad should remain zeroed (no-op)
		layout := wf.Scratchpad().AsVOPC()
		Expect(layout.EXEC).To(Equal(uint64(0)))
	})

	It("should prepare for FLAT", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.FLAT
		inst.Addr = insts.NewVRegOperand(0, 0, 2)
		inst.Data = insts.NewVRegOperand(2, 2, 4)
		wf.inst = inst

		for i := 0; i < 64; i++ {
			wf.WriteReg(insts.VReg(0), 2, i,
				insts.Uint64ToBytes(uint64(i+1024)))
			wf.WriteReg(insts.VReg(2), 1, i, insts.Uint32ToBytes(uint32(i)))
		}
		wf.SetEXEC(0xff)

		sp.Prepare(wf, wf)

		// Flat instructions now use ReadOperand directly; scratchpad is not used.
		layout := wf.Scratchpad().AsFlat()
		Expect(layout.EXEC).To(Equal(uint64(0)))
		for i := 0; i < 64; i++ {
			Expect(layout.ADDR[i]).To(Equal(uint64(0)))
		}
	})

	It("should prepare for SMEM", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SMEM
		inst.Opcode = 18
		inst.Data = insts.NewSRegOperand(0, 0, 4)
		inst.Offset = insts.NewIntOperand(1, 1)
		inst.Base = insts.NewSRegOperand(4, 4, 2)
		wf.inst = inst

		wf.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(100))
		wf.WriteReg(insts.SReg(1), 1, 0, insts.Uint32ToBytes(101))
		wf.WriteReg(insts.SReg(2), 1, 0, insts.Uint32ToBytes(102))
		wf.WriteReg(insts.SReg(3), 1, 0, insts.Uint32ToBytes(103))
		wf.WriteReg(insts.SReg(4), 2, 0, insts.Uint64ToBytes(1024))

		sp.Prepare(wf, wf)

		// SMEM instructions now use ReadOperand directly; scratchpad is not used.
		layout := wf.Scratchpad().AsSMEM()
		Expect(layout.DATA[0]).To(Equal(uint32(0)))
		Expect(layout.Offset).To(Equal(uint64(0)))
		Expect(layout.Base).To(Equal(uint64(0)))
	})

	It("should prepare for SOPP", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOPP
		inst.SImm16 = insts.NewIntOperand(1, 1)
		wf.SetEXEC(0x0f)
		wf.SetPC(160)
		wf.SetSCC(1)
		wf.inst = inst

		sp.Prepare(wf, wf)

		// SOPP instructions now use ReadOperand directly; scratchpad is not used.
		layout := wf.Scratchpad().AsSOPP()
		Expect(layout.EXEC).To(Equal(uint64(0)))
		Expect(layout.IMM).To(Equal(uint64(0)))
		Expect(layout.PC).To(Equal(uint64(0)))
		Expect(layout.SCC).To(Equal(byte(0)))
	})

	It("should prepare for SOPC", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOPC
		inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		wf.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(100))
		inst.Src1 = insts.NewIntOperand(192, 64)
		wf.inst = inst

		sp.Prepare(wf, wf)

		// SOPC instructions now use ReadOperand directly; scratchpad is not used.
		layout := wf.Scratchpad().AsSOPC()
		Expect(layout.SRC0).To(Equal(uint64(0)))
		Expect(layout.SRC1).To(Equal(uint64(0)))
	})

	It("should prepare for SOPK", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOPK
		inst.Dst = insts.NewSRegOperand(0, 0, 1)
		inst.SImm16 = insts.NewIntOperand(1, 1)
		wf.inst = inst
		wf.SetSCC(1)
		wf.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(100))

		sp.Prepare(wf, wf)

		// SOPK instructions now use ReadOperand directly; scratchpad is not used.
		layout := wf.Scratchpad().AsSOPK()
		Expect(layout.DST).To(Equal(uint64(0)))
		Expect(layout.IMM).To(Equal(uint64(0)))
		Expect(layout.SCC).To(Equal(byte(0)))
	})

	It("should prepare for DS", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.DS
		inst.Addr = insts.NewVRegOperand(0, 0, 1)
		inst.Data = insts.NewVRegOperand(2, 2, 2)
		inst.Data1 = insts.NewVRegOperand(4, 4, 2)

		wf.inst = inst
		wf.SetEXEC(uint64(0xff))

		for i := 0; i < 64; i++ {
			wf.WriteReg(insts.VReg(0), 1, i, insts.Uint64ToBytes(uint64(i)))
			wf.WriteReg(insts.VReg(2), 1, i, insts.Uint64ToBytes(uint64(i+1)))
		}

		sp.Prepare(wf, wf)

		// DS instructions now use ReadOperand directly; scratchpad is not used.
		layout := wf.Scratchpad().AsDS()
		Expect(layout.EXEC).To(Equal(uint64(0)))
		for i := 0; i < 64; i++ {
			Expect(layout.ADDR[i]).To(Equal(uint32(0)))
		}
	})

	It("should commit for SOP1", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOP1
		inst.Dst = insts.NewSRegOperand(0, 0, 1)
		wf.inst = inst

		// SOP1 commit is now a no-op; instructions write directly.
		// Set values before commit and verify they are unchanged.
		wf.SetSCC(1)
		wf.SetEXEC(0xffffffff00000000)
		wf.SetPC(20)
		wf.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(517))

		sp.Commit(wf, wf)

		Expect(wf.SCC()).To(Equal(byte(1)))
		Expect(wf.EXEC()).To(Equal(uint64(0xffffffff00000000)))
		Expect(wf.SRegValue(0)).To(Equal(uint32(517)))
		Expect(wf.PC()).To(Equal(uint64(20)))
	})

	It("should commit for SOP2", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOP2
		inst.Dst = insts.NewSRegOperand(0, 0, 1)
		wf.inst = inst

		// SOP2 commit is now a no-op; instructions write directly.
		wf.SetSCC(1)
		wf.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(517))

		sp.Commit(wf, wf)

		Expect(wf.SCC()).To(Equal(byte(1)))
		Expect(wf.SRegValue(0)).To(Equal(uint32(517)))
	})

	It("should commit for VOP1", func() {
		// VOP1 commit is now a no-op (instructions write directly via WriteOperand).
		inst := insts.NewInst()
		inst.FormatType = insts.VOP1
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		wf.inst = inst

		// Set VCC before commit
		wf.SetVCC(0x1234)

		sp.Commit(wf, wf)

		// VCC should remain unchanged (commit is no-op)
		Expect(wf.VCC()).To(Equal(uint64(0x1234)))
	})

	It("should commit for VOP2", func() {
		// VOP2 commit is now a no-op (instructions write directly via WriteOperand).
		inst := insts.NewInst()
		inst.FormatType = insts.VOP2
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		wf.inst = inst

		// Set VCC before commit
		wf.SetVCC(0x5678)

		sp.Commit(wf, wf)

		// VCC should remain unchanged (commit is no-op)
		Expect(wf.VCC()).To(Equal(uint64(0x5678)))
	})

	It("should commit for VOP3a CMP", func() {
		// VOP3A commit is now a no-op (instructions write directly via WriteOperand).
		inst := insts.NewInst()
		inst.FormatType = insts.VOP3a
		inst.Opcode = 20
		inst.Dst = insts.NewSRegOperand(0, 0, 1)
		wf.inst = inst

		wf.SetVCC(0xabcd)

		sp.Commit(wf, wf)

		// VCC should remain unchanged (commit is no-op)
		Expect(wf.VCC()).To(Equal(uint64(0xabcd)))
	})

	It("should commit for VOP3a", func() {
		// VOP3A commit is now a no-op (instructions write directly via WriteOperand).
		inst := insts.NewInst()
		inst.FormatType = insts.VOP3a
		inst.Opcode = 449
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		wf.inst = inst

		wf.SetVCC(0xffff0000ffff0000)

		sp.Commit(wf, wf)

		// VCC should remain unchanged (commit is no-op)
		Expect(wf.VCC()).To(Equal(uint64(0xffff0000ffff0000)))
	})

	It("should commit for VOP3b", func() {
		// VOP3B commit is now a no-op (instructions write directly via WriteOperand).
		inst := insts.NewInst()
		inst.FormatType = insts.VOP3b
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		inst.SDst = insts.NewSRegOperand(0, 0, 1)
		wf.inst = inst

		wf.SetVCC(0xffff0000ffff0000)

		sp.Commit(wf, wf)

		// VCC should remain unchanged (commit is no-op)
		Expect(wf.VCC()).To(Equal(uint64(0xffff0000ffff0000)))
	})

	It("should commit VOPC", func() {
		// VOPC commit is now a no-op (instructions write directly via SetVCC).
		inst := insts.NewInst()
		inst.FormatType = insts.VOPC
		wf.inst = inst

		wf.SetVCC(0xdeadbeef)
		wf.SetEXEC(0xcafebabe)

		sp.Commit(wf, wf)

		// VCC and EXEC should remain unchanged (commit is no-op)
		Expect(wf.VCC()).To(Equal(uint64(0xdeadbeef)))
		Expect(wf.EXEC()).To(Equal(uint64(0xcafebabe)))
	})

	It("should commit for FLAT", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.FLAT
		inst.Opcode = 20 // Load Dword
		inst.Dst = insts.NewVRegOperand(3, 3, 4)
		wf.inst = inst

		// Flat commit is now a no-op; instructions write directly.
		// Set values before commit and verify they are unchanged.
		for i := 0; i < 64; i++ {
			wf.WriteReg(insts.VReg(3), 1, i, insts.Uint32ToBytes(uint32(i+10)))
		}

		sp.Commit(wf, wf)

		for i := 0; i < 64; i++ {
			Expect(wf.VRegValue(i, 3)).To(Equal(uint32(i + 10)))
		}
	})

	It("should not commit for FLAT store operation", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.FLAT
		inst.Dst = insts.NewVRegOperand(3, 3, 4)
		inst.Opcode = 28 // Store dword
		wf.inst = inst

		// Flat commit is now a no-op; registers should not be modified.
		for i := 0; i < 64; i++ {
			Expect(wf.VRegValue(i, 3)).To(Equal(uint32(0)))
			Expect(wf.VRegValue(i, 4)).To(Equal(uint32(0)))
		}

		sp.Commit(wf, wf)

		for i := 0; i < 64; i++ {
			Expect(wf.VRegValue(i, 3)).To(Equal(uint32(0)))
			Expect(wf.VRegValue(i, 4)).To(Equal(uint32(0)))
		}
	})

	It("should commit for SMEM", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SMEM
		inst.Opcode = 4
		inst.Data = insts.NewSRegOperand(0, 0, 16)
		wf.inst = inst

		// SMEM commit is now a no-op; instructions write directly.
		for i := 0; i < 16; i++ {
			wf.WriteReg(insts.SReg(i), 1, 0, insts.Uint32ToBytes(uint32(i)))
		}

		sp.Commit(wf, wf)

		for i := 0; i < 16; i++ {
			Expect(wf.SRegValue(i)).To(Equal(uint32(i)))
		}
	})

	It("should commit for SOPC", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOPC
		wf.inst = inst

		// SOPC commit is now a no-op; instructions write directly.
		wf.SetSCC(1)

		sp.Commit(wf, wf)

		Expect(wf.SCC()).To(Equal(byte(1)))
	})

	It("should commit for SOPP", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOPP
		wf.inst = inst

		// SOPP commit is now a no-op; instructions write directly.
		wf.SetPC(164)

		sp.Commit(wf, wf)

		Expect(wf.PC()).To(Equal(uint64(164)))
	})

	It("should commit for SOPK", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOPK
		inst.Dst = insts.NewSRegOperand(0, 0, 1)
		wf.inst = inst

		// SOPK commit is now a no-op; instructions write directly.
		wf.SetSCC(1)
		wf.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(517))

		sp.Commit(wf, wf)

		Expect(wf.SCC()).To(Equal(byte(1)))
		Expect(wf.SRegValue(0)).To(Equal(uint32(517)))
	})

	It("should commit for DS", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.DS
		inst.Dst = insts.NewVRegOperand(0, 0, 2)
		wf.inst = inst

		// DS commit is now a no-op; instructions write directly.
		// Set values before commit and verify they are unchanged.
		for i := 0; i < 64; i++ {
			wf.WriteReg(insts.VReg(0), 1, i, insts.Uint32ToBytes(uint32(i+5)))
			wf.WriteReg(insts.VReg(1), 1, i, insts.Uint32ToBytes(uint32(i+6)))
		}

		sp.Commit(wf, wf)

		for i := 0; i < 64; i++ {
			Expect(wf.VRegValue(i, 0)).To(Equal(uint32(i + 5)))
			Expect(wf.VRegValue(i, 1)).To(Equal(uint32(i + 6)))
		}
	})

})
