package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/mem"
	"gitlab.com/yaotsu/mem/cache"
)

var _ = Describe("Vector Memory Unit", func() {

	var (
		cu        *ComputeUnit
		sp        *mockScratchpadPreparer
		bu        *VectorMemoryUnit
		vectorMem *core.MockComponent
		conn      *core.MockConnection
	)

	BeforeEach(func() {
		cu = NewComputeUnit("cu", nil)
		sp = new(mockScratchpadPreparer)
		bu = NewVectorMemoryUnit(cu, sp)
		vectorMem = core.NewMockComponent("VectorMem")
		conn = core.NewMockConnection()

		cu.VectorMemModules = new(cache.SingleLowModuleFinder)
		conn.PlugIn(cu.ToVectorMem)
	})

	It("should allow accepting wavefront", func() {
		bu.toRead = nil
		Expect(bu.CanAcceptWave()).To(BeTrue())
	})

	It("should not allow accepting wavefront is the read stage buffer is occupied", func() {
		bu.toRead = new(Wavefront)
		Expect(bu.CanAcceptWave()).To(BeFalse())
	})

	It("should accept wave", func() {
		wave := new(Wavefront)
		bu.AcceptWave(wave, 10)
		Expect(bu.toRead).To(BeIdenticalTo(wave))
	})

	It("should run flat_load_dword", func() {
		wave := NewWavefront(nil)
		inst := NewInst(insts.NewInst())
		inst.Format = insts.FormatTable[insts.FLAT]
		inst.Opcode = 20
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		wave.inst = inst

		sp := wave.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			sp.ADDR[i] = uint64(4096 + i*4)
		}

		bu.toExec = wave

		bu.Run(10)

		Expect(wave.State).To(Equal(WfReady))
		Expect(wave.OutstandingVectorMemAccess).To(Equal(1))
		Expect(cu.inFlightMemAccess).To(HaveLen(4))
		Expect(bu.ReadBuf).To(HaveLen(4))
	})

	It("should run flat_store_dword", func() {
		wave := NewWavefront(nil)
		inst := NewInst(insts.NewInst())
		inst.Format = insts.FormatTable[insts.FLAT]
		inst.Opcode = 28
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		wave.inst = inst

		sp := wave.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			sp.ADDR[i] = uint64(4096 + i*4)
			sp.DATA[i*4] = uint32(i)
		}

		bu.toExec = wave

		bu.Run(10)

		Expect(wave.State).To(Equal(WfReady))
		Expect(wave.OutstandingVectorMemAccess).To(Equal(1))
		Expect(cu.inFlightMemAccess).To(HaveLen(4))
		Expect(bu.WriteBuf).To(HaveLen(4))
	})

	It("should send memory access requests", func() {
		loadReq := mem.NewReadReq(10, cu.ToVectorMem, vectorMem.ToOutside, 0, 4)
		loadReq.SetSendTime(10)
		bu.ReadBuf = append(bu.ReadBuf, loadReq)

		storeReq := mem.NewWriteReq(10, cu.ToVectorMem, vectorMem.ToOutside, 0)
		bu.WriteBuf = append(bu.WriteBuf, storeReq)

		conn.ExpectSend(loadReq, nil)
		conn.ExpectSend(storeReq, nil)

		bu.Run(10)

		Expect(conn.AllExpectedSent()).To(BeTrue())
		Expect(len(bu.ReadBuf)).To(Equal(0))
		Expect(len(bu.WriteBuf)).To(Equal(0))
	})

	It("should not remove request from read buffer, if send fails", func() {
		loadReq := mem.NewReadReq(10, cu.ToVectorMem, vectorMem.ToOutside, 0, 4)
		bu.ReadBuf = append(bu.ReadBuf, loadReq)

		err := core.NewSendError()
		conn.ExpectSend(loadReq, err)

		bu.Run(10)

		Expect(conn.AllExpectedSent()).To(BeTrue())
		Expect(len(bu.ReadBuf)).To(Equal(1))
	})

})
