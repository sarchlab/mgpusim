package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/timing/wavefront"
)

var _ = Describe("Default Coalescer", func() {
	var (
		wf          *wavefront.Wavefront
		c           defaultCoalescer
		regAccessor *mockRegFileAccessor
	)

	BeforeEach(func() {
		wf = wavefront.NewWavefront(nil)
		c = defaultCoalescer{
			log2CacheLineSize: 6,
		}
		regAccessor = newMockRegFileAccessor()
		wf.RegAccessor = regAccessor
	})

	It("should coalesce to a single cacheline", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.FLAT
		inst.Opcode = 20 // flat_load_dword
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		inst.Addr = insts.NewVRegOperand(2, 2, 2)
		wf.SetDynamicInst(wavefront.NewInst(inst))
		wf.SetEXEC(0xffffffffffffffff)

		// Set all 64 lanes' ADDR (v2:v3) to 0x1000
		for i := 0; i < 64; i++ {
			addrReg := insts.VReg(2)
			regAccessor.setRegValue(addrReg, 2, i, wf.VRegOffset,
				insts.Uint64ToBytes(0x1000)[:8])
		}

		memTransactions := c.generateMemTransactions(wf)

		Expect(memTransactions).To(HaveLen(1))
		Expect(memTransactions[0].laneInfo).To(HaveLen(64))
	})

	It("should coalesce to multiple cachelines", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.FLAT
		inst.Opcode = 20 // flat_load_dword
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		inst.Addr = insts.NewVRegOperand(2, 2, 2)
		wf.SetDynamicInst(wavefront.NewInst(inst))
		wf.SetEXEC(0xffffffffffffffff)

		for i := 0; i < 64; i++ {
			addrReg := insts.VReg(2)
			regAccessor.setRegValue(addrReg, 2, i, wf.VRegOffset,
				insts.Uint64ToBytes(uint64(0x1000+i*4))[:8])
		}

		memTransactions := c.generateMemTransactions(wf)

		Expect(memTransactions).To(HaveLen(4))
		Expect(memTransactions[0].laneInfo).To(HaveLen(16))
		Expect(memTransactions[1].laneInfo).To(HaveLen(16))
		Expect(memTransactions[2].laneInfo).To(HaveLen(16))
		Expect(memTransactions[3].laneInfo).To(HaveLen(16))
	})

	It("should not generate cross-cache-line requests", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.FLAT
		inst.Opcode = 21 // flat_load_dwordx2
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		inst.Addr = insts.NewVRegOperand(4, 4, 2)
		wf.SetDynamicInst(wavefront.NewInst(inst))
		wf.SetEXEC(0xffffffffffffffff)

		for i := 0; i < 64; i++ {
			addrReg := insts.VReg(4)
			regAccessor.setRegValue(addrReg, 2, i, wf.VRegOffset,
				insts.Uint64ToBytes(uint64(0x1004+i*4))[:8])
		}

		memTransactions := c.generateMemTransactions(wf)

		Expect(memTransactions).To(HaveLen(5))
		Expect(memTransactions[0].laneInfo).To(HaveLen(29))
		Expect(memTransactions[1].laneInfo).To(HaveLen(32))
		Expect(memTransactions[2].laneInfo).To(HaveLen(32))
		Expect(memTransactions[3].laneInfo).To(HaveLen(32))
		Expect(memTransactions[4].laneInfo).To(HaveLen(3))
	})

	It("should coalesce store instructions", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.FLAT
		inst.Opcode = 28 // flat_store_dword
		inst.Addr = insts.NewVRegOperand(2, 2, 2)
		inst.Data = insts.NewVRegOperand(4, 4, 1)
		wf.SetDynamicInst(wavefront.NewInst(inst))
		wf.SetEXEC(0xffffffffffffffff)

		for i := 0; i < 64; i++ {
			addrReg := insts.VReg(2)
			regAccessor.setRegValue(addrReg, 2, i, wf.VRegOffset,
				insts.Uint64ToBytes(uint64(0x1000+i*4))[:8])

			dataReg := insts.VReg(4)
			regAccessor.setRegValue(dataReg, 1, i, wf.VRegOffset,
				insts.Uint32ToBytes(1))
		}

		memTransactions := c.generateMemTransactions(wf)

		Expect(memTransactions).To(HaveLen(4))
	})
})
