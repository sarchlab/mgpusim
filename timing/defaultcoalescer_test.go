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
})
