package cu

import (
	"bytes"
	"encoding/binary"
	"log"

	"gitlab.com/akita/mgpusim/timing/wavefront"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mgpusim/insts"
)

var _ = Describe("ScratchpadPreparer", func() {
	var (
		sRegFile  *SimpleRegisterFile
		vRegFile0 *SimpleRegisterFile
		cu        *ComputeUnit
		sp        *ScratchpadPreparerImpl
		wf        *wavefront.Wavefront
	)

	BeforeEach(func() {
		sRegFile = NewSimpleRegisterFile(3200*4, 0)
		vRegFile0 = NewSimpleRegisterFile(256*64*4, 1024)

		cu = NewComputeUnit("cu", nil)
		cu.SRegFile = sRegFile
		cu.VRegFile = append(cu.VRegFile, vRegFile0)

		sp = NewScratchpadPreparerImpl(cu)
		wf = wavefront.NewWavefront(nil)
		wf.EXEC = 0xffffffffffffffff
	})

	It("should prepare for SOP1", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOP1
		inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		sp.writeReg(insts.SReg(0), 1, wf, 0,
			insts.Uint32ToBytes(517))
		wf.SCC = 1
		wf.EXEC = 0xffffffff00000000

		sp.Prepare(wf, wf)

		sp := wf.Scratchpad().AsSOP1()
		Expect(sp.SRC0).To(Equal(uint64(517)))
		Expect(sp.EXEC).To(Equal(uint64(0xffffffff00000000)))
		Expect(sp.SCC).To(Equal(byte(1)))
	})

	It("should prepare for SOP2", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOP2
		inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		inst.Src1 = insts.NewIntOperand(1, 1)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		sp.writeReg(insts.SReg(0), 1, wf, 0, insts.Uint32ToBytes(517))
		//wf.WriteReg(insts.SReg(0), 1, 0, insts.Uint32ToBytes(517))
		wf.SCC = 1

		sp.Prepare(wf, wf)

		layout := wf.Scratchpad().AsSOP2()
		binary.Read(bytes.NewBuffer(wf.Scratchpad()), binary.LittleEndian, layout)
		Expect(layout.SRC0).To(Equal(uint64(517)))
		Expect(layout.SRC1).To(Equal(uint64(1)))
		Expect(layout.SCC).To(Equal(byte(1)))

	})

	It("should prepare for VOP1", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.VOP1
		inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		for i := 0; i < 64; i++ {
			sp.writeReg(insts.VReg(0), 1, wf, i, insts.Uint32ToBytes(uint32(i)))
		}
		wf.VCC = uint64(0xffff0000ffff0000)
		wf.EXEC = 0xff

		sp.Prepare(wf, wf)

		layout := wf.Scratchpad().AsVOP1()
		for i := 0; i < 64; i++ {
			Expect(layout.SRC0[i]).To(Equal(uint64(i)))
		}
		Expect(layout.VCC).To(Equal(uint64(0xffff0000ffff0000)))
		Expect(layout.EXEC).To(Equal(uint64(0xff)))
	})

	It("should prepare for VOP2", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.VOP2
		inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		inst.Src1 = insts.NewVRegOperand(2, 2, 2)
		inst.Dst = insts.NewVRegOperand(6, 6, 2)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		for i := 0; i < 64; i++ {
			sp.writeReg(insts.VReg(0), 2, wf, i, insts.Uint64ToBytes(uint64(i)))
			sp.writeReg(insts.VReg(2), 2, wf, i, insts.Uint64ToBytes(uint64(i+1)))
			sp.writeReg(insts.VReg(6), 2, wf, i, insts.Uint64ToBytes(uint64(i+2)))
		}
		wf.VCC = 0xffff0000ffff0000
		wf.EXEC = 0xff

		sp.Prepare(wf, wf)

		layout := wf.Scratchpad().AsVOP2()
		for i := 0; i < 64; i++ {
			Expect(layout.DST[i]).To(Equal(uint64(i + 2)))
			Expect(layout.SRC0[i]).To(Equal(uint64(i)))
			Expect(layout.SRC1[i]).To(Equal(uint64(i + 1)))
		}
		Expect(layout.VCC).To(Equal(uint64(0xffff0000ffff0000)))
		Expect(layout.EXEC).To(Equal(uint64(0xff)))
	})

	It("should prepare for VOP3a", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.VOP3a
		inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		inst.Src1 = insts.NewVRegOperand(2, 2, 2)
		inst.Src2 = insts.NewIntOperand(1, 1)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		for i := 0; i < 64; i++ {
			sp.writeReg(insts.VReg(0), 2, wf, i, insts.Uint64ToBytes(uint64(i)))
			sp.writeReg(insts.VReg(2), 2, wf, i, insts.Uint64ToBytes(uint64(i)))
		}
		wf.VCC = 0xffff0000ffff0000
		wf.EXEC = 0xff

		sp.Prepare(wf, wf)

		layout := wf.Scratchpad().AsVOP3A()
		for i := 0; i < 64; i++ {
			Expect(layout.SRC0[i]).To(Equal(uint64(i)))
			Expect(layout.SRC1[i]).To(Equal(uint64(i)))
			Expect(layout.SRC2[i]).To(Equal(uint64(1)))
		}
		Expect(layout.VCC).To(Equal(uint64(0xffff0000ffff0000)))
		Expect(layout.EXEC).To(Equal(uint64(0xff)))
	})

	It("should prepare for VOP3b", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.VOP3b
		inst.Src0 = insts.NewVRegOperand(0, 0, 2)
		inst.Src1 = insts.NewVRegOperand(2, 2, 2)
		inst.Src2 = insts.NewIntOperand(1, 1)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		for i := 0; i < 64; i++ {
			sp.writeReg(insts.VReg(0), 2, wf, i, insts.Uint64ToBytes(uint64(i)))
			sp.writeReg(insts.VReg(2), 2, wf, i, insts.Uint64ToBytes(uint64(i)))
		}
		wf.VCC = 0xffff0000ffff0000
		wf.EXEC = 0xff

		sp.Prepare(wf, wf)

		layout := wf.Scratchpad().AsVOP3B()
		for i := 0; i < 64; i++ {
			Expect(layout.SRC0[i]).To(Equal(uint64(i)))
			Expect(layout.SRC1[i]).To(Equal(uint64(i)))
			Expect(layout.SRC2[i]).To(Equal(uint64(1)))
		}
		Expect(layout.VCC).To(Equal(uint64(0xffff0000ffff0000)))
		Expect(layout.EXEC).To(Equal(uint64(0xff)))
	})

	It("should prepare for VOPC", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.VOPC
		inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		inst.Src1 = insts.NewVRegOperand(1, 1, 1)

		wf.SetDynamicInst(wavefront.NewInst(inst))

		for i := 0; i < 64; i++ {
			sp.writeReg(insts.VReg(0), 1, wf, i, insts.Uint64ToBytes(uint64(i)))
			sp.writeReg(insts.VReg(1), 1, wf, i, insts.Uint64ToBytes(uint64(i+1)))
		}
		wf.VCC = 1
		wf.EXEC = 0xff

		sp.Prepare(wf, wf)

		layout := wf.Scratchpad().AsVOPC()
		Expect(layout.EXEC).To(Equal(uint64(0xff)))
		for i := 0; i < 64; i++ {
			Expect(layout.SRC0[i]).To(Equal(uint64(i)))
			Expect(layout.SRC1[i]).To(Equal(uint64(i + 1)))
		}
	})

	It("should prepare for FLAT", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.FLAT
		inst.Addr = insts.NewVRegOperand(0, 0, 2)
		inst.Data = insts.NewVRegOperand(2, 2, 4)

		wf.SetDynamicInst(wavefront.NewInst(inst))

		for i := 0; i < 64; i++ {
			sp.writeReg(insts.VReg(0), 2, wf, i, insts.Uint64ToBytes(uint64(i+1024)))
			sp.writeReg(insts.VReg(2), 1, wf, i, insts.Uint32ToBytes(uint32(i)))
			sp.writeReg(insts.VReg(3), 1, wf, i, insts.Uint32ToBytes(uint32(i)))
			sp.writeReg(insts.VReg(4), 1, wf, i, insts.Uint32ToBytes(uint32(i)))
			sp.writeReg(insts.VReg(5), 1, wf, i, insts.Uint32ToBytes(uint32(i)))
		}
		wf.EXEC = 0xff

		sp.Prepare(wf, wf)

		layout := wf.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			log.Printf("iter %d", i)
			Expect(layout.ADDR[i]).To(Equal(uint64(i + 1024)))
			Expect(layout.DATA[i*4+0]).To(Equal(uint32(i)))
			Expect(layout.DATA[i*4+1]).To(Equal(uint32(i)))
			Expect(layout.DATA[i*4+2]).To(Equal(uint32(i)))
			Expect(layout.DATA[i*4+3]).To(Equal(uint32(i)))
		}
		Expect(layout.EXEC).To(Equal(uint64(0xff)))
	})

	It("should prepare for SMEM", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SMEM
		inst.Opcode = 18
		inst.Data = insts.NewSRegOperand(0, 0, 4)
		inst.Offset = insts.NewIntOperand(1, 1)
		inst.Base = insts.NewSRegOperand(4, 4, 2)

		wf.SetDynamicInst(wavefront.NewInst(inst))

		sp.writeReg(insts.SReg(0), 1, wf, 0, insts.Uint32ToBytes(100))
		sp.writeReg(insts.SReg(1), 1, wf, 0, insts.Uint32ToBytes(101))
		sp.writeReg(insts.SReg(2), 1, wf, 0, insts.Uint32ToBytes(102))
		sp.writeReg(insts.SReg(3), 1, wf, 0, insts.Uint32ToBytes(103))
		sp.writeReg(insts.SReg(4), 2, wf, 0, insts.Uint64ToBytes(1024))

		sp.Prepare(wf, wf)

		layout := wf.Scratchpad().AsSMEM()
		Expect(layout.DATA[0]).To(Equal(uint32(100)))
		Expect(layout.DATA[1]).To(Equal(uint32(101)))
		Expect(layout.DATA[2]).To(Equal(uint32(102)))
		Expect(layout.DATA[3]).To(Equal(uint32(103)))
		Expect(layout.Offset).To(Equal(uint64(1)))
		Expect(layout.Base).To(Equal(uint64(1024)))

	})

	It("should prepare for SOPP", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOPP
		inst.SImm16 = insts.NewIntOperand(1, 1)
		wf.EXEC = 0x0f
		wf.PC = 160
		wf.SCC = 1

		wf.SetDynamicInst(wavefront.NewInst(inst))

		sp.Prepare(wf, wf)

		layout := wf.Scratchpad().AsSOPP()
		Expect(layout.EXEC).To(Equal(uint64(0x0f)))
		Expect(layout.IMM).To(Equal(uint64(1)))
		Expect(layout.PC).To(Equal(uint64(160)))
		Expect(layout.SCC).To(Equal(byte(1)))
	})

	It("should prepare for SOPK", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOPK
		inst.Dst = insts.NewSRegOperand(0, 0, 1)
		inst.SImm16 = insts.NewIntOperand(1, 1)
		wf.SetDynamicInst(wavefront.NewInst(inst))
		wf.SCC = 1
		sp.writeReg(insts.SReg(0), 1, wf, 0, insts.Uint32ToBytes(100))

		sp.Prepare(wf, wf)

		layout := wf.Scratchpad().AsSOPK()
		Expect(layout.DST).To(Equal(uint64(100)))
		Expect(layout.IMM).To(Equal(uint64(1)))
		Expect(layout.SCC).To(Equal(byte(1)))
	})

	It("should prepare for SOPC", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOPC
		inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		sp.writeReg(insts.SReg(0), 1, wf, 0, insts.Uint32ToBytes(100))
		inst.Src1 = insts.NewIntOperand(192, 64)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		sp.Prepare(wf, wf)

		layout := wf.Scratchpad().AsSOPC()
		Expect(layout.SRC0).To(Equal(uint64(100)))
		Expect(layout.SRC1).To(Equal(uint64(64)))
	})

	It("should prepare for DS", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.DS
		inst.Addr = insts.NewVRegOperand(0, 0, 1)
		inst.Data = insts.NewVRegOperand(2, 2, 2)
		inst.Data1 = insts.NewVRegOperand(4, 4, 2)

		wf.SetDynamicInst(wavefront.NewInst(inst))
		wf.EXEC = uint64(0xff)

		for i := 0; i < 64; i++ {
			sp.writeReg(insts.VReg(0), 1, wf, i, insts.Uint64ToBytes(uint64(i)))
			sp.writeReg(insts.VReg(2), 1, wf, i, insts.Uint64ToBytes(uint64(i+1)))
			sp.writeReg(insts.VReg(3), 1, wf, i, insts.Uint64ToBytes(uint64(i+2)))
			sp.writeReg(insts.VReg(4), 1, wf, i, insts.Uint64ToBytes(uint64(i+3)))
			sp.writeReg(insts.VReg(5), 1, wf, i, insts.Uint64ToBytes(uint64(i+4)))
		}

		sp.Prepare(wf, wf)

		layout := wf.Scratchpad().AsDS()
		Expect(layout.EXEC).To(Equal(uint64(0xff)))

		for i := 0; i < 64; i++ {
			Expect(layout.ADDR[i]).To(Equal(uint32(i)))
			Expect(layout.DATA[i*4]).To(Equal(uint32(i + 1)))
			Expect(layout.DATA[i*4+1]).To(Equal(uint32(i + 2)))
			Expect(layout.DATA1[i*4]).To(Equal(uint32(i + 3)))
			Expect(layout.DATA1[i*4+1]).To(Equal(uint32(i + 4)))
		}
	})

	It("should commit for SOP1", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOP1
		inst.Dst = insts.NewSRegOperand(0, 0, 1)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		layout := wf.Scratchpad().AsSOP1()
		layout.DST = 517
		layout.EXEC = 0xffffffff00000000
		layout.SCC = 1

		sp.Commit(wf, wf)

		Expect(wf.SCC).To(Equal(byte(1)))
		Expect(wf.EXEC).To(Equal(uint64(0xffffffff00000000)))
		Expect(sp.readRegAsUint32(insts.SReg(0), wf, 0)).To(Equal(uint32(517)))
	})

	It("should commit for SOP2", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOP2
		inst.Dst = insts.NewSRegOperand(0, 0, 1)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		layout := wf.Scratchpad().AsSOP2()
		layout.DST = 517
		layout.SCC = 1

		sp.Commit(wf, wf)

		Expect(wf.SCC).To(Equal(byte(1)))
		Expect(sp.readRegAsUint32(insts.SReg(0), wf, 0)).To(Equal(uint32(517)))
	})

	It("should commit for VOP1", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.VOP1
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		layout := wf.Scratchpad().AsVOP1()
		layout.EXEC = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			layout.DST[i] = uint64(i)
		}
		layout.VCC = uint64(0xffff0000ffff0000)

		sp.Commit(wf, wf)

		for i := 0; i < 64; i++ {
			Expect(sp.readRegAsUint32(insts.VReg(i), wf, 0)).To(Equal(uint32(0)))
		}
		Expect(wf.VCC).To(Equal(uint64(0xffff0000ffff0000)))
	})

	It("should commit for VOP2", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.VOP2
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		layout := wf.Scratchpad().AsVOP2()
		layout.EXEC = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			layout.DST[i] = uint64(i)
		}
		layout.VCC = uint64(0xffff0000ffff0000)

		sp.Commit(wf, wf)

		for i := 0; i < 64; i++ {
			Expect(sp.readRegAsUint32(insts.VReg(i), wf, 0)).To(Equal(uint32(0)))
		}
		Expect(wf.VCC).To(Equal(uint64(0xffff0000ffff0000)))
	})

	It("should commit for VOP3a", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.VOP3a
		inst.Opcode = 449
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		layout := wf.Scratchpad().AsVOP3A()
		layout.EXEC = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			layout.DST[i] = uint64(i)
		}
		layout.VCC = uint64(0xffff0000ffff0000)

		sp.Commit(wf, wf)

		for i := 0; i < 64; i++ {
			Expect(sp.readRegAsUint32(insts.VReg(0), wf, i)).To(Equal(uint32(i)))
		}
		Expect(wf.VCC).To(Equal(uint64(0xffff0000ffff0000)))
	})

	It("should commit for VOP3a CMP", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.VOP3a
		inst.Opcode = 20
		inst.Dst = insts.NewSRegOperand(0, 0, 1)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		layout := wf.Scratchpad().AsVOP3A()
		layout.EXEC = 0xfffffffffffffffe
		for i := 0; i < 64; i++ {
			layout.DST[i] = uint64(i)
		}

		sp.Commit(wf, wf)

		Expect(sp.readRegAsUint32(insts.SReg(0), wf, 0)).To(Equal(uint32(0)))
	})

	It("should commit for VOP3b", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.VOP3b
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		inst.SDst = insts.NewSRegOperand(0, 0, 1)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		layout := wf.Scratchpad().AsVOP3B()
		layout.EXEC = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			layout.DST[i] = uint64(i)
		}
		layout.SDST = uint64(2)
		layout.VCC = uint64(0xffff0000ffff0000)

		sp.Commit(wf, wf)

		for i := 0; i < 64; i++ {
			Expect(sp.readRegAsUint32(insts.VReg(0), wf, i)).To(Equal(uint32(i)))
		}
		Expect(sp.readRegAsUint32(insts.SReg(0), wf, 0)).To(Equal(uint32(2)))
		Expect(sp.readRegAsUint32(insts.SReg(1), wf, 0)).To(Equal(uint32(0)))
		Expect(wf.VCC).To(Equal(uint64(0xffff0000ffff0000)))
	})

	It("should commit VOPC", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.VOPC
		wf.SetDynamicInst(wavefront.NewInst(inst))

		layout := wf.Scratchpad().AsVOPC()
		layout.VCC = uint64(0xff)
		layout.EXEC = uint64(0x01)

		sp.Commit(wf, wf)

		Expect(wf.VCC).To(Equal(uint64(0xff)))
		Expect(wf.EXEC).To(Equal(uint64(0x01)))
	})

	It("should commit for FLAT", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.FLAT
		inst.Opcode = 20 // Load Dword
		inst.Dst = insts.NewVRegOperand(3, 3, 4)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		layout := wf.Scratchpad().AsFlat()
		layout.EXEC = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			layout.DST[i*4+0] = uint32(i)
			layout.DST[i*4+1] = uint32(i)
			layout.DST[i*4+2] = uint32(i)
			layout.DST[i*4+3] = uint32(i)
		}

		sp.Commit(wf, wf)

		for i := 0; i < 64; i++ {
			Expect(sp.readRegAsUint32(insts.VReg(3), wf, i)).To(Equal(uint32(i)))
			Expect(sp.readRegAsUint32(insts.VReg(4), wf, i)).To(Equal(uint32(i)))
			Expect(sp.readRegAsUint32(insts.VReg(5), wf, i)).To(Equal(uint32(i)))
			Expect(sp.readRegAsUint32(insts.VReg(6), wf, i)).To(Equal(uint32(i)))
		}
	})

	It("should not commit for FLAT store operation", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.FLAT
		inst.Dst = insts.NewVRegOperand(3, 3, 4)
		inst.Opcode = 28 // Store dword
		wf.SetDynamicInst(wavefront.NewInst(inst))

		layout := wf.Scratchpad().AsFlat()
		layout.EXEC = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			layout.DST[i*4+0] = uint32(i)
			layout.DST[i*4+1] = uint32(i)
			layout.DST[i*4+2] = uint32(i)
			layout.DST[i*4+3] = uint32(i)
		}

		sp.Commit(wf, wf)

		for i := 0; i < 64; i++ {
			Expect(sp.readRegAsUint32(insts.VReg(3), wf, i)).To(Equal(uint32(0)))
			Expect(sp.readRegAsUint32(insts.VReg(4), wf, i)).To(Equal(uint32(0)))
			Expect(sp.readRegAsUint32(insts.VReg(5), wf, i)).To(Equal(uint32(0)))
			Expect(sp.readRegAsUint32(insts.VReg(6), wf, i)).To(Equal(uint32(0)))
		}
	})

	It("should commit for SMEM", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SMEM
		inst.Opcode = 4
		inst.Data = insts.NewSRegOperand(0, 0, 16)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		layout := wf.Scratchpad().AsSMEM()
		for i := 0; i < 16; i++ {
			layout.DST[i] = uint32(i)
		}

		sp.Commit(wf, wf)

		for i := 0; i < 16; i++ {
			Expect(sp.readRegAsUint32(insts.SReg(i), wf, 0)).To(Equal(uint32(i)))
		}
	})

	It("should commit for SOPC", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOPC
		wf.SetDynamicInst(wavefront.NewInst(inst))

		layout := wf.Scratchpad().AsSOPC()
		layout.SCC = 1

		sp.Commit(wf, wf)

		Expect(wf.SCC).To(Equal(byte(1)))
	})

	It("should commit for SOPK", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOPK
		inst.Dst = insts.NewSRegOperand(0, 0, 1)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		layout := wf.Scratchpad().AsSOPK()
		layout.SCC = 1
		layout.DST = 517

		sp.Commit(wf, wf)

		Expect(wf.SCC).To(Equal(byte(1)))
		Expect(sp.readRegAsUint32(insts.SReg(0), wf, 0)).To(Equal(uint32(517)))
	})

	It("should commit for SOPP", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.SOPP
		wf.SetDynamicInst(wavefront.NewInst(inst))
		wf.PC = 0

		layout := wf.Scratchpad().AsSOPP()
		layout.PC = 164

		sp.Commit(wf, wf)

		Expect(wf.PC).To(Equal(uint64(164)))
	})

	It("should commit for DS", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.DS
		inst.Dst = insts.NewVRegOperand(0, 0, 2)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		layout := wf.Scratchpad().AsDS()
		layout.EXEC = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			layout.DST[i*4] = uint32(i)
			layout.DST[i*4+1] = uint32(i + 1)
		}

		sp.Commit(wf, wf)

		for i := 0; i < 64; i++ {
			Expect(sp.readRegAsUint32(insts.VReg(0), wf, i)).To(Equal(uint32(i)))
			Expect(sp.readRegAsUint32(insts.VReg(1), wf, i)).To(Equal(uint32(i + 1)))
		}
	})

})
