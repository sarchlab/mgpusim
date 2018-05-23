package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/mem"
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

		cu.VectorMem = vectorMem
		core.PlugIn(cu, "ToVectorMem", conn)
	})

	It("should allow accepting wavefront", func() {
		// wave := new(Wavefront)
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

		info := new(MemAccessInfo)
		info.Action = MemAccessVectorDataLoad
		info.Inst = inst
		info.Dst = insts.VReg(0)
		info.Wf = wave
		info.TotalReqs = 4
		for i := 0; i < 64; i++ {
			info.PreCoalescedAddrs[i] = uint64(4096 + i*4)
		}
		expectedReq := mem.NewAccessReq()
		expectedReq.Address = 4096
		expectedReq.ByteSize = 64
		expectedReq.Type = mem.Read
		expectedReq.SetSrc(cu)
		expectedReq.SetDst(vectorMem)
		expectedReq.SetSendTime(10)
		expectedReq.Info = info
		conn.ExpectSend(expectedReq, nil)

		expectedReq = mem.NewAccessReq()
		expectedReq.Address = 4096 + 64
		expectedReq.ByteSize = 64
		expectedReq.Type = mem.Read
		expectedReq.SetSrc(cu)
		expectedReq.SetDst(vectorMem)
		expectedReq.SetSendTime(10)
		expectedReq.Info = info
		conn.ExpectSend(expectedReq, nil)

		expectedReq = mem.NewAccessReq()
		expectedReq.Address = 4096 + 64*2
		expectedReq.ByteSize = 64
		expectedReq.Type = mem.Read
		expectedReq.SetSrc(cu)
		expectedReq.SetDst(vectorMem)
		expectedReq.SetSendTime(10)
		expectedReq.Info = info
		conn.ExpectSend(expectedReq, nil)

		expectedReq = mem.NewAccessReq()
		expectedReq.Address = 4096 + 64*3
		expectedReq.ByteSize = 64
		expectedReq.Type = mem.Read
		expectedReq.SetSrc(cu)
		expectedReq.SetDst(vectorMem)
		expectedReq.SetSendTime(10)
		expectedReq.Info = info
		conn.ExpectSend(expectedReq, nil)

		bu.Run(10)

		Expect(conn.AllExpectedSent()).To(BeTrue())
		Expect(wave.State).To(Equal(WfReady))
		Expect(wave.OutstandingVectorMemAccess).To(Equal(1))
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

		info := new(MemAccessInfo)
		info.Action = MemAccessVectorDataLoad
		info.Inst = inst
		info.Dst = insts.VReg(0)
		info.Wf = wave
		info.TotalReqs = 4
		for i := 0; i < 64; i++ {
			info.PreCoalescedAddrs[i] = uint64(4096 + i*4)
		}
		expectedReq := mem.NewAccessReq()
		expectedReq.Address = 4096
		expectedReq.ByteSize = 64
		expectedReq.Type = mem.Read
		expectedReq.SetSrc(cu)
		expectedReq.SetDst(vectorMem)
		expectedReq.SetSendTime(10)
		expectedReq.Info = info
		expectedReq.Buf = make([]byte, 64)
		for i := 0; i < 16; i++ {
			copy(expectedReq.Buf[i*4:i*4+4], insts.Uint32ToBytes(uint32(i)))
		}
		conn.ExpectSend(expectedReq, nil)

		expectedReq = mem.NewAccessReq()
		expectedReq.Address = 4096 + 64
		expectedReq.ByteSize = 64
		expectedReq.Type = mem.Read
		expectedReq.SetSrc(cu)
		expectedReq.SetDst(vectorMem)
		expectedReq.SetSendTime(10)
		expectedReq.Info = info
		expectedReq.Buf = make([]byte, 64)
		for i := 0; i < 16; i++ {
			copy(expectedReq.Buf[i*4:i*4+4], insts.Uint32ToBytes(uint32(i+16)))
		}
		conn.ExpectSend(expectedReq, nil)

		expectedReq = mem.NewAccessReq()
		expectedReq.Address = 4096 + 64*2
		expectedReq.ByteSize = 64
		expectedReq.Type = mem.Read
		expectedReq.SetSrc(cu)
		expectedReq.SetDst(vectorMem)
		expectedReq.SetSendTime(10)
		expectedReq.Info = info
		expectedReq.Buf = make([]byte, 64)
		for i := 0; i < 16; i++ {
			copy(expectedReq.Buf[i*4:i*4+4], insts.Uint32ToBytes(uint32(i+32)))
		}
		conn.ExpectSend(expectedReq, nil)

		expectedReq = mem.NewAccessReq()
		expectedReq.Address = 4096 + 64*3
		expectedReq.ByteSize = 64
		expectedReq.Type = mem.Read
		expectedReq.SetSrc(cu)
		expectedReq.SetDst(vectorMem)
		expectedReq.SetSendTime(10)
		expectedReq.Info = info
		expectedReq.Buf = make([]byte, 64)
		for i := 0; i < 16; i++ {
			copy(expectedReq.Buf[i*4:i*4+4], insts.Uint32ToBytes(uint32(i+48)))
		}
		conn.ExpectSend(expectedReq, nil)

		bu.Run(10)

		Expect(conn.AllExpectedSent()).To(BeTrue())
		Expect(wave.State).To(Equal(WfReady))
		Expect(wave.OutstandingVectorMemAccess).To(Equal(1))
	})

})
