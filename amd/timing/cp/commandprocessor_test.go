package cp

import (
	"fmt"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/timing"
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
		spec       Spec
		mockCtrl   *gomock.Controller
		reg        *sim.Simulation
		cp         *Comp
		dispatcher *MockDispatcher

		toDriver messaging.Port
		toDMA    messaging.Port
		toCaches messaging.Port
		toRDMA   messaging.Port

		toTranslation messaging.Port
		toDataMover   messaging.Port
		pmPort        messaging.Port
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
		spec = DefaultSpec()
		spec.Driver = driverPort
		spec.DMAEngine = dmaPort
		spec.RDMA = rdmaPort
		spec.L1ICaches = remoteList("L1I", 2)
		spec.L1SCaches = remoteList("L1S", 2)
		spec.L1VCaches = remoteList("L1V", 2)
		spec.L2Caches = remoteList("L2", 2)
		spec.AddressTranslators = remoteList("AT", 3)
		spec.TLBs = remoteList("TLB", 2)
		spec.DataMover = "DataMover.Top"
	})

	JustBeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		reg = sim.MakeBuilder().WithTaskTracking().Build()
		DeferCleanup(func() { Expect(reg.Terminate()).To(Succeed()) })

		cp = MakeBuilder().
			WithSimulation(reg).
			WithSpec(spec).
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
		assign("ToCUs")
		toCaches = assign("ToCaches")
		toRDMA = assign("ToRDMA")
		toTranslation = assign("ToTranslation")
		toDataMover = assign("ToDataMover")
		pmPort = assign("PageMigration")

		Expect(reg.Initialize()).To(Succeed())

		dispatcher = NewMockDispatcher(mockCtrl)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	useMockDispatcher := func() {
		cp.Middlewares.Data.dispatchers = []dispatching.Dispatcher{dispatcher}
		dispatcher.EXPECT().Tick().Return(false).AnyTimes()
	}

	l1Dsts := func() []messaging.RemotePort {
		var dsts []messaging.RemotePort
		dsts = append(dsts, cp.Spec.L1ICaches...)
		dsts = append(dsts, cp.Spec.L1SCaches...)
		dsts = append(dsts, cp.Spec.L1VCaches...)
		return dsts
	}

	allCacheDsts := func() []messaging.RemotePort {
		return append(l1Dsts(), cp.Spec.L2Caches...)
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
			cp.Spec.L1SCaches...), cp.Spec.L1VCaches...)
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
			cp.Spec.L2Caches)
		expectCtrlStep(toCaches, memcontrolprotocol.CmdFlush,
			cp.Spec.L2Caches)
		expectCtrlStep(toCaches, memcontrolprotocol.CmdInvalidate,
			cp.Spec.L2Caches)
		expectCtrlStep(toCaches, memcontrolprotocol.CmdEnable, allCacheDsts())

		rsp, ok := toDriver.RetrieveOutgoing()
		Expect(ok).To(BeTrue())
		Expect(rsp.RspTo).To(Equal(req.ID))
		Expect(rsp.Dst).To(Equal(driverPort))
		Expect(cp.State.CtrlSeq).To(Equal(ctrlSeqNone))
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
				cp.Spec.L2Caches)
			expectCtrlStep(toCaches, memcontrolprotocol.CmdFlush,
				cp.Spec.L2Caches)
			expectCtrlStep(toCaches, memcontrolprotocol.CmdInvalidate,
				cp.Spec.L2Caches)
			expectCtrlStep(toCaches, memcontrolprotocol.CmdEnable,
				allCacheDsts())

			rsp, ok := toDriver.RetrieveOutgoing()
			Expect(ok).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(req.ID))
			Expect(rsp.Dst).To(Equal(driverPort))
		}
		Expect(toDriver.OutgoingBuf().Size()).To(BeZero())
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
		Expect(driverRsp.Payload).To(BeAssignableToTypeOf(protocol.RDMADrainRspToDriver{}))
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
		Expect(driverRsp.Payload).To(BeAssignableToTypeOf(protocol.RDMARestartRspToDriver{}))
		Expect(driverRsp.Dst).To(Equal(driverPort))
	})
	Context("without caches", func() {
		BeforeEach(func() {
			spec.L1ICaches = nil
			spec.L1SCaches = nil
			spec.L1VCaches = nil
			spec.L2Caches = nil
		})
		It("should dispatch a kernel immediately when there is no cache", func() {
			useMockDispatcher()

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

		It("should respond to a flush request immediately when there is no "+
			"cache", func() {

			req := messaging.Msg{ID: cp.NewID(),
				Src: driverPort,
				Dst: toDriver.AsRemote(), Payload: protocol.FlushReq{}}

			toDriver.Deliver(req)

			tickUntilQuiet()

			rsp, ok := toDriver.RetrieveOutgoing()
			Expect(ok).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(req.ID))
		})
	})

	Context("page migration", func() {
		migrationReq := func(payload any) messaging.Msg {
			req := messaging.Msg{
				ID:      cp.NewID(),
				Src:     "Driver.PageMigration",
				Dst:     pmPort.AsRemote(),
				Payload: payload,
			}
			pmPort.Deliver(req)
			tickUntilQuiet()

			return req
		}

		expectDone := func(req messaging.Msg) {
			rsp, ok := pmPort.RetrieveOutgoing()
			Expect(ok).To(BeTrue())
			Expect(rsp.Payload).To(BeAssignableToTypeOf(protocol.GeneralRsp{}))
			Expect(rsp.RspTo).To(Equal(req.ID))
			Expect(rsp.Dst).To(Equal(messaging.RemotePort("Driver.PageMigration")))
		}

		expectNotDone := func() {
			_, ok := pmPort.PeekOutgoing()
			Expect(ok).To(BeFalse())
		}

		pageOf := func(addr uint64) memcontrolprotocol.AddressRange {
			return memcontrolprotocol.AddressRange{Start: addr, End: addr + 4096}
		}

		// expectPageCtrl asserts that reqs are one request per destination for
		// the page at addr of process pid, addressed by range only.
		expectPageCtrl := func(
			reqs []messaging.Msg,
			cmd memcontrolprotocol.Command,
			dsts []messaging.RemotePort,
			pid int,
			addr uint64,
		) {
			Expect(reqs).To(HaveLen(len(dsts)))
			for i, r := range reqs {
				Expect(r.Dst).To(Equal(dsts[i]))
				Expect(r.Payload).To(Equal(memcontrolprotocol.Req{
					Command:  cmd,
					PID:      vm.PID(pid),
					HasRange: true,
					Range:    pageOf(addr),
				}))
			}
		}

		It("holds the page in every address translator", func() {
			useMockDispatcher()
			req := migrationReq(protocol.PageHoldReq{PID: 1, VAddr: 0x1000, PageSize: 4096})

			reqs := collectCtrlReqs(toTranslation)
			expectPageCtrl(reqs, memcontrolprotocol.CmdHold,
				cp.Spec.AddressTranslators, 1, 0x1000)

			// The answer waits for the last translator.
			ackCtrlReqs(toTranslation, reqs[:2])
			expectNotDone()

			ackCtrlReqs(toTranslation, reqs[2:])
			expectDone(req)
			Expect(cp.State.PageJobs).To(BeEmpty())
			Expect(cp.State.CtrlWaits).To(BeEmpty())
		})

		It("releases the page in every address translator", func() {
			useMockDispatcher()
			req := migrationReq(protocol.PageReleaseReq{PID: 1, VAddr: 0x1000, PageSize: 4096})

			reqs := collectCtrlReqs(toTranslation)
			expectPageCtrl(reqs, memcontrolprotocol.CmdRelease,
				cp.Spec.AddressTranslators, 1, 0x1000)

			ackCtrlReqs(toTranslation, reqs)
			expectDone(req)
		})

		It("works on the pages of several commands at the same time", func() {
			useMockDispatcher()
			first := migrationReq(protocol.PageHoldReq{PID: 1, VAddr: 0x1000, PageSize: 4096})
			second := migrationReq(protocol.PageHoldReq{PID: 1, VAddr: 0x2000, PageSize: 4096})

			// Both holds are out before either is acknowledged.
			reqs := collectCtrlReqs(toTranslation)
			expectPageCtrl(reqs[:3], memcontrolprotocol.CmdHold,
				cp.Spec.AddressTranslators, 1, 0x1000)
			expectPageCtrl(reqs[3:], memcontrolprotocol.CmdHold,
				cp.Spec.AddressTranslators, 1, 0x2000)

			// Each page is answered with its own last acknowledgement: the
			// second page does not wait for the first.
			ackCtrlReqs(toTranslation, reqs[3:])
			expectDone(second)

			ackCtrlReqs(toTranslation, reqs[:3])
			expectDone(first)
		})

		It("does not wait for a control sequence", func() {
			useMockDispatcher()
			cp.State.CtrlSeq = ctrlSeqKernelStart
			cp.State.PendingAcks = 1

			migrationReq(protocol.PageHoldReq{PID: 1, VAddr: 0x1000, PageSize: 4096})

			Expect(collectCtrlReqs(toTranslation)).To(HaveLen(3))
		})

		It("flushes and then invalidates the frame on every L2 bank", func() {
			useMockDispatcher()
			req := migrationReq(protocol.PageFlushReq{PAddr: 0x8000, PageSize: 4096})

			flushes := collectCtrlReqs(toCaches)
			expectPageCtrl(flushes, memcontrolprotocol.CmdFlush, cp.Spec.L2Caches, 0, 0x8000)

			// A bank invalidates only after its own Flush is acknowledged.
			ackCtrlReqs(toCaches, flushes[:1])
			invalidates := collectCtrlReqs(toCaches)
			expectPageCtrl(invalidates, memcontrolprotocol.CmdInvalidate,
				cp.Spec.L2Caches[:1], 0, 0x8000)

			ackCtrlReqs(toCaches, invalidates)
			expectNotDone()

			ackCtrlReqs(toCaches, flushes[1:])
			invalidates = collectCtrlReqs(toCaches)
			expectPageCtrl(invalidates, memcontrolprotocol.CmdInvalidate,
				cp.Spec.L2Caches[1:], 0, 0x8000)

			ackCtrlReqs(toCaches, invalidates)
			expectDone(req)
		})

		It("invalidates the frame on every L2 bank without a flush", func() {
			useMockDispatcher()
			req := migrationReq(protocol.PageInvalidateReq{PAddr: 0x8000, PageSize: 4096})

			reqs := collectCtrlReqs(toCaches)
			expectPageCtrl(reqs, memcontrolprotocol.CmdInvalidate, cp.Spec.L2Caches, 0, 0x8000)

			ackCtrlReqs(toCaches, reqs)
			expectDone(req)
		})

		It("maintains frames that have no byte in common at the same time", func() {
			useMockDispatcher()
			first := migrationReq(protocol.PageFlushReq{PAddr: 0x8000, PageSize: 4096})
			second := migrationReq(protocol.PageFlushReq{PAddr: 0x9000, PageSize: 4096})
			third := migrationReq(protocol.PageInvalidateReq{PAddr: 0xa000, PageSize: 4096})

			// The requests of all three frames are with both banks before any
			// is acknowledged.
			reqs := collectCtrlReqs(toCaches)
			expectPageCtrl(reqs[:2], memcontrolprotocol.CmdFlush, cp.Spec.L2Caches, 0, 0x8000)
			expectPageCtrl(reqs[2:4], memcontrolprotocol.CmdFlush, cp.Spec.L2Caches, 0, 0x9000)
			expectPageCtrl(reqs[4:], memcontrolprotocol.CmdInvalidate, cp.Spec.L2Caches, 0, 0xa000)

			// The acknowledgements arrive staggered. Bank 1 acknowledges the
			// Flush of the second frame: it alone gets the Invalidate of that
			// frame, while no bank has acknowledged the Flush of the first.
			ackCtrlReqs(toCaches, reqs[3:4])
			invalidates := collectCtrlReqs(toCaches)
			expectPageCtrl(invalidates, memcontrolprotocol.CmdInvalidate,
				cp.Spec.L2Caches[1:], 0, 0x9000)

			// A job is answered with its own last acknowledgement.
			ackCtrlReqs(toCaches, reqs[4:])
			expectDone(third)

			ackCtrlReqs(toCaches, invalidates)
			expectNotDone()

			ackCtrlReqs(toCaches, reqs[2:3])
			invalidates = collectCtrlReqs(toCaches)
			expectPageCtrl(invalidates, memcontrolprotocol.CmdInvalidate,
				cp.Spec.L2Caches[:1], 0, 0x9000)
			ackCtrlReqs(toCaches, invalidates)
			expectDone(second)

			// The first frame, whose Flush took the longest, ends last.
			ackCtrlReqs(toCaches, reqs[1:2])
			ackCtrlReqs(toCaches, reqs[:1])
			invalidates = collectCtrlReqs(toCaches)
			expectPageCtrl(invalidates[:1], memcontrolprotocol.CmdInvalidate,
				cp.Spec.L2Caches[1:], 0, 0x8000)
			expectPageCtrl(invalidates[1:], memcontrolprotocol.CmdInvalidate,
				cp.Spec.L2Caches[:1], 0, 0x8000)
			ackCtrlReqs(toCaches, invalidates)
			expectDone(first)

			expectNotDone()
			Expect(cp.State.L2Ops).To(BeEmpty())
			Expect(cp.State.CtrlWaits).To(BeEmpty())
			Expect(cp.State.PageJobs).To(BeEmpty())
		})

		It("maintains frames that have a byte in common in the order they arrived", func() {
			useMockDispatcher()
			first := migrationReq(protocol.PageFlushReq{PAddr: 0x8000, PageSize: 4096})

			// The second frame has bytes of the first. The third has bytes of
			// the second only, and the fourth has none of any of them.
			second := migrationReq(protocol.PageInvalidateReq{PAddr: 0x8800, PageSize: 4096})
			third := migrationReq(protocol.PageFlushReq{PAddr: 0x9000, PageSize: 4096})
			apart := migrationReq(protocol.PageInvalidateReq{PAddr: 0x20000, PageSize: 4096})

			// The first and the fourth start. The third waits for the second,
			// although the second has not started itself.
			reqs := collectCtrlReqs(toCaches)
			expectPageCtrl(reqs[:2], memcontrolprotocol.CmdFlush, cp.Spec.L2Caches, 0, 0x8000)
			expectPageCtrl(reqs[2:], memcontrolprotocol.CmdInvalidate, cp.Spec.L2Caches, 0, 0x20000)

			ackCtrlReqs(toCaches, reqs[2:])
			expectDone(apart)
			Expect(collectCtrlReqs(toCaches)).To(BeEmpty())

			// The second starts when the last bank has invalidated the first.
			ackCtrlReqs(toCaches, reqs[:2])
			invalidates := collectCtrlReqs(toCaches)
			expectPageCtrl(invalidates, memcontrolprotocol.CmdInvalidate,
				cp.Spec.L2Caches, 0, 0x8000)
			ackCtrlReqs(toCaches, invalidates[:1])
			Expect(collectCtrlReqs(toCaches)).To(BeEmpty())
			ackCtrlReqs(toCaches, invalidates[1:])
			expectDone(first)

			reqs = collectCtrlReqs(toCaches)
			expectPageCtrl(reqs, memcontrolprotocol.CmdInvalidate, cp.Spec.L2Caches, 0, 0x8800)
			ackCtrlReqs(toCaches, reqs)
			expectDone(second)

			// Then the third.
			reqs = collectCtrlReqs(toCaches)
			expectPageCtrl(reqs, memcontrolprotocol.CmdFlush, cp.Spec.L2Caches, 0, 0x9000)
			ackCtrlReqs(toCaches, reqs)
			reqs = collectCtrlReqs(toCaches)
			expectPageCtrl(reqs, memcontrolprotocol.CmdInvalidate, cp.Spec.L2Caches, 0, 0x9000)
			ackCtrlReqs(toCaches, reqs)
			expectDone(third)

			expectNotDone()
			Expect(cp.State.L2Ops).To(BeEmpty())
		})

		// flushScenario is a flush of the driver that arrives while the L2
		// maintenance of one frame is in progress, that of a second frame
		// waits for the first, and that of a third arrives after the flush.
		type flushScenario struct {
			active, blocked, later, flush messaging.Msg
			flushes                       []messaging.Msg
		}

		startFlushScenario := func() flushScenario {
			var s flushScenario

			s.active = migrationReq(protocol.PageFlushReq{PAddr: 0x8000, PageSize: 4096})
			s.blocked = migrationReq(protocol.PageInvalidateReq{PAddr: 0x8000, PageSize: 4096})
			s.flushes = collectCtrlReqs(toCaches)
			expectPageCtrl(s.flushes, memcontrolprotocol.CmdFlush, cp.Spec.L2Caches, 0, 0x8000)

			s.flush = messaging.Msg{ID: cp.NewID(), Src: driverPort,
				Dst: toDriver.AsRemote(), Payload: protocol.FlushReq{}}
			toDriver.Deliver(s.flush)
			s.later = migrationReq(protocol.PageInvalidateReq{PAddr: 0x9000, PageSize: 4096})

			// The flush waits for the frame in progress. The frame that
			// arrived after the flush does not start, although it has no
			// byte in common with any other.
			Expect(cp.State.CtrlSeq).To(Equal(ctrlSeqNone))
			Expect(toDriver.IncomingBuf().Size()).To(Equal(1))
			Expect(collectCtrlReqs(toCaches)).To(BeEmpty())

			return s
		}

		// finishFlushScenario runs the scenario to its end and returns what
		// the Command Processor sent, in order.
		finishFlushScenario := func(s flushScenario) []string {
			var sent []string

			step := func(reqs []messaging.Msg) {
				for _, req := range reqs {
					payload := req.Payload.(memcontrolprotocol.Req)
					sent = append(sent, fmt.Sprintf("%v>%s[%#x,%#x)",
						payload.Command, req.Dst, payload.Range.Start, payload.Range.End))
				}

				ackCtrlReqs(toCaches, reqs)
			}

			answers := func() {
				for {
					rsp, ok := pmPort.RetrieveOutgoing()
					if !ok {
						return
					}

					sent = append(sent, fmt.Sprintf("answer to %d", rsp.RspTo))
				}
			}

			// The frame in progress runs to its end: each bank still gets
			// the Invalidate that follows its Flush, and the flush of the
			// driver waits for the last of them.
			ackCtrlReqs(toCaches, s.flushes[1:])
			invalidates := collectCtrlReqs(toCaches)
			expectPageCtrl(invalidates, memcontrolprotocol.CmdInvalidate,
				cp.Spec.L2Caches[1:], 0, 0x8000)
			step(invalidates)

			ackCtrlReqs(toCaches, s.flushes[:1])
			invalidates = collectCtrlReqs(toCaches)
			expectPageCtrl(invalidates, memcontrolprotocol.CmdInvalidate,
				cp.Spec.L2Caches[:1], 0, 0x8000)
			Expect(cp.State.CtrlSeq).To(Equal(ctrlSeqNone))
			step(invalidates)
			answers()

			// The flush runs before the frames that wait.
			Expect(cp.State.CtrlSeq).To(Equal(ctrlSeqFlush))
			Expect(toDriver.IncomingBuf().Size()).To(BeZero())

			for _, dsts := range [][]messaging.RemotePort{
				l1Dsts(), l1Dsts(), l1Dsts(),
				cp.Spec.L2Caches, cp.Spec.L2Caches, cp.Spec.L2Caches, allCacheDsts(),
			} {
				reqs := collectCtrlReqs(toCaches)
				Expect(reqs).To(HaveLen(len(dsts)))
				step(reqs)
			}

			rsp, ok := toDriver.RetrieveOutgoing()
			Expect(ok).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(s.flush.ID))
			answers()

			// The flush is over: the frames that waited are maintained.
			reqs := collectCtrlReqs(toCaches)
			expectPageCtrl(reqs[:2], memcontrolprotocol.CmdInvalidate, cp.Spec.L2Caches, 0, 0x8000)
			expectPageCtrl(reqs[2:], memcontrolprotocol.CmdInvalidate, cp.Spec.L2Caches, 0, 0x9000)
			step(reqs)
			answers()

			Expect(collectCtrlReqs(toCaches)).To(BeEmpty())
			Expect(cp.State.L2Ops).To(BeEmpty())
			Expect(cp.State.CtrlWaits).To(BeEmpty())
			Expect(cp.State.PageJobs).To(BeEmpty())

			return sent
		}

		It("lets the flush of the driver go before the L2 maintenance that has not started", func() {
			useMockDispatcher()
			s := startFlushScenario()
			sent := finishFlushScenario(s)

			// Every command is answered once: the frame that was in progress
			// before the flush, the other two after it.
			var answers []string
			for _, what := range sent {
				if strings.HasPrefix(what, "answer") {
					answers = append(answers, what)
				}
			}

			Expect(answers).To(Equal([]string{
				fmt.Sprintf("answer to %d", s.active.ID),
				fmt.Sprintf("answer to %d", s.blocked.ID),
				fmt.Sprintf("answer to %d", s.later.ID),
			}))
			Expect(sent[2]).To(Equal(answers[0]))
		})

		It("lets flushes of the driver that follow each other all go before the frames that wait", func() {
			useMockDispatcher()

			var flushes []messaging.Msg
			for range 2 {
				flush := messaging.Msg{ID: cp.NewID(), Src: driverPort,
					Dst: toDriver.AsRemote(), Payload: protocol.FlushReq{}}
				toDriver.Deliver(flush)
				flushes = append(flushes, flush)
			}

			// runFlush acknowledges the steps of the flush that runs. Each
			// step has the requests of the flush only: no frame is
			// maintained between them.
			runFlush := func() {
				Expect(cp.State.CtrlSeq).To(Equal(ctrlSeqFlush))
				expectCtrlStep(toCaches, memcontrolprotocol.CmdDrain, l1Dsts())
				expectCtrlStep(toCaches, memcontrolprotocol.CmdFlush, l1Dsts())
				expectCtrlStep(toCaches, memcontrolprotocol.CmdInvalidate, l1Dsts())
				expectCtrlStep(toCaches, memcontrolprotocol.CmdDrain, cp.Spec.L2Caches)
				expectCtrlStep(toCaches, memcontrolprotocol.CmdFlush, cp.Spec.L2Caches)
				expectCtrlStep(toCaches, memcontrolprotocol.CmdInvalidate, cp.Spec.L2Caches)
				expectCtrlStep(toCaches, memcontrolprotocol.CmdEnable, allCacheDsts())
			}

			expectAnswered := func(flush messaging.Msg) {
				rsp, ok := toDriver.RetrieveOutgoing()
				Expect(ok).To(BeTrue())
				Expect(rsp.RspTo).To(Equal(flush.ID))
			}

			// A frame arrives while the first flush runs, with the second
			// flush behind the first at the driver port.
			tickUntilQuiet()
			page := migrationReq(protocol.PageInvalidateReq{PAddr: 0x8000, PageSize: 4096})

			// The port to the driver is full, so the answer to the first
			// flush cannot leave when the flush is over, and the second
			// flush cannot start yet. The frame still does not start: a
			// flush is next.
			held := 0
			for toDriver.CanSend() {
				toDriver.Send(messaging.Msg{ID: cp.NewID(), Src: toDriver.AsRemote(),
					Dst: driverPort, Payload: protocol.GeneralRsp{}})
				held++
			}

			runFlush()
			Expect(cp.State.CtrlSeq).To(Equal(ctrlSeqNone))
			Expect(cp.State.CurrFlushReq.ID).To(Equal(flushes[0].ID))
			Expect(collectCtrlReqs(toCaches)).To(BeEmpty())

			// The port is free again. The answer leaves, and the second
			// flush runs before the frame.
			for range held {
				toDriver.RetrieveOutgoing()
			}

			tickUntilQuiet()
			expectAnswered(flushes[0])
			runFlush()
			expectAnswered(flushes[1])
			expectNotDone()

			// Only now is the frame maintained.
			reqs := collectCtrlReqs(toCaches)
			expectPageCtrl(reqs, memcontrolprotocol.CmdInvalidate, cp.Spec.L2Caches, 0, 0x8000)
			ackCtrlReqs(toCaches, reqs)
			expectDone(page)
		})

		It("continues a waiting flush and the L2 maintenance around it from a checkpoint", func() {
			useMockDispatcher()
			s := startFlushScenario()

			engine := reg.Engine().(*timing.SerialEngine)
			path := filepath.Join(GinkgoT().TempDir(), "flush-waiting.tar.gz")
			Expect(engine.Run()).To(Succeed())
			Expect(reg.SaveCheckpoint(path)).To(Succeed())

			uninterrupted := finishFlushScenario(s)

			Expect(engine.Run()).To(Succeed())
			Expect(reg.LoadCheckpoint(path)).To(Succeed())
			Expect(toDriver.IncomingBuf().Size()).To(Equal(1))
			Expect(cp.State.L2Ops).To(HaveLen(3))

			Expect(finishFlushScenario(s)).To(Equal(uninterrupted))
		})

		It("shoots the pages that wait down in one round of the TLBs", func() {
			useMockDispatcher()
			first := migrationReq(protocol.PageShootdownReq{PID: 1, VAddr: 0x1000, PageSize: 4096})

			pauses := collectCtrlReqs(toTranslation)
			Expect(pauses).To(HaveLen(2))
			Expect(pauses[0].Payload).To(Equal(
				memcontrolprotocol.Req{Command: memcontrolprotocol.CmdPause}))

			// Two more pages arrive during the round: they form the next one.
			second := migrationReq(protocol.PageShootdownReq{PID: 1, VAddr: 0x2000, PageSize: 4096})
			third := migrationReq(protocol.PageShootdownReq{PID: 2, VAddr: 0x3000, PageSize: 4096})
			Expect(collectCtrlReqs(toTranslation)).To(BeEmpty())

			ackCtrlReqs(toTranslation, pauses)
			invalidates := collectCtrlReqs(toTranslation)
			expectPageCtrl(invalidates, memcontrolprotocol.CmdInvalidate, cp.Spec.TLBs, 1, 0x1000)

			ackCtrlReqs(toTranslation, invalidates)
			expectCtrlStep(toTranslation, memcontrolprotocol.CmdEnable, cp.Spec.TLBs)
			expectDone(first)

			expectCtrlStep(toTranslation, memcontrolprotocol.CmdPause, cp.Spec.TLBs)
			invalidates = collectCtrlReqs(toTranslation)
			expectPageCtrl(invalidates[:2], memcontrolprotocol.CmdInvalidate,
				cp.Spec.TLBs, 1, 0x2000)
			expectPageCtrl(invalidates[2:], memcontrolprotocol.CmdInvalidate,
				cp.Spec.TLBs, 2, 0x3000)

			ackCtrlReqs(toTranslation, invalidates)
			expectCtrlStep(toTranslation, memcontrolprotocol.CmdEnable, cp.Spec.TLBs)
			expectDone(second)
			expectDone(third)
			Expect(cp.State.Shootdown.Step).To(Equal(shootdownIdle))
		})

		It("pulls a page with the data mover", func() {
			useMockDispatcher()
			req := migrationReq(protocol.PageCopyReq{
				SrcAddress: 0x1000, DstAddress: 0x9000, PageSize: 4096})

			move, ok := toDataMover.RetrieveOutgoing()
			Expect(ok).To(BeTrue())
			Expect(move.Dst).To(Equal(messaging.RemotePort("DataMover.Top")))
			Expect(move.Payload).To(Equal(datamoverprotocol.Req{
				SrcAddress: 0x1000,
				DstAddress: 0x9000,
				ByteSize:   4096,
				SrcSide:    datamoverprotocol.SideOutside,
				DstSide:    datamoverprotocol.SideInside,
			}))

			toDataMover.Deliver(messaging.Msg{
				ID: cp.NewID(), RspTo: move.ID, Payload: datamoverprotocol.Rsp{}})
			tickUntilQuiet()

			expectDone(req)
			Expect(cp.State.MoveJobs).To(BeEmpty())
		})

		It("refuses to copy a page without a data mover", func() {
			useMockDispatcher()
			cp.Spec.DataMover = ""

			pmPort.Deliver(messaging.Msg{ID: cp.NewID(), Src: "Driver.PageMigration",
				Dst: pmPort.AsRemote(), Payload: protocol.PageCopyReq{PageSize: 4096}})

			Expect(func() { tickCP(cp) }).To(PanicWith("cp: CP has no data mover to copy a page with"))
		})

		It("submits the copies in arrival order and answers each on its own", func() {
			useMockDispatcher()
			var reqs []messaging.Msg
			for i := range uint64(3) {
				reqs = append(reqs, migrationReq(protocol.PageCopyReq{
					SrcAddress: 0x1000 * (i + 1), DstAddress: 0x9000, PageSize: 4096}))
			}

			// All three are with the data mover before any completes.
			moves := collectCtrlReqs(toDataMover)
			Expect(moves).To(HaveLen(3))
			for i, move := range moves {
				Expect(move.Payload.(datamoverprotocol.Req).SrcAddress).
					To(Equal(0x1000 * uint64(i+1)))
			}

			toDataMover.Deliver(messaging.Msg{
				ID: cp.NewID(), RspTo: moves[1].ID, Payload: datamoverprotocol.Rsp{}})
			tickUntilQuiet()

			expectDone(reqs[1])
			expectNotDone()
		})
	})
})

func tickCP(c *Comp) bool {
	a := c.Middlewares.Data.Tick()
	b := c.Middlewares.Control.Tick()
	return a || b
}
