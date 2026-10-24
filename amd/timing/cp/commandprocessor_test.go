package cp

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/dispatching"
	"github.com/sarchlab/mgpusim/v5/amd/timing/rdma"
	"go.uber.org/mock/gomock"
)

// noopConn is a minimal messaging.Connection used to drive the Command
// Processor's real ports in isolation. Tests feed requests with Deliver and
// read responses with RetrieveOutgoing; the port still needs a connection so
// its send/retrieve notifications have somewhere to go.
type noopConn struct {
	hooking.HookableBase
}

func (c *noopConn) Name() string                     { return "NoopConn" }
func (c *noopConn) BindPort(port messaging.Port)     { port.BindConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend(messaging.Port)        {}

var _ = Describe("CommandProcessor", func() {
	var (
		mockCtrl   *gomock.Controller
		reg        *sim.Simulation
		cp         *Comp
		dispatcher *MockDispatcher

		toDriver messaging.Port
		toDMA    messaging.Port
		toCUs    messaging.Port
		toTLBs   messaging.Port
		toATs    messaging.Port
		toCaches messaging.Port
		toRDMA   messaging.Port
	)

	const driverPort = messaging.RemotePort("Driver.ToGPU")
	const dmaPort = messaging.RemotePort("DMA.ToCP")
	const rdmaPort = messaging.RemotePort("RDMA.Ctrl")

	remoteList := func(prefix string, n int) []messaging.RemotePort {
		ports := make([]messaging.RemotePort, n)
		for i := range ports {
			ports[i] = messaging.RemotePort(fmt.Sprintf("%s%d", prefix, i))
		}
		return ports
	}

	tickUntilQuiet := func() {
		for tickCP(cp) {
		}
	}

	collectCtrlReqs := func(port messaging.Port) []messaging.Msg {
		var reqs []messaging.Msg
		for {
			msg, ok := port.RetrieveOutgoing()
			if !ok {
				break
			}
			reqs = append(reqs, msg)
		}
		return reqs
	}

	ackCtrlReqs := func(
		port messaging.Port,
		reqs []messaging.Msg,
	) {
		for _, req := range reqs {
			rsp := messaging.Msg{Payload: memcontrolprotocol.Rsp{Command: req.Payload.(memcontrolprotocol.Req).Command,
				Success: true}}

			rsp.ID = cp.NewID()
			rsp.Src = req.Dst
			rsp.Dst = req.Src
			rsp.RspTo = req.ID
			port.Deliver(rsp)
		}
		tickUntilQuiet()
	}

	// expectCtrlStep drains the port's outgoing buffer, asserts that it holds
	// one request per destination with the given command, and acknowledges
	// all of them.
	expectCtrlStep := func(
		port messaging.Port,
		cmd memcontrolprotocol.Command,
		dsts []messaging.RemotePort,
	) {
		reqs := collectCtrlReqs(port)

		Expect(reqs).To(HaveLen(len(dsts)))
		seen := map[messaging.RemotePort]memcontrolprotocol.Command{}
		for _, req := range reqs {
			seen[req.Dst] = req.Payload.(memcontrolprotocol.Req).Command
		}
		for _, dst := range dsts {
			Expect(seen).To(HaveKeyWithValue(dst, cmd))
		}

		ackCtrlReqs(port, reqs)
	}

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		reg = sim.MakeBuilder().WithTaskTracking().Build()
		DeferCleanup(func() { Expect(reg.Terminate()).To(Succeed()) })

		cp = MakeBuilder().
			WithSimulation(reg).
			WithDriver(driverPort).
			Build("CP")

		assign := func(name string) messaging.Port {
			p := twowaybuffered.MakeBuilder().
				WithSimulation(reg).
				WithSpec(twowaybuffered.Spec{IncomingCapacity: 64, OutgoingCapacity: 64}).
				Build()
			cp.BindPort(name, p)
			(&noopConn{}).BindPort(p)
			return p
		}

		toDriver = assign("ToDriver")
		toDMA = assign("ToDMA")
		toCUs = assign("ToCUs")
		toTLBs = assign("ToTLBs")
		toATs = assign("ToAddressTranslators")
		toCaches = assign("ToCaches")
		toRDMA = assign("ToRDMA")

		Expect(reg.Initialize()).To(Succeed())
		cp.State.DMAEngine = dmaPort
		cp.State.RDMA = rdmaPort
		cp.State.CUs = remoteList("CU", 10)
		cp.State.TLBs = remoteList("TLB", 10)
		cp.State.AddressTranslators = remoteList("AT", 10)
		cp.State.ROBs = remoteList("ROB", 10)
		cp.State.L1ICaches = remoteList("L1I", 2)
		cp.State.L1SCaches = remoteList("L1S", 2)
		cp.State.L1VCaches = remoteList("L1V", 2)
		cp.State.L2Caches = remoteList("L2", 2)

		dispatcher = NewMockDispatcher(mockCtrl)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	useMockDispatcher := func() {
		cp.Resources.Dispatchers = []dispatching.Dispatcher{dispatcher}
		cp.Middlewares.Data.dispatchers = cp.Resources.Dispatchers
		dispatcher.EXPECT().Tick().Return(false).AnyTimes()
	}

	l1Dsts := func() []messaging.RemotePort {
		var dsts []messaging.RemotePort
		dsts = append(dsts, cp.State.L1ICaches...)
		dsts = append(dsts, cp.State.L1SCaches...)
		dsts = append(dsts, cp.State.L1VCaches...)
		return dsts
	}

	allCacheDsts := func() []messaging.RemotePort {
		return append(l1Dsts(), cp.State.L2Caches...)
	}

	atAndROBDsts := func() []messaging.RemotePort {
		return append(append([]messaging.RemotePort{},
			cp.State.AddressTranslators...), cp.State.ROBs...)
	}

	It("should forward kernel launching request to a Dispatcher", func() {
		useMockDispatcher()

		req := messaging.Msg{ID: cp.NewID(),
			Src: driverPort,
			Dst: toDriver.AsRemote(), Payload: protocol.LaunchKernelReq{}}

		toDriver.Deliver(req)

		dispatcher.EXPECT().IsDispatching().Return(false).AnyTimes()

		tickUntilQuiet()

		// The L1 data caches are invalidated before the kernel starts, and
		// the request waits in the buffer meanwhile.
		l1DataDsts := append(append([]messaging.RemotePort{},
			cp.State.L1SCaches...), cp.State.L1VCaches...)
		expectCtrlStep(toCaches, memcontrolprotocol.CmdDrain, l1DataDsts)
		expectCtrlStep(toCaches, memcontrolprotocol.CmdInvalidate, l1DataDsts)
		Expect(toDriver.IncomingBuf().Size()).To(BeNumerically(">", 0))

		dispatcher.EXPECT().StartDispatching(gomock.Any()).
			Do(func(launched messaging.Msg) {
				Expect(launched.ID).To(Equal(req.ID))
			})

		expectCtrlStep(toCaches, memcontrolprotocol.CmdEnable, l1DataDsts)

		Expect(toDriver.IncomingBuf().Size()).To(BeZero())
		Expect(cp.State.CtrlSeq).To(Equal(ctrlSeqNone))
		Expect(cp.State.KernelStartReqID).To(BeZero())
	})

	It("should dispatch a kernel immediately when there is no cache", func() {
		useMockDispatcher()
		cp.State.L1ICaches = nil
		cp.State.L1SCaches = nil
		cp.State.L1VCaches = nil
		cp.State.L2Caches = nil

		req := messaging.Msg{ID: cp.NewID(),
			Src: driverPort,
			Dst: toDriver.AsRemote(), Payload: protocol.LaunchKernelReq{}}

		toDriver.Deliver(req)

		dispatcher.EXPECT().IsDispatching().Return(false)
		dispatcher.EXPECT().StartDispatching(gomock.Any())

		madeProgress := tickCP(cp)

		Expect(madeProgress).To(BeTrue())
		Expect(toDriver.IncomingBuf().Size()).To(BeZero())
	})

	It("should wait if there is no dispatcher available", func() {
		useMockDispatcher()

		req := messaging.Msg{ID: cp.NewID(),
			Src: driverPort,
			Dst: toDriver.AsRemote(), Payload: protocol.LaunchKernelReq{}}

		toDriver.Deliver(req)

		dispatcher.EXPECT().IsDispatching().Return(true).AnyTimes()

		madeProgress := tickCP(cp)

		Expect(madeProgress).To(BeFalse())
		Expect(toDriver.IncomingBuf().Size()).To(BeNumerically(">", 0))
	})

	It("should forward memory copies to the DMA engine and respond "+
		"to the driver", func() {
		req := messaging.Msg{ID: cp.NewID(),
			Src: driverPort,
			Dst: toDriver.AsRemote(), Payload: protocol.MemCopyH2DReq{SrcBuffer: make([]byte, 128),
				DstAddress: 0x1000}}

		toDriver.Deliver(req)

		tickUntilQuiet()

		cloned, ok := toDMA.RetrieveOutgoing()
		Expect(ok).To(BeTrue())
		Expect(cloned.Dst).To(Equal(dmaPort))
		Expect(
			cloned.Payload.(protocol.MemCopyH2DReq).DstAddress,
		).To(Equal(req.Payload.(protocol.MemCopyH2DReq).DstAddress))
		Expect(cloned.ID).NotTo(Equal(req.ID))
		Expect(cp.State.BottomMemCopyH2DToTop).To(HaveKey(cloned.ID))

		dmaRsp := messaging.Msg{ID: cp.NewID(),
			Src:   dmaPort,
			Dst:   toDMA.AsRemote(),
			RspTo: cloned.ID, Payload: protocol.GeneralRsp{}}

		toDMA.Deliver(dmaRsp)

		tickUntilQuiet()

		rsp, ok := toDriver.RetrieveOutgoing()
		Expect(ok).To(BeTrue())
		Expect(rsp.RspTo).To(Equal(req.ID))
		Expect(rsp.Dst).To(Equal(driverPort))
		Expect(cp.State.BottomMemCopyH2DToTop).To(BeEmpty())
	})

	It("should handle a driver flush request", func() {
		req := messaging.Msg{ID: cp.NewID(),
			Src: driverPort,
			Dst: toDriver.AsRemote(), Payload: protocol.FlushReq{}}

		toDriver.Deliver(req)

		tickUntilQuiet()

		// Drain -> Flush -> Invalidate the L1 caches, then the L2 caches, then
		// re-enable. The Invalidate matches v4's flush, which unconditionally
		// reset the cache directory (dropping all lines) on every flush.
		expectCtrlStep(toCaches, memcontrolprotocol.CmdDrain, l1Dsts())
		expectCtrlStep(toCaches, memcontrolprotocol.CmdFlush, l1Dsts())
		expectCtrlStep(toCaches, memcontrolprotocol.CmdInvalidate, l1Dsts())
		expectCtrlStep(toCaches, memcontrolprotocol.CmdDrain,
			cp.State.L2Caches)
		expectCtrlStep(toCaches, memcontrolprotocol.CmdFlush,
			cp.State.L2Caches)
		expectCtrlStep(toCaches, memcontrolprotocol.CmdInvalidate,
			cp.State.L2Caches)
		expectCtrlStep(toCaches, memcontrolprotocol.CmdEnable, allCacheDsts())

		rsp, ok := toDriver.RetrieveOutgoing()
		Expect(ok).To(BeTrue())
		Expect(rsp.RspTo).To(Equal(req.ID))
		Expect(rsp.Dst).To(Equal(driverPort))
		Expect(cp.State.CtrlSeq).To(Equal(ctrlSeqNone))
	})

	It("should respond to a flush request immediately when there is no "+
		"cache", func() {
		cp.State.L1ICaches = nil
		cp.State.L1SCaches = nil
		cp.State.L1VCaches = nil
		cp.State.L2Caches = nil

		req := messaging.Msg{ID: cp.NewID(),
			Src: driverPort,
			Dst: toDriver.AsRemote(), Payload: protocol.FlushReq{}}

		toDriver.Deliver(req)

		tickUntilQuiet()

		rsp, ok := toDriver.RetrieveOutgoing()
		Expect(ok).To(BeTrue())
		Expect(rsp.RspTo).To(Equal(req.ID))
	})

	It("should answer back-to-back flush requests in order", func() {
		var reqs []messaging.Msg
		for i := 0; i < 2; i++ {
			req := messaging.Msg{ID: cp.NewID(),
				Src: driverPort,
				Dst: toDriver.AsRemote(), Payload: protocol.FlushReq{}}

			toDriver.Deliver(req)
			reqs = append(reqs, req)
		}

		tickUntilQuiet()

		// The first flush finishes when the last Enable ack arrives, which
		// queues its response. The second flush is taken from the buffer on
		// a later tick and must not overwrite the first request before that
		// response has been sent.
		for _, req := range reqs {
			expectCtrlStep(toCaches, memcontrolprotocol.CmdDrain, l1Dsts())
			expectCtrlStep(toCaches, memcontrolprotocol.CmdFlush, l1Dsts())
			expectCtrlStep(toCaches, memcontrolprotocol.CmdInvalidate,
				l1Dsts())
			expectCtrlStep(toCaches, memcontrolprotocol.CmdDrain,
				cp.State.L2Caches)
			expectCtrlStep(toCaches, memcontrolprotocol.CmdFlush,
				cp.State.L2Caches)
			expectCtrlStep(toCaches, memcontrolprotocol.CmdInvalidate,
				cp.State.L2Caches)
			expectCtrlStep(toCaches, memcontrolprotocol.CmdEnable,
				allCacheDsts())

			rsp, ok := toDriver.RetrieveOutgoing()
			Expect(ok).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(req.ID))
			Expect(rsp.Dst).To(Equal(driverPort))
		}
		Expect(toDriver.OutgoingBuf().Size()).To(BeZero())
	})

	It("should handle a shootdown command", func() {
		cmd := messaging.Msg{ID: cp.NewID(),
			Src: driverPort,
			Dst: toDriver.AsRemote(), Payload: protocol.ShootDownCommand{VAddr: []uint64{0x1000},
				PID: 1}}

		toDriver.Deliver(cmd)

		tickUntilQuiet()

		Expect(cp.State.ShootDownInProcess).To(BeTrue())

		// Step 1: flush the CU pipelines.
		var cuFlushReqs []messaging.Msg
		for {
			msg, ok := toCUs.RetrieveOutgoing()
			if !ok {
				break
			}
			cuFlushReqs = append(
				cuFlushReqs, msg)
		}
		Expect(cuFlushReqs).To(HaveLen(10))
		Expect(cp.State.PendingAcks).To(Equal(uint64(10)))

		for range cuFlushReqs {
			rsp := messaging.Msg{ID: cp.NewID(),
				Src: "CU",
				Dst: toCUs.AsRemote(), Payload: protocol.CUPipelineFlushRsp{}}

			toCUs.Deliver(rsp)
		}
		tickUntilQuiet()

		// Step 2: pause the address translators and the ROBs.
		expectCtrlStep(toATs, memcontrolprotocol.CmdPause, atAndROBDsts())

		// Step 3: reset the address translators.
		expectCtrlStep(toATs, memcontrolprotocol.CmdReset,
			cp.State.AddressTranslators)

		// Steps 4-9: drain + flush + invalidate the caches, L1 first.
		expectCtrlStep(toCaches, memcontrolprotocol.CmdDrain, l1Dsts())
		expectCtrlStep(toCaches, memcontrolprotocol.CmdFlush, l1Dsts())
		expectCtrlStep(toCaches, memcontrolprotocol.CmdInvalidate, l1Dsts())
		expectCtrlStep(toCaches, memcontrolprotocol.CmdDrain,
			cp.State.L2Caches)
		expectCtrlStep(toCaches, memcontrolprotocol.CmdFlush,
			cp.State.L2Caches)
		expectCtrlStep(toCaches, memcontrolprotocol.CmdInvalidate,
			cp.State.L2Caches)

		// Step 10: pause the TLBs.
		expectCtrlStep(toTLBs, memcontrolprotocol.CmdPause, cp.State.TLBs)

		// Step 11: invalidate the TLBs with the shootdown filter.
		tlbReqs := collectCtrlReqs(toTLBs)
		Expect(tlbReqs).To(HaveLen(10))
		for _, req := range tlbReqs {
			Expect(req.Payload.(memcontrolprotocol.Req).Command).To(Equal(memcontrolprotocol.CmdInvalidate))
			Expect(req.Payload.(memcontrolprotocol.Req).Addresses).To(Equal([]uint64{0x1000}))
			Expect(req.Payload.(memcontrolprotocol.Req).PID).To(Equal(cmd.Payload.(protocol.ShootDownCommand).PID))
		}
		ackCtrlReqs(toTLBs, tlbReqs)

		// Finally, the CP responds to the driver.
		rsp, ok := toDriver.RetrieveOutgoing()
		Expect(ok).To(BeTrue())
		Expect(rsp.Payload).To(BeAssignableToTypeOf(protocol.ShootDownCompleteRsp{}))
		Expect(rsp.Dst).To(Equal(driverPort))
		Expect(cp.State.ShootDownInProcess).To(BeFalse())
		Expect(cp.State.CtrlSeq).To(Equal(ctrlSeqNone))
	})

	It("should handle a GPU restart request", func() {
		req := messaging.Msg{ID: cp.NewID(),
			Src: driverPort,
			Dst: toDriver.AsRemote(), Payload: protocol.GPURestartReq{}}

		toDriver.Deliver(req)

		tickUntilQuiet()

		// Step 1: hard-reset the caches.
		expectCtrlStep(toCaches, memcontrolprotocol.CmdReset, allCacheDsts())

		// Step 2: re-enable the TLBs.
		expectCtrlStep(toTLBs, memcontrolprotocol.CmdEnable, cp.State.TLBs)

		// Step 3: re-enable the ATs, reset the ROBs.
		atReqs := collectCtrlReqs(toATs)
		Expect(atReqs).To(HaveLen(20))
		for _, r := range atReqs {
			switch {
			case r.Dst[:2] == "AT":
				Expect(r.Payload.(memcontrolprotocol.Req).Command).To(Equal(memcontrolprotocol.CmdEnable))
			case r.Dst[:3] == "ROB":
				Expect(r.Payload.(memcontrolprotocol.Req).Command).To(Equal(memcontrolprotocol.CmdReset))
			default:
				Fail("unexpected destination " + string(r.Dst))
			}
		}
		ackCtrlReqs(toATs, atReqs)

		// Step 4: restart the CU pipelines.
		var cuRestartReqs []messaging.Msg
		for {
			msg, ok := toCUs.RetrieveOutgoing()
			if !ok {
				break
			}
			cuRestartReqs = append(
				cuRestartReqs, msg)
		}
		Expect(cuRestartReqs).To(HaveLen(10))

		for range cuRestartReqs {
			rsp := messaging.Msg{ID: cp.NewID(),
				Src: "CU",
				Dst: toCUs.AsRemote(), Payload: protocol.CUPipelineRestartRsp{}}

			toCUs.Deliver(rsp)
		}
		tickUntilQuiet()

		rsp, ok := toDriver.RetrieveOutgoing()
		Expect(ok).To(BeTrue())
		Expect(rsp.Payload).To(BeAssignableToTypeOf(protocol.GPURestartRsp{}))
		Expect(rsp.Dst).To(Equal(driverPort))
		Expect(cp.State.CtrlSeq).To(Equal(ctrlSeqNone))
	})

	It("should handle a RDMA drain command from the driver", func() {
		cmd := messaging.Msg{ID: cp.NewID(),
			Src: driverPort,
			Dst: toDriver.AsRemote(), Payload: protocol.RDMADrainCmdFromDriver{}}

		toDriver.Deliver(cmd)

		tickUntilQuiet()

		drainReq, ok := toRDMA.RetrieveOutgoing()
		Expect(ok).To(BeTrue())
		Expect(drainReq.Payload).To(BeAssignableToTypeOf(rdma.DrainReq{}))
		Expect(drainReq.Dst).To(Equal(rdmaPort))
	})

	It("should handle a RDMA drain response", func() {
		rsp := messaging.Msg{ID: cp.NewID(),
			Src: rdmaPort,
			Dst: toRDMA.AsRemote(), Payload: rdma.DrainRsp{}}

		toRDMA.Deliver(rsp)

		tickUntilQuiet()

		driverRsp, ok := toDriver.RetrieveOutgoing()
		Expect(ok).To(BeTrue())
		Expect(driverRsp).
			To(BeAssignableToTypeOf(messaging.Msg{Payload: protocol.RDMADrainRspToDriver{}}))
		Expect(driverRsp.Dst).To(Equal(driverPort))
	})

	It("should handle a RDMA restart command and response", func() {
		cmd := messaging.Msg{ID: cp.NewID(),
			Src: driverPort,
			Dst: toDriver.AsRemote(), Payload: protocol.RDMARestartCmdFromDriver{}}

		toDriver.Deliver(cmd)

		tickUntilQuiet()

		restartReq, ok := toRDMA.RetrieveOutgoing()
		Expect(ok).To(BeTrue())
		Expect(restartReq.Payload).To(BeAssignableToTypeOf(rdma.RestartReq{}))
		Expect(restartReq.Dst).To(Equal(rdmaPort))

		rsp := messaging.Msg{ID: cp.NewID(),
			Src: rdmaPort,
			Dst: toRDMA.AsRemote(), Payload: rdma.RestartRsp{}}

		toRDMA.Deliver(rsp)

		tickUntilQuiet()

		driverRsp, ok := toDriver.RetrieveOutgoing()
		Expect(ok).To(BeTrue())
		Expect(driverRsp).
			To(BeAssignableToTypeOf(messaging.Msg{Payload: protocol.RDMARestartRspToDriver{}}))
		Expect(driverRsp.Dst).To(Equal(driverPort))
	})
})

func tickCP(c *Comp) bool {
	a := c.Middlewares.Data.Tick()
	b := c.Middlewares.Control.Tick()
	return a || b
}
