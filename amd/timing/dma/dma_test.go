package dma

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

var _ = Describe("DMAEngine", func() {
	var (
		reg       *sim.Simulation
		dmaEngine *Comp
		dmaMW     *dmaMiddleware

		toCP  messaging.Port
		toMem messaging.Port
	)

	const cpPort = messaging.RemotePort("CP.ToDMA")
	const memPort = messaging.RemotePort("Mem.Top")

	BeforeEach(func() {
		reg = sim.MakeBuilder().WithTaskTracking().Build()
		DeferCleanup(func() { Expect(reg.Terminate()).To(Succeed()) })

		dmaEngine = MakeBuilder().
			WithSimulation(reg).
			WithSpec(func() Spec {
				spec := DefaultSpec()
				spec.LocalDataSource = mem.AddressMapper{Destinations: []messaging.RemotePort{memPort}}
				return spec
			}()).
			Build("DMA")

		assign := func(name string) messaging.Port {
			p := twowaybuffered.MakeBuilder().
				WithSimulation(reg).
				WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
				Build()
			dmaEngine.BindPort(name, p)
			(&noopConn{}).BindPort(p)
			return p
		}

		toCP = assign("ToCP")
		toMem = assign("ToMem")

		Expect(reg.Initialize()).To(Succeed())
		dmaMW = dmaEngine.Middlewares.DMA
	})

	makeH2DReq := func(buf []byte, dstAddress uint64) messaging.Msg {
		return messaging.Msg{ID: dmaEngine.NewID(),
			Src: cpPort,
			Dst: toCP.AsRemote(), Payload: protocol.MemCopyH2DReq{SrcBuffer: buf,
				DstAddress: dstAddress}}

	}

	makeD2HReq := func(srcAddress uint64, buf []byte) messaging.Msg {
		return messaging.Msg{ID: dmaEngine.NewID(),
			Src: cpPort,
			Dst: toCP.AsRemote(), Payload: protocol.MemCopyD2HReq{SrcAddress: srcAddress,
				ByteSize: uint64(len(buf))}}

	}

	makeReadReq := func(addr, byteSize uint64) messaging.Msg {
		return messaging.Msg{ID: dmaEngine.NewID(),
			Src: toMem.AsRemote(),
			Dst: memPort, Payload: memprotocol.ReadReq{Address: addr,
				AccessByteSize: byteSize}}

	}

	makeWriteReq := func(addr uint64) messaging.Msg {
		return messaging.Msg{ID: dmaEngine.NewID(),
			Src: toMem.AsRemote(),
			Dst: memPort, Payload: memprotocol.WriteReq{Address: addr}}

	}

	It("should stall if dma is processing max request number", func() {
		for i := 0; i < int(dmaEngine.Spec.MaxRequestCount); i++ {
			req := makeH2DReq(make([]byte, 128), uint64(20+128*i))
			rqC := NewRequestCollection(req)
			if body, ok := req.Payload.(protocol.MemCopyD2HReq); ok {
				rqC.Data = make([]byte, body.ByteSize)
			}

			dmaMW.comp.State.ProcessingReqs = append(dmaMW.comp.State.ProcessingReqs, *rqC)
			rqC = &dmaMW.comp.State.ProcessingReqs[len(dmaMW.comp.State.ProcessingReqs)-1]
		}

		madeProgress := dmaMW.parseFromCP()

		Expect(dmaMW.comp.State.ToSendToMem).To(HaveLen(0))
		Expect(madeProgress).To(BeFalse())
	})

	It("should parse MemCopyH2D from CP", func() {
		req := makeH2DReq(make([]byte, 128), 20)
		toCP.Deliver(req)

		madeProgress := dmaMW.parseFromCP()

		Expect(madeProgress).To(BeTrue())
		Expect(dmaMW.comp.State.ProcessingReqs[0].SuperiorRequest.ID).
			To(Equal(req.ID))
		Expect(dmaMW.comp.State.ToSendToMem).To(HaveLen(3))
		Expect(dmaMW.comp.State.ToSendToMem[0].Payload.(memprotocol.WriteReq).Address).
			To(Equal(uint64(20)))
		Expect(dmaMW.comp.State.ToSendToMem[1].Payload.(memprotocol.WriteReq).Address).
			To(Equal(uint64(64)))
		Expect(dmaMW.comp.State.ToSendToMem[2].Payload.(memprotocol.WriteReq).Address).
			To(Equal(uint64(128)))
		Expect(dmaMW.comp.State.PendingReqs).To(HaveLen(3))
	})

	It("should parse MemCopyD2H from CP", func() {
		req := makeD2HReq(20, make([]byte, 128))
		toCP.Deliver(req)

		madeProgress := dmaMW.parseFromCP()

		Expect(madeProgress).To(BeTrue())
		Expect(dmaMW.comp.State.ProcessingReqs[0].SuperiorRequest.ID).
			To(Equal(req.ID))
		Expect(dmaMW.comp.State.ToSendToMem).To(HaveLen(3))
		Expect(dmaMW.comp.State.ToSendToMem[0].Payload.(memprotocol.ReadReq).Address).
			To(Equal(uint64(20)))
		Expect(dmaMW.comp.State.ToSendToMem[0].Payload.(memprotocol.ReadReq).AccessByteSize).
			To(Equal(uint64(44)))
		Expect(dmaMW.comp.State.ToSendToMem[1].Payload.(memprotocol.ReadReq).Address).
			To(Equal(uint64(64)))
		Expect(dmaMW.comp.State.ToSendToMem[2].Payload.(memprotocol.ReadReq).Address).
			To(Equal(uint64(128)))
		Expect(dmaMW.comp.State.PendingReqs).To(HaveLen(3))
	})

	It("should parse DataReady from mem", func() {
		dstBuf := make([]byte, 128)
		req := makeD2HReq(20, dstBuf)
		rqC := NewRequestCollection(req)
		if body, ok := req.Payload.(protocol.MemCopyD2HReq); ok {
			rqC.Data = make([]byte, body.ByteSize)
		}
		dmaMW.comp.State.ProcessingReqs = append(dmaMW.comp.State.ProcessingReqs, *rqC)
		rqC = &dmaMW.comp.State.ProcessingReqs[len(dmaMW.comp.State.ProcessingReqs)-1]

		reqToBottom1 := makeReadReq(20, 44)
		reqToBottom2 := makeReadReq(64, 64)
		reqToBottom3 := makeReadReq(128, 20)
		dmaMW.comp.State.PendingReqs = append(dmaMW.comp.State.PendingReqs, reqToBottom1)
		rqC.appendSubordinateID(reqToBottom1.ID)
		dmaMW.comp.State.PendingReqs = append(dmaMW.comp.State.PendingReqs, reqToBottom2)
		rqC.appendSubordinateID(reqToBottom2.ID)
		dmaMW.comp.State.PendingReqs = append(dmaMW.comp.State.PendingReqs, reqToBottom3)
		rqC.appendSubordinateID(reqToBottom3.ID)

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
		dataReady := messaging.Msg{ID: dmaEngine.NewID(),
			Src:   memPort,
			Dst:   toMem.AsRemote(),
			RspTo: reqToBottom2.ID, Payload: memprotocol.DataReadyRsp{Data: data}}

		toMem.Deliver(dataReady)

		madeProgress := dmaMW.parseFromMem()

		Expect(madeProgress).To(BeTrue())
		Expect(dmaMW.comp.State.ProcessingReqs[0]).To(Equal(*rqC))
		Expect(dmaMW.comp.State.PendingReqs).To(HaveLen(2))
		Expect(rqC.Data[44:108]).To(Equal(data))
	})

	It("should respond MemCopyD2H", func() {
		dstBuf := make([]byte, 128)
		req := makeD2HReq(20, dstBuf)
		rqC := NewRequestCollection(req)
		if body, ok := req.Payload.(protocol.MemCopyD2HReq); ok {
			rqC.Data = make([]byte, body.ByteSize)
		}
		dmaMW.comp.State.ProcessingReqs = append(dmaMW.comp.State.ProcessingReqs, *rqC)
		rqC = &dmaMW.comp.State.ProcessingReqs[len(dmaMW.comp.State.ProcessingReqs)-1]

		reqToBottom2 := makeReadReq(64, 64)
		dmaMW.comp.State.PendingReqs = append(dmaMW.comp.State.PendingReqs, reqToBottom2)
		rqC.appendSubordinateID(reqToBottom2.ID)

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
		dataReady := messaging.Msg{ID: dmaEngine.NewID(),
			Src:   memPort,
			Dst:   toMem.AsRemote(),
			RspTo: reqToBottom2.ID, Payload: memprotocol.DataReadyRsp{Data: data}}

		toMem.Deliver(dataReady)

		madeProgress := dmaMW.parseFromMem()

		Expect(madeProgress).To(BeTrue())
		Expect(dmaMW.comp.State.ProcessingReqs).To(BeEmpty())
		Expect(dmaMW.comp.State.PendingReqs).To(BeEmpty())
		Expect(dmaMW.comp.State.ToSendToCP[0].Payload.(protocol.MemCopyD2HRsp).Data[44:108]).To(Equal(data))

		rsp := dmaMW.comp.State.ToSendToCP[0]
		Expect(rsp.RspTo).To(Equal(req.ID))
		Expect(rsp.Dst).To(Equal(cpPort))
	})

	It("should parse Done from mem", func() {
		req := makeH2DReq(make([]byte, 128), 20)
		rqC := NewRequestCollection(req)
		if body, ok := req.Payload.(protocol.MemCopyD2HReq); ok {
			rqC.Data = make([]byte, body.ByteSize)
		}
		dmaMW.comp.State.ProcessingReqs = append(dmaMW.comp.State.ProcessingReqs, *rqC)
		rqC = &dmaMW.comp.State.ProcessingReqs[len(dmaMW.comp.State.ProcessingReqs)-1]

		reqToBottom1 := makeWriteReq(20)
		reqToBottom2 := makeWriteReq(64)
		reqToBottom3 := makeWriteReq(128)

		dmaMW.comp.State.PendingReqs = append(dmaMW.comp.State.PendingReqs, reqToBottom1)
		rqC.appendSubordinateID(reqToBottom1.ID)
		dmaMW.comp.State.PendingReqs = append(dmaMW.comp.State.PendingReqs, reqToBottom2)
		rqC.appendSubordinateID(reqToBottom2.ID)
		dmaMW.comp.State.PendingReqs = append(dmaMW.comp.State.PendingReqs, reqToBottom3)
		rqC.appendSubordinateID(reqToBottom3.ID)

		done := messaging.Msg{ID: dmaEngine.NewID(),
			Src:   memPort,
			Dst:   toMem.AsRemote(),
			RspTo: reqToBottom2.ID, Payload: memprotocol.WriteDoneRsp{}}

		toMem.Deliver(done)

		madeProgress := dmaMW.parseFromMem()

		Expect(madeProgress).To(BeTrue())
		Expect(dmaMW.comp.State.ProcessingReqs[0]).To(Equal(*rqC))
		Expect(dmaMW.comp.State.PendingReqs).To(HaveLen(2))
	})

	It("should send MemCopyH2D rsp to CP", func() {
		req := makeH2DReq(make([]byte, 128), 20)
		rqC := NewRequestCollection(req)
		if body, ok := req.Payload.(protocol.MemCopyD2HReq); ok {
			rqC.Data = make([]byte, body.ByteSize)
		}
		dmaMW.comp.State.ProcessingReqs = append(dmaMW.comp.State.ProcessingReqs, *rqC)
		rqC = &dmaMW.comp.State.ProcessingReqs[len(dmaMW.comp.State.ProcessingReqs)-1]

		reqToBottom2 := makeWriteReq(64)
		dmaMW.comp.State.PendingReqs = append(dmaMW.comp.State.PendingReqs, reqToBottom2)
		rqC.appendSubordinateID(reqToBottom2.ID)

		done := messaging.Msg{ID: dmaEngine.NewID(),
			Src:   memPort,
			Dst:   toMem.AsRemote(),
			RspTo: reqToBottom2.ID, Payload: memprotocol.WriteDoneRsp{}}

		toMem.Deliver(done)

		madeProgress := dmaMW.parseFromMem()

		Expect(madeProgress).To(BeTrue())
		Expect(dmaMW.comp.State.ProcessingReqs).To(BeEmpty())
		Expect(dmaMW.comp.State.PendingReqs).To(BeEmpty())

		rsp := dmaMW.comp.State.ToSendToCP[0]
		Expect(rsp.RspTo).To(Equal(req.ID))
		Expect(rsp.Dst).To(Equal(cpPort))
	})

	It("should copy data end to end", func() {
		req := makeH2DReq([]byte{1, 2, 3, 4}, 20)
		toCP.Deliver(req)

		for dmaEngine.Middlewares.DMA.Tick() {
		}

		write, ok := toMem.RetrieveOutgoing()
		Expect(ok).To(BeTrue())
		Expect(write.Payload.(memprotocol.WriteReq).Address).To(Equal(uint64(20)))
		Expect(write.Payload.(memprotocol.WriteReq).Data).To(Equal([]byte{1, 2, 3, 4}))

		done := messaging.Msg{ID: dmaEngine.NewID(),
			Src:   memPort,
			Dst:   toMem.AsRemote(),
			RspTo: write.ID, Payload: memprotocol.WriteDoneRsp{}}

		toMem.Deliver(done)

		for dmaEngine.Middlewares.DMA.Tick() {
		}

		rsp, ok := toCP.RetrieveOutgoing()
		Expect(ok).To(BeTrue())
		Expect(rsp.RspTo).To(Equal(req.ID))
	})
})
