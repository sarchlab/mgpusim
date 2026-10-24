package rdma

import (
	"log"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
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
func (c *noopConn) BindPort(port messaging.Port)     { port.BindConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend(messaging.Port)        {}

var _ = Describe("RDMA Engine", func() {
	var (
		simulation *sim.Simulation
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
		port := twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: bufSize, OutgoingCapacity: bufSize}).
			Build()
		rdmaEngine.BindPort(name, port)

		conn := &noopConn{}
		conn.BindPort(port)

		return port
	}

	BeforeEach(func() {
		simulation = sim.MakeBuilder().Build()
		DeferCleanup(func() { Expect(simulation.Terminate()).To(Succeed()) })

		localCache = messaging.RemotePort("LocalCache")
		remoteGPU = messaging.RemotePort("RemoteGPU")
		cpCtrl = messaging.RemotePort("CP.Ctrl")

		rdmaEngine = MakeBuilder().
			WithSimulation(simulation).
			WithResources(Resources{
				LocalModules:           mem.AddressMapper{Destinations: []messaging.RemotePort{localCache}},
				RemoteRDMAAddressTable: mem.AddressMapper{Destinations: []messaging.RemotePort{remoteGPU}},
			}).
			Build("RDMAEngine")

		requestInside = makePort("RDMARequestInside", 4)
		requestOutside = makePort("RDMARequestOutside", 4)
		dataInside = makePort("RDMADataInside", 4)
		dataOutside = makePort("RDMADataOutside", 4)
		ctrlPort = makePort("Ctrl", 4)
		Expect(simulation.Initialize()).To(Succeed())
	})

	makeReadReq := func(src, dst messaging.RemotePort) messaging.Msg {
		return messaging.Msg{ID: rdmaEngine.NewID(),
			Src: src,
			Dst: dst, Payload: memprotocol.ReadReq{Address: 0x100,
				AccessByteSize: 64}}

	}

	makeDataReadyRsp := func(
		src, dst messaging.RemotePort,
		rspTo uint64,
	) messaging.Msg {
		return messaging.Msg{ID: rdmaEngine.NewID(),
			Src:   src,
			Dst:   dst,
			RspTo: rspTo, Payload: memprotocol.DataReadyRsp{Data: []byte{1, 2, 3, 4}}}

	}

	// fillOutgoing exhausts the outgoing buffer of a port so that CanSend
	// returns false.
	fillOutgoing := func(port messaging.Port) {
		for port.CanSend() {
			port.Send(messaging.Msg{ID: rdmaEngine.NewID(),
				Src: port.AsRemote(),
				Dst: "Elsewhere", Payload: RestartRsp{}},
			)
		}
	}

	Context("read from inside", func() {
		It("should forward the read to outside", func() {
			read := makeReadReq(localCache, requestInside.AsRemote())
			requestInside.Deliver(read)

			madeProgress := tickRDMA(rdmaEngine)

			Expect(madeProgress).To(BeTrue())
			Expect(rdmaEngine.State.TransactionsFromInside).To(HaveLen(1))
			Expect(rdmaEngine.State.TransactionsFromInside[0].OriginalReqID).
				To(Equal(read.ID))
			Expect(rdmaEngine.State.TransactionsFromInside[0].OriginalSrc).
				To(Equal(localCache))

			forwarded, _ := requestOutside.RetrieveOutgoing()
			Expect(forwarded.Payload).To(BeAssignableToTypeOf(memprotocol.ReadReq{}))
			Expect(forwarded.Dst).To(Equal(remoteGPU))
			Expect(forwarded.ID).To(Equal(
				rdmaEngine.State.TransactionsFromInside[0].ForwardedReqID))
			Expect(forwarded.Payload.(memprotocol.ReadReq).Address).
				To(Equal(uint64(0x100)))
		})

		It("should wait if the outside port is busy", func() {
			fillOutgoing(requestOutside)

			read := makeReadReq(localCache, requestInside.AsRemote())
			requestInside.Deliver(read)

			tickRDMA(rdmaEngine)

			Expect(rdmaEngine.State.TransactionsFromInside).To(HaveLen(0))
			Expect(hasIncoming(requestInside)).To(BeTrue())
		})
	})

	Context("read from outside", func() {
		It("should forward the read to inside", func() {
			read := makeReadReq(remoteGPU, dataOutside.AsRemote())
			dataOutside.Deliver(read)

			madeProgress := tickRDMA(rdmaEngine)

			Expect(madeProgress).To(BeTrue())
			Expect(rdmaEngine.State.TransactionsFromOutside).To(HaveLen(1))

			forwarded, _ := dataInside.RetrieveOutgoing()
			Expect(forwarded.Payload).To(BeAssignableToTypeOf(memprotocol.ReadReq{}))
			Expect(forwarded.Dst).To(Equal(localCache))
		})

		It("should wait if the inside port is busy", func() {
			fillOutgoing(dataInside)

			read := makeReadReq(remoteGPU, dataOutside.AsRemote())
			dataOutside.Deliver(read)

			tickRDMA(rdmaEngine)

			Expect(rdmaEngine.State.TransactionsFromOutside).To(HaveLen(0))
			Expect(hasIncoming(dataOutside)).To(BeTrue())
		})
	})

	Context("data-ready from outside", func() {
		var trans transaction

		BeforeEach(func() {
			trans = transaction{
				OriginalReqID:  rdmaEngine.NewID(),
				OriginalSrc:    localCache,
				ForwardedReqID: rdmaEngine.NewID(),
			}
			rdmaEngine.State.TransactionsFromInside = append(
				rdmaEngine.State.TransactionsFromInside, trans)
		})

		It("should forward the response to inside", func() {
			rsp := makeDataReadyRsp(
				remoteGPU, requestOutside.AsRemote(), trans.ForwardedReqID)
			requestOutside.Deliver(rsp)

			madeProgress := tickRDMA(rdmaEngine)

			Expect(madeProgress).To(BeTrue())
			Expect(rdmaEngine.State.TransactionsFromInside).To(HaveLen(0))

			forwarded, _ := requestInside.RetrieveOutgoing()
			Expect(forwarded).
				To(BeAssignableToTypeOf(messaging.Msg{Payload: memprotocol.DataReadyRsp{}}))
			Expect(forwarded.Dst).To(Equal(localCache))
			Expect(forwarded.RspTo).To(Equal(trans.OriginalReqID))
		})

		It("should wait if the inside port is busy", func() {
			fillOutgoing(requestInside)

			rsp := makeDataReadyRsp(
				remoteGPU, requestOutside.AsRemote(), trans.ForwardedReqID)
			requestOutside.Deliver(rsp)

			tickRDMA(rdmaEngine)

			Expect(rdmaEngine.State.TransactionsFromInside).To(HaveLen(1))
			Expect(hasIncoming(requestOutside)).To(BeTrue())
		})
	})

	Context("data-ready from inside", func() {
		var trans transaction

		BeforeEach(func() {
			trans = transaction{
				OriginalReqID:  rdmaEngine.NewID(),
				OriginalSrc:    remoteGPU,
				ForwardedReqID: rdmaEngine.NewID(),
			}
			rdmaEngine.State.TransactionsFromOutside = append(
				rdmaEngine.State.TransactionsFromOutside, trans)
		})

		It("should forward the response to outside", func() {
			rsp := makeDataReadyRsp(
				localCache, dataInside.AsRemote(), trans.ForwardedReqID)
			dataInside.Deliver(rsp)

			madeProgress := tickRDMA(rdmaEngine)

			Expect(madeProgress).To(BeTrue())
			Expect(rdmaEngine.State.TransactionsFromOutside).To(HaveLen(0))

			forwarded, _ := dataOutside.RetrieveOutgoing()
			Expect(forwarded).
				To(BeAssignableToTypeOf(messaging.Msg{Payload: memprotocol.DataReadyRsp{}}))
			Expect(forwarded.Dst).To(Equal(remoteGPU))
			Expect(forwarded.RspTo).To(Equal(trans.OriginalReqID))
		})

		It("should wait if the outside port is busy", func() {
			fillOutgoing(dataOutside)

			rsp := makeDataReadyRsp(
				localCache, dataInside.AsRemote(), trans.ForwardedReqID)
			dataInside.Deliver(rsp)

			tickRDMA(rdmaEngine)

			Expect(rdmaEngine.State.TransactionsFromOutside).To(HaveLen(1))
			Expect(hasIncoming(dataInside)).To(BeTrue())
		})
	})

	Context("drain and restart", func() {
		makeDrainReq := func() messaging.Msg {
			return messaging.Msg{ID: rdmaEngine.NewID(),
				Src: cpCtrl,
				Dst: ctrlPort.AsRemote(), Payload: DrainReq{}}

		}

		makeRestartReq := func() messaging.Msg {
			return messaging.Msg{ID: rdmaEngine.NewID(),
				Src: cpCtrl,
				Dst: ctrlPort.AsRemote(), Payload: RestartReq{}}

		}

		It("should pause L1 requests and keep draining while busy", func() {
			trans := transaction{
				OriginalReqID:  rdmaEngine.NewID(),
				OriginalSrc:    localCache,
				ForwardedReqID: rdmaEngine.NewID(),
			}
			rdmaEngine.State.TransactionsFromInside = append(
				rdmaEngine.State.TransactionsFromInside, trans)

			drainReq := makeDrainReq()
			ctrlPort.Deliver(drainReq)

			tickRDMA(rdmaEngine)

			Expect(rdmaEngine.State.IsDraining).To(BeTrue())
			Expect(rdmaEngine.State.PauseIncomingReqsFromL1).To(BeTrue())
			Expect(rdmaEngine.State.CurrentDrainReqID).To(Equal(drainReq.ID))
			Expect(rdmaEngine.State.CurrentDrainReqSrc).To(Equal(cpCtrl))
			Expect(hasOutgoing(ctrlPort)).To(BeFalse())
		})

		It("should not forward L1 requests while paused", func() {
			rdmaEngine.State.PauseIncomingReqsFromL1 = true

			read := makeReadReq(localCache, requestInside.AsRemote())
			requestInside.Deliver(read)

			tickRDMA(rdmaEngine)

			Expect(rdmaEngine.State.TransactionsFromInside).To(HaveLen(0))
			Expect(hasOutgoing(requestOutside)).To(BeFalse())
		})

		It("should send a drain-complete response when fully drained", func() {
			drainReq := makeDrainReq()
			ctrlPort.Deliver(drainReq)

			tickRDMA(rdmaEngine)

			Expect(rdmaEngine.State.IsDraining).To(BeFalse())
			Expect(rdmaEngine.State.PauseIncomingReqsFromL1).To(BeTrue())

			rsp, _ := ctrlPort.RetrieveOutgoing()
			Expect(rsp.Payload).To(BeAssignableToTypeOf(DrainRsp{}))
			Expect(rsp.Dst).To(Equal(cpCtrl))
			Expect(rsp.RspTo).To(Equal(drainReq.ID))
		})

		It("should handle a restart request", func() {
			rdmaEngine.State.PauseIncomingReqsFromL1 = true
			rdmaEngine.State.CurrentDrainReqID =
				rdmaEngine.NewID()
			rdmaEngine.State.CurrentDrainReqSrc = cpCtrl

			restartReq := makeRestartReq()
			ctrlPort.Deliver(restartReq)

			tickRDMA(rdmaEngine)

			Expect(rdmaEngine.State.PauseIncomingReqsFromL1).To(BeFalse())
			Expect(rdmaEngine.State.CurrentDrainReqID).To(Equal(uint64(0)))

			rsp, _ := ctrlPort.RetrieveOutgoing()
			Expect(rsp.Payload).To(BeAssignableToTypeOf(RestartRsp{}))
			Expect(rsp.Dst).To(Equal(cpCtrl))
			Expect(rsp.RspTo).To(Equal(restartReq.ID))
		})
	})
})

func tickRDMA(c *Comp) bool {
	ctrl := c.Middlewares.Ctrl.Tick()
	data := c.Middlewares.Forward.Tick()
	return ctrl || data
}
func hasIncoming(p messaging.Port) bool { _, ok := p.PeekIncoming(); return ok }
func hasOutgoing(p messaging.Port) bool { _, ok := p.PeekOutgoing(); return ok }
