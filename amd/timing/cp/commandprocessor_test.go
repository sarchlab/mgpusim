package cp

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
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
func (c *noopConn) PlugIn(port messaging.Port)       { port.SetConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend()                      {}

var _ = Describe("CommandProcessor", func() {
	var (
		mockCtrl   *gomock.Controller
		engine     timing.Engine
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
		for cp.Tick() {
		}
	}

	collectCtrlReqs := func(port messaging.Port) []memcontrolprotocol.Req {
		var reqs []memcontrolprotocol.Req
		for {
			msg := port.RetrieveOutgoing()
			if msg == nil {
				break
			}
			reqs = append(reqs, msg.(memcontrolprotocol.Req))
		}
		return reqs
	}

	ackCtrlReqs := func(
		port messaging.Port,
		reqs []memcontrolprotocol.Req,
	) {
		for _, req := range reqs {
			rsp := memcontrolprotocol.Rsp{
				Command: req.Command,
				Success: true,
			}
			rsp.ID = timing.GetIDGenerator().Generate()
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
			seen[req.Dst] = req.Command
		}
		for _, dst := range dsts {
			Expect(seen).To(HaveKeyWithValue(dst, cmd))
		}

		ackCtrlReqs(port, reqs)
	}

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = timing.NewSerialEngine()
		reg := modeling.NewStandaloneRegistrar(engine)

		cp = MakeBuilder().
			WithRegistrar(reg).
			WithDriver(driverPort).
			Build("CP")

		assign := func(name string) messaging.Port {
			p := modeling.MakePortBuilder().
				WithRegistrar(reg).
				WithComponent(cp).
				WithSpec(modeling.PortSpec{BufSize: 64}).
				Build(name)
			cp.AssignPort(name, p)
			(&noopConn{}).PlugIn(p)
			return p
		}

		toDriver = assign("ToDriver")
		toDMA = assign("ToDMA")
		toCUs = assign("ToCUs")
		toTLBs = assign("ToTLBs")
		toATs = assign("ToAddressTranslators")
		toCaches = assign("ToCaches")
		toRDMA = assign("ToRDMA")

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
		cpMW := cp.Middlewares()[0].(*cpMiddleware)
		cpMW.dispatchers = []dispatching.Dispatcher{dispatcher}
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

		req := protocol.LaunchKernelReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: driverPort,
				Dst: toDriver.AsRemote(),
			},
		}
		toDriver.Deliver(req)

		dispatcher.EXPECT().IsDispatching().Return(false)
		dispatcher.EXPECT().StartDispatching(gomock.Any()).
			Do(func(launched protocol.LaunchKernelReq) {
				Expect(launched.ID).To(Equal(req.ID))
			})

		madeProgress := cp.Tick()

		Expect(madeProgress).To(BeTrue())
		Expect(toDriver.PeekIncoming()).To(BeNil())
	})

	It("should wait if there is no dispatcher available", func() {
		useMockDispatcher()

		req := protocol.LaunchKernelReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: driverPort,
				Dst: toDriver.AsRemote(),
			},
		}
		toDriver.Deliver(req)

		dispatcher.EXPECT().IsDispatching().Return(true).AnyTimes()

		madeProgress := cp.Tick()

		Expect(madeProgress).To(BeFalse())
		Expect(toDriver.PeekIncoming()).NotTo(BeNil())
	})

	It("should forward memory copies to the DMA engine and respond "+
		"to the driver", func() {
		req := protocol.MemCopyH2DReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: driverPort,
				Dst: toDriver.AsRemote(),
			},
			SrcBuffer:  make([]byte, 128),
			DstAddress: 0x1000,
		}
		toDriver.Deliver(req)

		tickUntilQuiet()

		cloned := toDMA.RetrieveOutgoing().(protocol.MemCopyH2DReq)
		Expect(cloned.Dst).To(Equal(dmaPort))
		Expect(cloned.DstAddress).To(Equal(req.DstAddress))
		Expect(cloned.ID).NotTo(Equal(req.ID))
		Expect(cp.State.BottomMemCopyH2DToTop).To(HaveKey(cloned.ID))

		dmaRsp := protocol.GeneralRsp{
			MsgMeta: messaging.MsgMeta{
				ID:    timing.GetIDGenerator().Generate(),
				Src:   dmaPort,
				Dst:   toDMA.AsRemote(),
				RspTo: cloned.ID,
			},
		}
		toDMA.Deliver(dmaRsp)

		tickUntilQuiet()

		rsp := toDriver.RetrieveOutgoing().(protocol.GeneralRsp)
		Expect(rsp.RspTo).To(Equal(req.ID))
		Expect(rsp.Dst).To(Equal(driverPort))
		Expect(cp.State.BottomMemCopyH2DToTop).To(BeEmpty())
	})

	It("should handle a driver flush request", func() {
		req := protocol.FlushReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: driverPort,
				Dst: toDriver.AsRemote(),
			},
		}
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

		rsp := toDriver.RetrieveOutgoing().(protocol.GeneralRsp)
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

		req := protocol.FlushReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: driverPort,
				Dst: toDriver.AsRemote(),
			},
		}
		toDriver.Deliver(req)

		tickUntilQuiet()

		rsp := toDriver.RetrieveOutgoing().(protocol.GeneralRsp)
		Expect(rsp.RspTo).To(Equal(req.ID))
	})

	It("should handle a shootdown command", func() {
		cmd := protocol.ShootDownCommand{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: driverPort,
				Dst: toDriver.AsRemote(),
			},
			VAddr: []uint64{0x1000},
			PID:   1,
		}
		toDriver.Deliver(cmd)

		tickUntilQuiet()

		Expect(cp.State.ShootDownInProcess).To(BeTrue())

		// Step 1: flush the CU pipelines.
		var cuFlushReqs []protocol.CUPipelineFlushReq
		for {
			msg := toCUs.RetrieveOutgoing()
			if msg == nil {
				break
			}
			cuFlushReqs = append(
				cuFlushReqs, msg.(protocol.CUPipelineFlushReq))
		}
		Expect(cuFlushReqs).To(HaveLen(10))
		Expect(cp.State.PendingAcks).To(Equal(uint64(10)))

		for range cuFlushReqs {
			rsp := protocol.CUPipelineFlushRsp{
				MsgMeta: messaging.MsgMeta{
					ID:  timing.GetIDGenerator().Generate(),
					Src: "CU",
					Dst: toCUs.AsRemote(),
				},
			}
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
			Expect(req.Command).To(Equal(memcontrolprotocol.CmdInvalidate))
			Expect(req.Addresses).To(Equal([]uint64{0x1000}))
			Expect(req.PID).To(Equal(cmd.PID))
		}
		ackCtrlReqs(toTLBs, tlbReqs)

		// Finally, the CP responds to the driver.
		rsp := toDriver.RetrieveOutgoing()
		Expect(rsp).To(BeAssignableToTypeOf(protocol.ShootDownCompleteRsp{}))
		Expect(rsp.Meta().Dst).To(Equal(driverPort))
		Expect(cp.State.ShootDownInProcess).To(BeFalse())
		Expect(cp.State.CtrlSeq).To(Equal(ctrlSeqNone))
	})

	It("should handle a GPU restart request", func() {
		req := protocol.GPURestartReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: driverPort,
				Dst: toDriver.AsRemote(),
			},
		}
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
				Expect(r.Command).To(Equal(memcontrolprotocol.CmdEnable))
			case r.Dst[:3] == "ROB":
				Expect(r.Command).To(Equal(memcontrolprotocol.CmdReset))
			default:
				Fail("unexpected destination " + string(r.Dst))
			}
		}
		ackCtrlReqs(toATs, atReqs)

		// Step 4: restart the CU pipelines.
		var cuRestartReqs []protocol.CUPipelineRestartReq
		for {
			msg := toCUs.RetrieveOutgoing()
			if msg == nil {
				break
			}
			cuRestartReqs = append(
				cuRestartReqs, msg.(protocol.CUPipelineRestartReq))
		}
		Expect(cuRestartReqs).To(HaveLen(10))

		for range cuRestartReqs {
			rsp := protocol.CUPipelineRestartRsp{
				MsgMeta: messaging.MsgMeta{
					ID:  timing.GetIDGenerator().Generate(),
					Src: "CU",
					Dst: toCUs.AsRemote(),
				},
			}
			toCUs.Deliver(rsp)
		}
		tickUntilQuiet()

		rsp := toDriver.RetrieveOutgoing()
		Expect(rsp).To(BeAssignableToTypeOf(protocol.GPURestartRsp{}))
		Expect(rsp.Meta().Dst).To(Equal(driverPort))
		Expect(cp.State.CtrlSeq).To(Equal(ctrlSeqNone))
	})

	It("should handle a RDMA drain command from the driver", func() {
		cmd := protocol.RDMADrainCmdFromDriver{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: driverPort,
				Dst: toDriver.AsRemote(),
			},
		}
		toDriver.Deliver(cmd)

		tickUntilQuiet()

		drainReq := toRDMA.RetrieveOutgoing()
		Expect(drainReq).To(BeAssignableToTypeOf(rdma.DrainReq{}))
		Expect(drainReq.Meta().Dst).To(Equal(rdmaPort))
	})

	It("should handle a RDMA drain response", func() {
		rsp := rdma.DrainRsp{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: rdmaPort,
				Dst: toRDMA.AsRemote(),
			},
		}
		toRDMA.Deliver(rsp)

		tickUntilQuiet()

		driverRsp := toDriver.RetrieveOutgoing()
		Expect(driverRsp).
			To(BeAssignableToTypeOf(protocol.RDMADrainRspToDriver{}))
		Expect(driverRsp.Meta().Dst).To(Equal(driverPort))
	})

	It("should handle a RDMA restart command and response", func() {
		cmd := protocol.RDMARestartCmdFromDriver{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: driverPort,
				Dst: toDriver.AsRemote(),
			},
		}
		toDriver.Deliver(cmd)

		tickUntilQuiet()

		restartReq := toRDMA.RetrieveOutgoing()
		Expect(restartReq).To(BeAssignableToTypeOf(rdma.RestartReq{}))
		Expect(restartReq.Meta().Dst).To(Equal(rdmaPort))

		rsp := rdma.RestartRsp{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: rdmaPort,
				Dst: toRDMA.AsRemote(),
			},
		}
		toRDMA.Deliver(rsp)

		tickUntilQuiet()

		driverRsp := toDriver.RetrieveOutgoing()
		Expect(driverRsp).
			To(BeAssignableToTypeOf(protocol.RDMARestartRspToDriver{}))
		Expect(driverRsp.Meta().Dst).To(Equal(driverPort))
	})
})
