package cp

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

var _ = Describe("DMAEngine", func() {
	var (
		engine    timing.Engine
		dmaEngine *DMAComp
		dmaMW     *dmaMiddleware

		toCP  messaging.Port
		toMem messaging.Port
	)

	const cpPort = messaging.RemotePort("CP.ToDMA")
	const memPort = messaging.RemotePort("Mem.Top")

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		reg := modeling.NewStandaloneRegistrar(engine)

		dmaEngine = MakeDMAEngineBuilder().
			WithRegistrar(reg).
			WithResources(DMAResources{
				LocalDataSource: &mem.SinglePortMapper{Port: memPort},
			}).
			Build("DMA")

		assign := func(name string) messaging.Port {
			p := modeling.MakePortBuilder().
				WithRegistrar(reg).
				WithComponent(dmaEngine).
				WithSpec(modeling.PortSpec{BufSize: 16}).
				Build(name)
			dmaEngine.AssignPort(name, p)
			(&noopConn{}).PlugIn(p)
			return p
		}

		toCP = assign("ToCP")
		toMem = assign("ToMem")

		dmaMW = dmaEngine.Middlewares()[0].(*dmaMiddleware)
	})

	makeH2DReq := func(buf []byte, dstAddress uint64) protocol.MemCopyH2DReq {
		return protocol.MemCopyH2DReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: cpPort,
				Dst: toCP.AsRemote(),
			},
			SrcBuffer:  buf,
			DstAddress: dstAddress,
		}
	}

	makeD2HReq := func(srcAddress uint64, buf []byte) protocol.MemCopyD2HReq {
		return protocol.MemCopyD2HReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: cpPort,
				Dst: toCP.AsRemote(),
			},
			SrcAddress: srcAddress,
			DstBuffer:  buf,
		}
	}

	makeReadReq := func(addr, byteSize uint64) memprotocol.ReadReq {
		return memprotocol.ReadReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: toMem.AsRemote(),
				Dst: memPort,
			},
			Address:        addr,
			AccessByteSize: byteSize,
		}
	}

	makeWriteReq := func(addr uint64) memprotocol.WriteReq {
		return memprotocol.WriteReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: toMem.AsRemote(),
				Dst: memPort,
			},
			Address: addr,
		}
	}

	It("should stall if dma is processing max request number", func() {
		for i := 0; i < int(dmaEngine.Spec().MaxRequestCount); i++ {
			req := makeH2DReq(make([]byte, 128), uint64(20+128*i))
			rqC := NewRequestCollection(req)

			dmaMW.processingReqs = append(dmaMW.processingReqs, rqC)
		}

		madeProgress := dmaMW.parseFromCP()

		Expect(dmaMW.toSendToMem).To(HaveLen(0))
		Expect(madeProgress).To(BeFalse())
	})

	It("should parse MemCopyH2D from CP", func() {
		req := makeH2DReq(make([]byte, 128), 20)
		toCP.Deliver(req)

		madeProgress := dmaMW.parseFromCP()

		Expect(madeProgress).To(BeTrue())
		Expect(dmaMW.processingReqs[0].superiorRequest.Meta().ID).
			To(Equal(req.ID))
		Expect(dmaMW.toSendToMem).To(HaveLen(3))
		Expect(dmaMW.toSendToMem[0].(memprotocol.WriteReq).Address).
			To(Equal(uint64(20)))
		Expect(dmaMW.toSendToMem[1].(memprotocol.WriteReq).Address).
			To(Equal(uint64(64)))
		Expect(dmaMW.toSendToMem[2].(memprotocol.WriteReq).Address).
			To(Equal(uint64(128)))
		Expect(dmaMW.pendingReqs).To(HaveLen(3))
	})

	It("should parse MemCopyD2H from CP", func() {
		req := makeD2HReq(20, make([]byte, 128))
		toCP.Deliver(req)

		madeProgress := dmaMW.parseFromCP()

		Expect(madeProgress).To(BeTrue())
		Expect(dmaMW.processingReqs[0].superiorRequest.Meta().ID).
			To(Equal(req.ID))
		Expect(dmaMW.toSendToMem).To(HaveLen(3))
		Expect(dmaMW.toSendToMem[0].(memprotocol.ReadReq).Address).
			To(Equal(uint64(20)))
		Expect(dmaMW.toSendToMem[0].(memprotocol.ReadReq).AccessByteSize).
			To(Equal(uint64(44)))
		Expect(dmaMW.toSendToMem[1].(memprotocol.ReadReq).Address).
			To(Equal(uint64(64)))
		Expect(dmaMW.toSendToMem[2].(memprotocol.ReadReq).Address).
			To(Equal(uint64(128)))
		Expect(dmaMW.pendingReqs).To(HaveLen(3))
	})

	It("should parse DataReady from mem", func() {
		dstBuf := make([]byte, 128)
		req := makeD2HReq(20, dstBuf)
		rqC := NewRequestCollection(req)
		dmaMW.processingReqs = append(dmaMW.processingReqs, rqC)

		reqToBottom1 := makeReadReq(20, 44)
		reqToBottom2 := makeReadReq(64, 64)
		reqToBottom3 := makeReadReq(128, 20)
		dmaMW.pendingReqs = append(dmaMW.pendingReqs, reqToBottom1)
		rqC.appendSubordinateID(reqToBottom1.Meta().ID)
		dmaMW.pendingReqs = append(dmaMW.pendingReqs, reqToBottom2)
		rqC.appendSubordinateID(reqToBottom2.Meta().ID)
		dmaMW.pendingReqs = append(dmaMW.pendingReqs, reqToBottom3)
		rqC.appendSubordinateID(reqToBottom3.Meta().ID)

		data := []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}
		dataReady := memprotocol.DataReadyRsp{
			MsgMeta: messaging.MsgMeta{
				ID:    timing.GetIDGenerator().Generate(),
				Src:   memPort,
				Dst:   toMem.AsRemote(),
				RspTo: reqToBottom2.ID,
			},
			Data: data,
		}
		toMem.Deliver(dataReady)

		madeProgress := dmaMW.parseFromMem()

		Expect(madeProgress).To(BeTrue())
		Expect(dmaMW.processingReqs[0]).To(BeIdenticalTo(rqC))
		Expect(dmaMW.pendingReqs).To(HaveLen(2))
		Expect(dstBuf[44:108]).To(Equal(data))
	})

	It("should respond MemCopyD2H", func() {
		dstBuf := make([]byte, 128)
		req := makeD2HReq(20, dstBuf)
		rqC := NewRequestCollection(req)
		dmaMW.processingReqs = append(dmaMW.processingReqs, rqC)

		reqToBottom2 := makeReadReq(64, 64)
		dmaMW.pendingReqs = append(dmaMW.pendingReqs, reqToBottom2)
		rqC.appendSubordinateID(reqToBottom2.Meta().ID)

		data := []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}
		dataReady := memprotocol.DataReadyRsp{
			MsgMeta: messaging.MsgMeta{
				ID:    timing.GetIDGenerator().Generate(),
				Src:   memPort,
				Dst:   toMem.AsRemote(),
				RspTo: reqToBottom2.ID,
			},
			Data: data,
		}
		toMem.Deliver(dataReady)

		madeProgress := dmaMW.parseFromMem()

		Expect(madeProgress).To(BeTrue())
		Expect(dmaMW.processingReqs).To(BeEmpty())
		Expect(dmaMW.pendingReqs).To(BeEmpty())
		Expect(dstBuf[44:108]).To(Equal(data))

		rsp := dmaMW.toSendToCP[0].(protocol.GeneralRsp)
		Expect(rsp.RspTo).To(Equal(req.ID))
		Expect(rsp.Dst).To(Equal(cpPort))
	})

	It("should parse Done from mem", func() {
		req := makeH2DReq(make([]byte, 128), 20)
		rqC := NewRequestCollection(req)
		dmaMW.processingReqs = append(dmaMW.processingReqs, rqC)

		reqToBottom1 := makeWriteReq(20)
		reqToBottom2 := makeWriteReq(64)
		reqToBottom3 := makeWriteReq(128)

		dmaMW.pendingReqs = append(dmaMW.pendingReqs, reqToBottom1)
		rqC.appendSubordinateID(reqToBottom1.Meta().ID)
		dmaMW.pendingReqs = append(dmaMW.pendingReqs, reqToBottom2)
		rqC.appendSubordinateID(reqToBottom2.Meta().ID)
		dmaMW.pendingReqs = append(dmaMW.pendingReqs, reqToBottom3)
		rqC.appendSubordinateID(reqToBottom3.Meta().ID)

		done := memprotocol.WriteDoneRsp{
			MsgMeta: messaging.MsgMeta{
				ID:    timing.GetIDGenerator().Generate(),
				Src:   memPort,
				Dst:   toMem.AsRemote(),
				RspTo: reqToBottom2.ID,
			},
		}
		toMem.Deliver(done)

		madeProgress := dmaMW.parseFromMem()

		Expect(madeProgress).To(BeTrue())
		Expect(dmaMW.processingReqs[0]).To(BeIdenticalTo(rqC))
		Expect(dmaMW.pendingReqs).To(HaveLen(2))
	})

	It("should send MemCopyH2D rsp to CP", func() {
		req := makeH2DReq(make([]byte, 128), 20)
		rqC := NewRequestCollection(req)
		dmaMW.processingReqs = append(dmaMW.processingReqs, rqC)

		reqToBottom2 := makeWriteReq(64)
		dmaMW.pendingReqs = append(dmaMW.pendingReqs, reqToBottom2)
		rqC.appendSubordinateID(reqToBottom2.Meta().ID)

		done := memprotocol.WriteDoneRsp{
			MsgMeta: messaging.MsgMeta{
				ID:    timing.GetIDGenerator().Generate(),
				Src:   memPort,
				Dst:   toMem.AsRemote(),
				RspTo: reqToBottom2.ID,
			},
		}
		toMem.Deliver(done)

		madeProgress := dmaMW.parseFromMem()

		Expect(madeProgress).To(BeTrue())
		Expect(dmaMW.processingReqs).To(BeEmpty())
		Expect(dmaMW.pendingReqs).To(BeEmpty())

		rsp := dmaMW.toSendToCP[0].(protocol.GeneralRsp)
		Expect(rsp.RspTo).To(Equal(req.ID))
		Expect(rsp.Dst).To(Equal(cpPort))
	})

	It("should copy data end to end", func() {
		req := makeH2DReq([]byte{1, 2, 3, 4}, 20)
		toCP.Deliver(req)

		for dmaEngine.Tick() {
		}

		write := toMem.RetrieveOutgoing().(memprotocol.WriteReq)
		Expect(write.Address).To(Equal(uint64(20)))
		Expect(write.Data).To(Equal([]byte{1, 2, 3, 4}))

		done := memprotocol.WriteDoneRsp{
			MsgMeta: messaging.MsgMeta{
				ID:    timing.GetIDGenerator().Generate(),
				Src:   memPort,
				Dst:   toMem.AsRemote(),
				RspTo: write.ID,
			},
		}
		toMem.Deliver(done)

		for dmaEngine.Tick() {
		}

		rsp := toCP.RetrieveOutgoing().(protocol.GeneralRsp)
		Expect(rsp.RspTo).To(Equal(req.ID))
	})
})
