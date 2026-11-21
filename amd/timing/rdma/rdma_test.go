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
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
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
		accessCounter  messaging.Port

		migrationOutside messaging.Port
		migrationInside  messaging.Port

		localCache  messaging.RemotePort
		localMemory messaging.RemotePort
		remoteGPU   messaging.RemotePort
		cpCtrl      messaging.RemotePort
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
		localMemory = messaging.RemotePort("LocalMemory")
		remoteGPU = messaging.RemotePort("RemoteGPU")
		cpCtrl = messaging.RemotePort("CP.Ctrl")

		rdmaEngine = MakeBuilder().
			WithSimulation(simulation).
			WithSpec(func() Spec {
				spec := DefaultSpec()
				spec.LocalModules = mem.AddressMapper{Destinations: []messaging.RemotePort{localCache}}
				spec.RemoteRDMAAddressTable = mem.AddressMapper{Destinations: []messaging.RemotePort{remoteGPU}}
				spec.MigrationModules = mem.AddressMapper{Destinations: []messaging.RemotePort{localMemory}}
				return spec
			}()).
			Build("RDMAEngine")

		requestInside = makePort("RDMARequestInside", 4)
		requestOutside = makePort("RDMARequestOutside", 4)
		dataInside = makePort("RDMADataInside", 4)
		dataOutside = makePort("RDMADataOutside", 4)
		ctrlPort = makePort("Ctrl", 4)
		accessCounter = makePort("AccessCounter", 4)
		migrationOutside = makePort("MigrationOutside", 4)
		migrationInside = makePort("MigrationInside", 4)
		Expect(simulation.Initialize()).To(Succeed())
	})

	It("reports the unsupported control payload type", func() {
		msg := messaging.Msg{ID: rdmaEngine.NewID(), Payload: memprotocol.ReadReq{}}
		ctrlPort.Deliver(msg)
		Expect(func() { rdmaEngine.Middlewares.Ctrl.processFromCtrlPort() }).
			To(PanicWith("cannot process request of type memprotocol.ReadReq"))
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

	Context("access counters", func() {
		readsFromInsideAt := func(addr uint64, n int) {
			for i := 0; i < n; i++ {
				read := makeReadReq(localCache, requestInside.AsRemote())
				read.Payload = memprotocol.ReadReq{Address: addr, AccessByteSize: 64}
				requestInside.Deliver(read)
				tickRDMA(rdmaEngine)
				requestOutside.RetrieveOutgoing()
			}
			tickRDMA(rdmaEngine)
		}

		readsFromInside := func(n int) { readsFromInsideAt(0x100, n) }

		It("should count nothing without a handler", func() {
			readsFromInside(300)

			Expect(rdmaEngine.State.AccessCounts).To(BeEmpty())
			Expect(hasOutgoing(accessCounter)).To(BeFalse())
		})

		It("should notify the handler at the threshold and clear the counter", func() {
			rdmaEngine.Spec.AccessCounterHandler = "Driver.MigrationNotification"
			rdmaEngine.Spec.AccessCounterThreshold = 2
			rdmaEngine.Spec.DeviceID = 3

			readsFromInside(1)
			Expect(hasOutgoing(accessCounter)).To(BeFalse())

			readsFromInside(1)
			msg, ok := accessCounter.RetrieveOutgoing()
			Expect(ok).To(BeTrue())
			Expect(msg.Dst).To(Equal(messaging.RemotePort("Driver.MigrationNotification")))
			Expect(msg.Payload).To(Equal(protocol.AccessCounterNotification{
				PAddr: 0, DeviceID: 3,
			}))
			Expect(rdmaEngine.State.AccessCounts).To(BeEmpty())
			Expect(hasOutgoing(accessCounter)).To(BeFalse())
		})

		It("should count per physical page frame of the system page size", func() {
			rdmaEngine.Spec.AccessCounterHandler = "Driver.MigrationNotification"
			rdmaEngine.Spec.AccessCounterThreshold = 3
			rdmaEngine.Spec.Log2PageSize = 21

			// Two accesses to each of two 2 MiB frames reach no threshold,
			// although four accesses hit the same 4 KiB-aligned offsets.
			readsFromInsideAt(0x200040, 2)
			readsFromInsideAt(0x400040, 2)
			Expect(hasOutgoing(accessCounter)).To(BeFalse())

			// A third access anywhere in the first frame does.
			readsFromInsideAt(0x3ff000, 1)
			msg, ok := accessCounter.RetrieveOutgoing()
			Expect(ok).To(BeTrue())
			Expect(msg.Payload).To(Equal(protocol.AccessCounterNotification{
				PAddr: 0x200000,
			}))
			Expect(rdmaEngine.State.AccessCounts).To(Equal(map[uint64]uint64{2: 2}))
		})

		It("should keep the count of a frame that is freed and used again", func() {
			rdmaEngine.Spec.AccessCounterHandler = "Driver.MigrationNotification"
			rdmaEngine.Spec.AccessCounterThreshold = 3

			// The engine counts frames and knows nothing of pages: whatever
			// page gets the frame next inherits the two accesses, and its
			// first access notifies.
			readsFromInsideAt(0x5000, 2)
			Expect(rdmaEngine.State.AccessCounts).To(Equal(map[uint64]uint64{5: 2}))

			readsFromInsideAt(0x5fc0, 1)
			Expect(hasOutgoing(accessCounter)).To(BeTrue())
		})
	})

	Context("page copies of other GPUs", func() {
		It("should forward a read to the memory below the L2", func() {
			read := makeReadReq("GPU[2].DataMover.Outside", migrationOutside.AsRemote())
			migrationOutside.Deliver(read)

			Expect(tickRDMA(rdmaEngine)).To(BeTrue())
			Expect(rdmaEngine.State.TransactionsFromMigration).To(HaveLen(1))
			Expect(rdmaEngine.State.TransactionsFromOutside).To(BeEmpty())
			Expect(hasOutgoing(dataInside)).To(BeFalse())

			forwarded, _ := migrationInside.RetrieveOutgoing()
			Expect(forwarded.Payload).To(BeAssignableToTypeOf(memprotocol.ReadReq{}))
			Expect(forwarded.Dst).To(Equal(localMemory))

			rsp := makeDataReadyRsp(localMemory, migrationInside.AsRemote(), forwarded.ID)
			migrationInside.Deliver(rsp)

			Expect(tickRDMA(rdmaEngine)).To(BeTrue())
			Expect(rdmaEngine.State.TransactionsFromMigration).To(BeEmpty())

			returned, _ := migrationOutside.RetrieveOutgoing()
			Expect(returned.Dst).To(Equal(
				messaging.RemotePort("GPU[2].DataMover.Outside")))
			Expect(returned.RspTo).To(Equal(read.ID))
		})

		It("should not count a page copy as a remote access", func() {
			rdmaEngine.Spec.AccessCounterHandler = "Driver.MigrationNotification"
			rdmaEngine.Spec.AccessCounterThreshold = 1

			migrationOutside.Deliver(
				makeReadReq("GPU[2].DataMover.Outside", migrationOutside.AsRemote()))
			tickRDMA(rdmaEngine)

			Expect(rdmaEngine.State.AccessCounts).To(BeEmpty())
			Expect(hasOutgoing(accessCounter)).To(BeFalse())
		})

		It("should wait if the port to the memory is busy", func() {
			fillOutgoing(migrationInside)

			migrationOutside.Deliver(
				makeReadReq("GPU[2].DataMover.Outside", migrationOutside.AsRemote()))
			tickRDMA(rdmaEngine)

			Expect(rdmaEngine.State.TransactionsFromMigration).To(BeEmpty())
			Expect(hasIncoming(migrationOutside)).To(BeTrue())
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
			Expect(forwarded.Payload).To(BeAssignableToTypeOf(memprotocol.DataReadyRsp{}))
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
			Expect(forwarded.Payload).To(BeAssignableToTypeOf(memprotocol.DataReadyRsp{}))
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
