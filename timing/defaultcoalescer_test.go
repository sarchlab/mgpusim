package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/timing/wavefront"
)

var _ = Describe("Default Coalescer", func() {
	var (
		wf *wavefront.Wavefront
		c  defaultCoalescer
	)

	BeforeEach(func() {
		wf = wavefront.NewWavefront(nil)
		c = defaultCoalescer{
			log2CacheLineSize: 6,
		}
	})

	It("should coalesce to a single cacheline", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.FLAT
		inst.Opcode = 20 // flat_load_dword
		inst.Dst = insts.NewRegOperand(0, 0, 1)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		sp := wf.Scratchpad().AsFlat()
		sp.EXEC = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			sp.ADDR[i] = 0x1000
		}

		memTransactions := c.generateMemTransactions(wf)

		Expect(memTransactions).To(HaveLen(1))
		Expect(memTransactions[0].laneInfo).To(HaveLen(64))
	})

	It("should coalesce to multiple cachelines", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.FLAT
		inst.Opcode = 20 // flat_load_dword
		inst.Dst = insts.NewRegOperand(0, 0, 1)
		wf.SetDynamicInst(wavefront.NewInst(inst))

		sp := wf.Scratchpad().AsFlat()
		sp.EXEC = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			sp.ADDR[i] = uint64(0x1000 + i*4)
		}

		memTransactions := c.generateMemTransactions(wf)

		Expect(memTransactions).To(HaveLen(4))
		Expect(memTransactions[0].laneInfo).To(HaveLen(16))
		Expect(memTransactions[1].laneInfo).To(HaveLen(16))
		Expect(memTransactions[2].laneInfo).To(HaveLen(16))
		Expect(memTransactions[3].laneInfo).To(HaveLen(16))
	})

	It("should coalesce store instructions", func() {
		inst := insts.NewInst()
		inst.FormatType = insts.FLAT
		inst.Opcode = 28 // flat_store_dword
		wf.SetDynamicInst(wavefront.NewInst(inst))

		sp := wf.Scratchpad().AsFlat()
		sp.EXEC = 0xffffffffffffffff
		for i := 0; i < 64; i++ {
			sp.ADDR[i] = uint64(0x1000 + i*4)
			sp.DATA[i*4] = 1
		}

		memTransactions := c.generateMemTransactions(wf)

		Expect(memTransactions).To(HaveLen(4))
	})
})
