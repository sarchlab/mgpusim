package emu

import (
	"bytes"
	"encoding/binary"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3/insts"
)

var _ = Describe("ScratchpadPreparer", func() {
	var (
		sp *ScratchpadPreparerImpl
		wf *Wavefront
	)

	BeforeEach(func() {
		sp = NewScratchpadPreparerImpl()
		wf = NewWavefront(nil)
	})

	It("should prepare for SOP2", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.Sop2
		inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		inst.Src1 = insts.NewIntOperand(1, 1)
		wf.inst = inst

		copy(wf.SRegFile[0:4], insts.Uint32ToBytes(517))
		wf.SCC = 1

		sp.Prepare(wf, wf)

		layout := wf.Scratchpad().AsSOP2()
		binary.Read(bytes.NewBuffer(wf.scratchpad), binary.LittleEndian, layout)
		Expect(layout.SRC0).To(Equal(uint64(517)))
		Expect(layout.SRC1).To(Equal(uint64(1)))
		Expect(layout.SCC).To(Equal(byte(1)))

	})

	It("should prepare for VOP1", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.Vop1
		inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		wf.inst = inst

		for i := 0; i < 64; i++ {
			copy(wf.VRegFile[i*256*4:i*256*4+4], insts.Uint32ToBytes(uint32(i)))
		}

		sp.Prepare(wf, wf)

		layout := wf.Scratchpad().AsVop1()
		for i := 0; i < 64; i++ {
			Expect(layout.SRC0[i]).To(Equal(uint64(i)))
		}
	})

	It("should commit for SOP2", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.Sop2
		inst.Dst = insts.NewSRegOperand(0, 0, 1)
		wf.inst = inst

		layout := wf.Scratchpad().AsSOP2()
		layout.DST = 517
		layout.SCC = 1

		sp.Commit(wf, wf)

		Expect(wf.SCC).To(Equal(byte(1)))
		Expect(wf.SRegValue(0)).To(Equal(uint32(517)))
	})

	It("should commit for VOP1", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.Vop1
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		wf.inst = inst

		layout := wf.Scratchpad().AsVop1()

		for i := 0; i < 64; i++ {
			layout.DST[i] = uint64(i)
		}

		sp.Commit(wf, wf)

		for i := 0; i < 64; i++ {
			Expect(wf.VRegValue(i, 0)).To(Equal(uint32(i)))
		}
	})
})
