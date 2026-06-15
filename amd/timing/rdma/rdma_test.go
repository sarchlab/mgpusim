package rdma

import (
	"log"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

func TestRDMA(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "RDMA")
}

// noopConn is a minimal messaging.Connection used to drive a component's
// real ports in isolation. Tests feed requests with Deliver and read
// responses with RetrieveOutgoing; the port still needs a connection so its
// send/retrieve notifications have somewhere to go.
type noopConn struct {
	hooking.HookableBase
}

func (c *noopConn) Name() string                     { return "NoopConn" }
func (c *noopConn) PlugIn(port messaging.Port)       { port.SetConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend()                      {}

var _ = Describe("RDMA Engine", func() {
	var (
		engine     timing.Engine
		rdmaEngine *Comp

		requestInside  messaging.Port
		requestOutside messaging.Port
		dataInside     messaging.Port
		dataOutside    messaging.Port
		ctrlPort       messaging.Port

		localCache messaging.RemotePort
		remoteGPU  messaging.RemotePort
		cpCtrl     messaging.RemotePort
	)

	makePort := func(name string, bufSize int) messaging.Port {
		port := messaging.NewPort(
			rdmaEngine, bufSize, bufSize, rdmaEngine.Name()+"."+name)
		rdmaEngine.AssignPort(name, port)

		conn := &noopConn{}
		conn.PlugIn(port)

		return port
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()

		localCache = messaging.RemotePort("LocalCache")
		remoteGPU = messaging.RemotePort("RemoteGPU")
		cpCtrl = messaging.RemotePort("CP.Ctrl")

		rdmaEngine = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithResources(Resources{
				LocalModules:           &mem.SinglePortMapper{Port: localCache},
				RemoteRDMAAddressTable: &mem.SinglePortMapper{Port: remoteGPU},
			}).
			Build("RDMAEngine")

		requestInside = makePort("RDMARequestInside", 4)
		requestOutside = makePort("RDMARequestOutside", 4)
		dataInside = makePort("RDMADataInside", 4)
		dataOutside = makePort("RDMADataOutside", 4)
		ctrlPort = makePort("Ctrl", 4)
	})

	makeReadReq := func(src, dst messaging.RemotePort) memprotocol.ReadReq {
		return memprotocol.ReadReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: src,
				Dst: dst,
			},
			Address:        0x100,
			AccessByteSize: 64,
		}
	}

	makeDataReadyRsp := func(
		src, dst messaging.RemotePort,
		rspTo uint64,
	) memprotocol.DataReadyRsp {
		return memprotocol.DataReadyRsp{
			MsgMeta: messaging.MsgMeta{
				ID:    timing.GetIDGenerator().Generate(),
				Src:   src,
				Dst:   dst,
				RspTo: rspTo,
			},
			Data: []byte{1, 2, 3, 4},
		}
	}

	// fillOutgoing exhausts the outgoing buffer of a port so that CanSend
	// returns false.
	fillOutgoing := func(port messaging.Port) {
		for port.CanSend() {
			port.Send(RestartRsp{
				MsgMeta: messaging.MsgMeta{
					ID:  timing.GetIDGenerator().Generate(),
					Src: port.AsRemote(),
					Dst: "Elsewhere",
				},
			})
		}
	}

	Context("read from inside", func() {
		It("should forward the read to outside", func() {
			read := makeReadReq(localCache, requestInside.AsRemote())
			requestInside.Deliver(read)

			madeProgress := rdmaEngine.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(rdmaEngine.State.TransactionsFromInside).To(HaveLen(1))
			Expect(rdmaEngine.State.TransactionsFromInside[0].OriginalReqID).
				To(Equal(read.ID))
			Expect(rdmaEngine.State.TransactionsFromInside[0].OriginalSrc).
				To(Equal(localCache))

			forwarded := requestOutside.RetrieveOutgoing()
			Expect(forwarded).To(BeAssignableToTypeOf(memprotocol.ReadReq{}))
			Expect(forwarded.Meta().Dst).To(Equal(remoteGPU))
			Expect(forwarded.Meta().ID).To(Equal(
				rdmaEngine.State.TransactionsFromInside[0].ForwardedReqID))
			Expect(forwarded.(memprotocol.ReadReq).Address).
				To(Equal(uint64(0x100)))
		})

		It("should wait if the outside port is busy", func() {
			fillOutgoing(requestOutside)

			read := makeReadReq(localCache, requestInside.AsRemote())
			requestInside.Deliver(read)

			rdmaEngine.Tick()

			Expect(rdmaEngine.State.TransactionsFromInside).To(HaveLen(0))
			Expect(requestInside.PeekIncoming()).NotTo(BeNil())
		})
	})

	Context("read from outside", func() {
		It("should forward the read to inside", func() {
			read := makeReadReq(remoteGPU, dataOutside.AsRemote())
			dataOutside.Deliver(read)

			madeProgress := rdmaEngine.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(rdmaEngine.State.TransactionsFromOutside).To(HaveLen(1))

			forwarded := dataInside.RetrieveOutgoing()
			Expect(forwarded).To(BeAssignableToTypeOf(memprotocol.ReadReq{}))
			Expect(forwarded.Meta().Dst).To(Equal(localCache))
		})

		It("should wait if the inside port is busy", func() {
			fillOutgoing(dataInside)

			read := makeReadReq(remoteGPU, dataOutside.AsRemote())
			dataOutside.Deliver(read)

			rdmaEngine.Tick()

			Expect(rdmaEngine.State.TransactionsFromOutside).To(HaveLen(0))
			Expect(dataOutside.PeekIncoming()).NotTo(BeNil())
		})
	})

	Context("data-ready from outside", func() {
		var trans transaction

		BeforeEach(func() {
			trans = transaction{
				OriginalReqID:  timing.GetIDGenerator().Generate(),
				OriginalSrc:    localCache,
				ForwardedReqID: timing.GetIDGenerator().Generate(),
			}
			rdmaEngine.State.TransactionsFromInside = append(
				rdmaEngine.State.TransactionsFromInside, trans)
		})

		It("should forward the response to inside", func() {
			rsp := makeDataReadyRsp(
				remoteGPU, requestOutside.AsRemote(), trans.ForwardedReqID)
			requestOutside.Deliver(rsp)

			madeProgress := rdmaEngine.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(rdmaEngine.State.TransactionsFromInside).To(HaveLen(0))

			forwarded := requestInside.RetrieveOutgoing()
			Expect(forwarded).
				To(BeAssignableToTypeOf(memprotocol.DataReadyRsp{}))
			Expect(forwarded.Meta().Dst).To(Equal(localCache))
			Expect(forwarded.Meta().RspTo).To(Equal(trans.OriginalReqID))
		})

		It("should wait if the inside port is busy", func() {
			fillOutgoing(requestInside)

			rsp := makeDataReadyRsp(
				remoteGPU, requestOutside.AsRemote(), trans.ForwardedReqID)
			requestOutside.Deliver(rsp)

			rdmaEngine.Tick()

			Expect(rdmaEngine.State.TransactionsFromInside).To(HaveLen(1))
			Expect(requestOutside.PeekIncoming()).NotTo(BeNil())
		})
	})

	Context("data-ready from inside", func() {
		var trans transaction

		BeforeEach(func() {
			trans = transaction{
				OriginalReqID:  timing.GetIDGenerator().Generate(),
				OriginalSrc:    remoteGPU,
				ForwardedReqID: timing.GetIDGenerator().Generate(),
			}
			rdmaEngine.State.TransactionsFromOutside = append(
				rdmaEngine.State.TransactionsFromOutside, trans)
		})

		It("should forward the response to outside", func() {
			rsp := makeDataReadyRsp(
				localCache, dataInside.AsRemote(), trans.ForwardedReqID)
			dataInside.Deliver(rsp)

			madeProgress := rdmaEngine.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(rdmaEngine.State.TransactionsFromOutside).To(HaveLen(0))

			forwarded := dataOutside.RetrieveOutgoing()
			Expect(forwarded).
				To(BeAssignableToTypeOf(memprotocol.DataReadyRsp{}))
			Expect(forwarded.Meta().Dst).To(Equal(remoteGPU))
			Expect(forwarded.Meta().RspTo).To(Equal(trans.OriginalReqID))
		})

		It("should wait if the outside port is busy", func() {
			fillOutgoing(dataOutside)

			rsp := makeDataReadyRsp(
				localCache, dataInside.AsRemote(), trans.ForwardedReqID)
			dataInside.Deliver(rsp)

			rdmaEngine.Tick()

			Expect(rdmaEngine.State.TransactionsFromOutside).To(HaveLen(1))
			Expect(dataInside.PeekIncoming()).NotTo(BeNil())
		})
	})

	Context("drain and restart", func() {
		makeDrainReq := func() DrainReq {
			return DrainReq{
				MsgMeta: messaging.MsgMeta{
					ID:  timing.GetIDGenerator().Generate(),
					Src: cpCtrl,
					Dst: ctrlPort.AsRemote(),
				},
			}
		}

		makeRestartReq := func() RestartReq {
			return RestartReq{
				MsgMeta: messaging.MsgMeta{
					ID:  timing.GetIDGenerator().Generate(),
					Src: cpCtrl,
					Dst: ctrlPort.AsRemote(),
				},
			}
		}

		It("should pause L1 requests and keep draining while busy", func() {
			trans := transaction{
				OriginalReqID:  timing.GetIDGenerator().Generate(),
				OriginalSrc:    localCache,
				ForwardedReqID: timing.GetIDGenerator().Generate(),
			}
			rdmaEngine.State.TransactionsFromInside = append(
				rdmaEngine.State.TransactionsFromInside, trans)

			drainReq := makeDrainReq()
			ctrlPort.Deliver(drainReq)

			rdmaEngine.Tick()

			Expect(rdmaEngine.State.IsDraining).To(BeTrue())
			Expect(rdmaEngine.State.PauseIncomingReqsFromL1).To(BeTrue())
			Expect(rdmaEngine.State.CurrentDrainReqID).To(Equal(drainReq.ID))
			Expect(rdmaEngine.State.CurrentDrainReqSrc).To(Equal(cpCtrl))
			Expect(ctrlPort.PeekOutgoing()).To(BeNil())
		})

		It("should not forward L1 requests while paused", func() {
			rdmaEngine.State.PauseIncomingReqsFromL1 = true

			read := makeReadReq(localCache, requestInside.AsRemote())
			requestInside.Deliver(read)

			rdmaEngine.Tick()

			Expect(rdmaEngine.State.TransactionsFromInside).To(HaveLen(0))
			Expect(requestOutside.PeekOutgoing()).To(BeNil())
		})

		It("should send a drain-complete response when fully drained", func() {
			drainReq := makeDrainReq()
			ctrlPort.Deliver(drainReq)

			rdmaEngine.Tick()

			Expect(rdmaEngine.State.IsDraining).To(BeFalse())
			Expect(rdmaEngine.State.PauseIncomingReqsFromL1).To(BeTrue())

			rsp := ctrlPort.RetrieveOutgoing()
			Expect(rsp).To(BeAssignableToTypeOf(DrainRsp{}))
			Expect(rsp.Meta().Dst).To(Equal(cpCtrl))
			Expect(rsp.Meta().RspTo).To(Equal(drainReq.ID))
		})

		It("should handle a restart request", func() {
			rdmaEngine.State.PauseIncomingReqsFromL1 = true
			rdmaEngine.State.CurrentDrainReqID =
				timing.GetIDGenerator().Generate()
			rdmaEngine.State.CurrentDrainReqSrc = cpCtrl

			restartReq := makeRestartReq()
			ctrlPort.Deliver(restartReq)

			rdmaEngine.Tick()

			Expect(rdmaEngine.State.PauseIncomingReqsFromL1).To(BeFalse())
			Expect(rdmaEngine.State.CurrentDrainReqID).To(Equal(uint64(0)))

			rsp := ctrlPort.RetrieveOutgoing()
			Expect(rsp).To(BeAssignableToTypeOf(RestartRsp{}))
			Expect(rsp.Meta().Dst).To(Equal(cpCtrl))
			Expect(rsp.Meta().RspTo).To(Equal(restartReq.ID))
		})
	})
})
