package tlb

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/vm"
	"github.com/sarchlab/mgpusim/v3/mem/vm/tlb/internal"
)

var _ = Describe("TLB", func() {

	var (
		mockCtrl    *gomock.Controller
		engine      *MockEngine
		tlb         *TLB
		set         *MockSet
		topPort     *MockPort
		bottomPort  *MockPort
		controlPort *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)
		set = NewMockSet(mockCtrl)
		topPort = NewMockPort(mockCtrl)
		bottomPort = NewMockPort(mockCtrl)
		controlPort = NewMockPort(mockCtrl)

		tlb = MakeBuilder().WithEngine(engine).Build("TLB")
		tlb.topPort = topPort
		tlb.bottomPort = bottomPort
		tlb.controlPort = controlPort
		tlb.Sets = []internal.Set{set}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if there is no req in TopPort", func() {
		topPort.EXPECT().Peek().Return(nil)

		madeProgress := tlb.lookup(10)

		Expect(madeProgress).To(BeFalse())
	})

	Context("hit", func() {
		var (
			wayID int
			page  vm.Page
			req   *vm.TranslationReq
		)

		BeforeEach(func() {
			wayID = 1
			page = vm.Page{
				PID:   1,
				VAddr: 0x100,
				PAddr: 0x200,
				Valid: true,
			}
			set.EXPECT().Lookup(vm.PID(1), uint64(0x100)).
				Return(wayID, page, true)

			req = vm.TranslationReqBuilder{}.
				WithSendTime(5).
				WithPID(1).
				WithVAddr(uint64(0x100)).
				WithDeviceID(1).
				Build()
		})

		It("should respond to top", func() {
			topPort.EXPECT().Peek().Return(req)
			topPort.EXPECT().Retrieve(gomock.Any())
			topPort.EXPECT().Send(gomock.Any())

			set.EXPECT().Visit(wayID)

			madeProgress := tlb.lookup(10)

			Expect(madeProgress).To(BeTrue())
		})

		It("should stall if cannot send to top", func() {
			topPort.EXPECT().Peek().Return(req)
			topPort.EXPECT().Send(gomock.Any()).
				Return(&sim.SendError{})

			madeProgress := tlb.lookup(10)

			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("miss", func() {
		var (
			wayID int
			page  vm.Page
			req   *vm.TranslationReq
		)

		BeforeEach(func() {
			wayID = 1
			page = vm.Page{
				PID:   1,
				VAddr: 0x100,
				PAddr: 0x200,
				Valid: false,
			}
			set.EXPECT().
				Lookup(vm.PID(1), uint64(0x100)).
				Return(wayID, page, true).
				AnyTimes()

			req = vm.TranslationReqBuilder{}.
				WithSendTime(5).
				WithPID(1).
				WithVAddr(0x100).
				WithDeviceID(1).
				Build()
		})

		It("should fetch from bottom and add entry to MSHR", func() {
			topPort.EXPECT().Peek().Return(req)
			topPort.EXPECT().Retrieve(gomock.Any())
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(req *vm.TranslationReq) {
					Expect(req.VAddr).To(Equal(uint64(0x100)))
					Expect(req.PID).To(Equal(vm.PID(1)))
					Expect(req.DeviceID).To(Equal(uint64(1)))
				}).
				Return(nil)

			madeProgress := tlb.lookup(10)

			Expect(madeProgress).To(BeTrue())
			Expect(tlb.mshr.IsEntryPresent(vm.PID(1), uint64(0x100))).To(Equal(true))
		})

		It("should find the entry in MSHR and not request from bottom", func() {
			tlb.mshr.Add(1, 0x100)
			topPort.EXPECT().Peek().Return(req)
			topPort.EXPECT().Retrieve(gomock.Any())

			madeProgress := tlb.lookup(10)
			Expect(tlb.mshr.IsEntryPresent(vm.PID(1), uint64(0x100))).
				To(Equal(true))
			Expect(madeProgress).To(BeTrue())
		})

		It("should stall if bottom is busy", func() {
			topPort.EXPECT().Peek().Return(req)
			bottomPort.EXPECT().Send(gomock.Any()).
				Return(&sim.SendError{})

			madeProgress := tlb.lookup(10)

			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("parse bottom", func() {
		var (
			wayID       int
			req         *vm.TranslationReq
			fetchBottom *vm.TranslationReq
			page        vm.Page
			rsp         *vm.TranslationRsp
		)

		BeforeEach(func() {
			wayID = 1
			req = vm.TranslationReqBuilder{}.
				WithSendTime(5).
				WithPID(1).
				WithVAddr(0x100).
				WithDeviceID(1).
				Build()
			fetchBottom = vm.TranslationReqBuilder{}.
				WithSendTime(5).
				WithPID(1).
				WithVAddr(0x100).
				WithDeviceID(1).
				Build()
			page = vm.Page{
				PID:   1,
				VAddr: 0x100,
				PAddr: 0x200,
				Valid: true,
			}
			rsp = vm.TranslationRspBuilder{}.
				WithSendTime(5).
				WithRspTo(fetchBottom.ID).
				WithPage(page).
				Build()
		})

		It("should do nothing if no return", func() {
			bottomPort.EXPECT().Peek().Return(nil)

			madeProgress := tlb.parseBottom(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if the TLB is responding to an MSHR entry", func() {
			mshrEntry := tlb.mshr.Add(1, 0x100)
			mshrEntry.Requests = append(mshrEntry.Requests, req)
			tlb.respondingMSHREntry = mshrEntry

			madeProgress := tlb.parseBottom(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should parse respond from bottom", func() {
			bottomPort.EXPECT().Peek().Return(rsp)
			bottomPort.EXPECT().Retrieve(gomock.Any())
			mshrEntry := tlb.mshr.Add(1, 0x100)
			mshrEntry.Requests = append(mshrEntry.Requests, req)
			mshrEntry.reqToBottom = &vm.TranslationReq{}

			set.EXPECT().Evict().Return(wayID, true)
			set.EXPECT().Update(wayID, page)
			set.EXPECT().Visit(wayID)

			// topPort.EXPECT().Send(gomock.Any()).
			// 	Do(func(rsp *vm.TranslationRsp) {
			// 		Expect(rsp.Page).To(Equal(page))
			// 		Expect(rsp.RespondTo).To(Equal(req.ID))
			// 	})

			madeProgress := tlb.parseBottom(10)

			Expect(madeProgress).To(BeTrue())
			Expect(tlb.respondingMSHREntry).NotTo(BeNil())
			Expect(tlb.mshr.IsEntryPresent(vm.PID(1), uint64(0x100))).
				To(Equal(false))
		})

		It("should respond", func() {
			mshrEntry := tlb.mshr.Add(1, 0x100)
			mshrEntry.Requests = append(mshrEntry.Requests, req)
			tlb.respondingMSHREntry = mshrEntry

			topPort.EXPECT().Send(gomock.Any()).Return(nil)

			madeProgress := tlb.respondMSHREntry(10)

			Expect(madeProgress).To(BeTrue())
			Expect(mshrEntry.Requests).To(HaveLen(0))
			Expect(tlb.respondingMSHREntry).To(BeNil())
		})
	})

	Context("flush related handling", func() {
		var (
		// flushReq   *TLBFlushReq
		// restartReq *TLBRestartReq
		)

		BeforeEach(func() {

			// restartReq = TLBRestartReqBuilder{}.
			// 	WithSrc(nil).
			// 	WithDst(nil).
			// 	WithSendTime(10).
			// 	Build()
		})

		It("should do nothing if no req", func() {
			controlPort.EXPECT().Peek().Return(nil)
			madeProgress := tlb.performCtrlReq(10)
			Expect(madeProgress).To(BeFalse())
		})

		It("should handle flush request", func() {
			flushReq := FlushReqBuilder{}.
				WithSrc(nil).
				WithDst(nil).
				WithSendTime(10).
				WithVAddrs([]uint64{0x1000}).
				WithPID(1).
				Build()
			page := vm.Page{
				PID:   1,
				VAddr: 0x1000,
				Valid: true,
			}
			wayID := 1

			set.EXPECT().Lookup(vm.PID(1), uint64(0x1000)).
				Return(wayID, page, true)
			set.EXPECT().Update(wayID, vm.Page{
				PID:   1,
				VAddr: 0x1000,
				Valid: false,
			})
			controlPort.EXPECT().Peek().Return(flushReq)
			controlPort.EXPECT().Retrieve(sim.VTimeInSec(10)).Return(flushReq)
			controlPort.EXPECT().Send(gomock.Any())

			madeProgress := tlb.performCtrlReq(10)

			Expect(madeProgress).To(BeTrue())
			Expect(tlb.isPaused).To(BeTrue())
		})

		It("should handle restart request", func() {
			restartReq := RestartReqBuilder{}.
				WithSrc(nil).
				WithDst(nil).
				WithSendTime(10).
				Build()
			controlPort.EXPECT().Peek().
				Return(restartReq)
			controlPort.EXPECT().Retrieve(sim.VTimeInSec(10)).
				Return(restartReq)
			controlPort.EXPECT().Send(gomock.Any())
			topPort.EXPECT().Retrieve(gomock.Any()).Return(nil)
			bottomPort.EXPECT().Retrieve(gomock.Any()).Return(nil)

			madeProgress := tlb.performCtrlReq(10)

			Expect(madeProgress).To(BeTrue())
			Expect(tlb.isPaused).To(BeFalse())
		})
	})
})

var _ = Describe("TLB Integration", func() {
	var (
		mockCtrl   *gomock.Controller
		engine     sim.Engine
		tlb        *TLB
		lowModule  *MockPort
		agent      *MockPort
		connection sim.Connection
		page       vm.Page
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = sim.NewSerialEngine()
		lowModule = NewMockPort(mockCtrl)
		agent = NewMockPort(mockCtrl)
		connection = sim.NewDirectConnection("Conn", engine, 1*sim.GHz)
		tlb = MakeBuilder().WithEngine(engine).Build("TLB")
		tlb.LowModule = lowModule

		agent.EXPECT().SetConnection(connection)
		lowModule.EXPECT().SetConnection(connection)
		connection.PlugIn(agent, 10)
		connection.PlugIn(lowModule, 10)
		connection.PlugIn(tlb.topPort, 10)
		connection.PlugIn(tlb.bottomPort, 10)
		connection.PlugIn(tlb.controlPort, 10)

		page = vm.Page{
			PID:   1,
			VAddr: 0x1000,
			PAddr: 0x2000,
			Valid: true,
		}
		lowModule.EXPECT().Recv(gomock.Any()).
			Do(func(req *vm.TranslationReq) {
				rsp := vm.TranslationRspBuilder{}.
					WithSendTime(req.RecvTime + 1).
					WithSrc(lowModule).
					WithDst(req.Src).
					WithPage(page).
					WithRspTo(req.ID).
					Build()
				connection.Send(rsp)
			}).
			AnyTimes()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do tlb miss", func() {
		req := vm.TranslationReqBuilder{}.
			WithSendTime(10).
			WithSrc(agent).
			WithDst(tlb.topPort).
			WithPID(1).
			WithVAddr(0x1000).
			WithDeviceID(1).
			Build()
		req.RecvTime = 10
		tlb.topPort.Recv(req)

		agent.EXPECT().Recv(gomock.Any()).
			Do(func(rsp *vm.TranslationRsp) {
				Expect(rsp.Page).To(Equal(page))
			})

		engine.Run()
	})

	It("should have faster hit than miss", func() {
		time1 := sim.VTimeInSec(10)
		req := vm.TranslationReqBuilder{}.
			WithSendTime(time1).
			WithSrc(agent).
			WithDst(tlb.topPort).
			WithPID(1).
			WithVAddr(0x1000).
			WithDeviceID(1).
			Build()
		req.RecvTime = time1
		tlb.topPort.Recv(req)

		agent.EXPECT().Recv(gomock.Any()).
			Do(func(rsp *vm.TranslationRsp) {
				Expect(rsp.Page).To(Equal(page))
			})

		engine.Run()

		time2 := engine.CurrentTime()

		req.RecvTime = time2
		tlb.topPort.Recv(req)

		agent.EXPECT().Recv(gomock.Any()).
			Do(func(rsp *vm.TranslationRsp) {
				Expect(rsp.Page).To(Equal(page))
			})

		engine.Run()

		time3 := engine.CurrentTime()

		Expect(time3 - time2).To(BeNumerically("<", time2-time1))
	})

	/*It("should have miss after shootdown ", func() {
		time1 := sim.VTimeInSec(10)
		req := vm.NewTranslationReq(time1, agent, tlb.TopPort, 1, 0x1000, 1)
		req.SetRecvTime(time1)
		tlb.TopPort.Recv(*req)
		agent.EXPECT().Recv(gomock.Any()).
			Do(func(rsp vm.TranslationReadyRsp) {
				Expect(rsp.Page).To(Equal(&page))
			})
		engine.Run()

		time2 := engine.CurrentTime()
		shootdownReq := vm.NewPTEInvalidationReq(
			time2, agent, tlb.ControlPort, 1, []uint64{0x1000})
		shootdownReq.SetRecvTime(time2)
		tlb.ControlPort.Recv(*shootdownReq)
		agent.EXPECT().Recv(gomock.Any()).
			Do(func(rsp vm.InvalidationCompleteRsp) {
				Expect(rsp.RespondTo).To(Equal(shootdownReq.ID))
			})
		engine.Run()

		time3 := engine.CurrentTime()
		req.SetRecvTime(time3)
		tlb.TopPort.Recv(*req)
		agent.EXPECT().Recv(gomock.Any()).
			Do(func(rsp vm.TranslationReadyRsp) {
				Expect(rsp.Page).To(Equal(&page))
			})
		engine.Run()
		time4 := engine.CurrentTime()

		Expect(time4 - time3).To(BeNumerically("~", time2-time1))
	})*/

})
