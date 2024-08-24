package rdma

import (
	"log"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim Port,Engine

func TestRDMA(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "RDMA")
}

var _ = Describe("Engine", func() {
	var (
		mockCtrl *gomock.Controller

		engine               *MockEngine
		rdmaEngine           *Comp
		toL1                 *MockPort
		toL2                 *MockPort
		ctrlPort             *MockPort
		toOutside            *MockPort
		localModules         *mem.SingleLowModuleFinder
		remoteModules        *mem.SingleLowModuleFinder
		localCache           *MockPort
		remoteGPU            *MockPort
		controllingComponent *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)
		localCache = NewMockPort(mockCtrl)
		controllingComponent = NewMockPort(mockCtrl)
		remoteGPU = NewMockPort(mockCtrl)
		localModules = new(mem.SingleLowModuleFinder)
		localModules.LowModule = localCache
		remoteModules = new(mem.SingleLowModuleFinder)
		remoteModules.LowModule = remoteGPU

		// rdmaEngine = NewEngine("RDMAEngine", engine, localModules, remoteModules)
		rdmaEngine = MakeBuilder().
			WithEngine(engine).
			WithLocalModules(localModules).
			WithRemoteModules(remoteModules).
			Build("RDMAEngine")

		toL1 = NewMockPort(mockCtrl)
		toL2 = NewMockPort(mockCtrl)
		ctrlPort = NewMockPort(mockCtrl)
		toOutside = NewMockPort(mockCtrl)
		rdmaEngine.ToL1 = toL1
		rdmaEngine.ToL2 = toL2
		rdmaEngine.CtrlPort = ctrlPort

		rdmaEngine.ToOutside = toOutside
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("Read from inside", func() {
		var read *mem.ReadReq

		BeforeEach(func() {
			read = mem.ReadReqBuilder{}.
				WithSrc(localCache).
				WithDst(rdmaEngine.ToOutside).
				WithAddress(0x100).
				WithByteSize(64).
				Build()
		})

		It("should send read to outside", func() {
			toL1.EXPECT().PeekIncoming().Return(read)
			toOutside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.ReadReq{})).
				Return(nil)
			toL1.EXPECT().RetrieveIncoming().Return(read)
			toL1.EXPECT().PeekIncoming().Return(nil)

			rdmaEngine.processFromL1()

			Expect(rdmaEngine.transactionsFromInside).To(HaveLen(1))
		})

		It("should wait if outside connection is busy", func() {
			toL1.EXPECT().PeekIncoming().Return(read)
			toOutside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.ReadReq{})).
				Return(sim.NewSendError())

			rdmaEngine.processFromL1()

			Expect(rdmaEngine.transactionsFromInside).To(HaveLen(0))
		})
	})

	Context("Read from outside", func() {
		var read *mem.ReadReq

		BeforeEach(func() {
			read = mem.ReadReqBuilder{}.
				WithSrc(localCache).
				WithDst(rdmaEngine.ToOutside).
				WithAddress(0x100).
				WithByteSize(64).
				Build()
		})

		It("should send read to outside", func() {
			toOutside.EXPECT().PeekIncoming().Return(read)
			toOutside.EXPECT().PeekIncoming().Return(nil)
			toL2.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.ReadReq{})).
				Return(nil)
			toOutside.EXPECT().RetrieveIncoming().Return(read)

			rdmaEngine.processFromOutside()

			Expect(rdmaEngine.transactionsFromOutside).To(HaveLen(1))
		})

		It("should wait if outside connection is busy", func() {
			toOutside.EXPECT().PeekIncoming().Return(read)
			toL2.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.ReadReq{})).
				Return(sim.NewSendError())

			rdmaEngine.processFromOutside()

			Expect(rdmaEngine.transactionsFromInside).To(HaveLen(0))
		})
	})

	Context("DataReady from outside", func() {
		var (
			readFromInside *mem.ReadReq
			read           *mem.ReadReq
			rsp            *mem.DataReadyRsp
		)

		BeforeEach(func() {
			readFromInside = mem.ReadReqBuilder{}.
				WithSrc(localCache).
				WithDst(rdmaEngine.ToL1).
				WithAddress(0x100).
				WithByteSize(64).
				Build()
			read = mem.ReadReqBuilder{}.
				WithSrc(rdmaEngine.ToOutside).
				WithDst(remoteGPU).
				WithAddress(0x100).
				WithByteSize(64).
				Build()
			rsp = mem.DataReadyRspBuilder{}.
				WithSrc(remoteGPU).
				WithDst(rdmaEngine.ToOutside).
				WithRspTo(read.ID).
				Build()

			rdmaEngine.transactionsFromInside = append(
				rdmaEngine.transactionsFromInside,
				transaction{
					fromInside: readFromInside,
					toOutside:  read,
				})
		})

		It("should send rsp to inside", func() {
			toOutside.EXPECT().PeekIncoming().Return(rsp)
			toOutside.EXPECT().PeekIncoming().Return(nil)
			toL1.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.DataReadyRsp{})).
				Return(nil)
			toOutside.EXPECT().RetrieveIncoming().Return(read)

			rdmaEngine.processFromOutside()

			Expect(rdmaEngine.transactionsFromInside).To(HaveLen(0))
		})

		It("should not send rsp to inside if busy", func() {
			toOutside.EXPECT().PeekIncoming().Return(rsp)
			toL1.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.DataReadyRsp{})).
				Return(sim.NewSendError())

			rdmaEngine.processFromOutside()

			Expect(rdmaEngine.transactionsFromInside).To(HaveLen(1))
		})
	})

	Context("DataReady from inside", func() {
		var (
			readFromOutside *mem.ReadReq
			read            *mem.ReadReq
			rsp             *mem.DataReadyRsp
		)

		BeforeEach(func() {
			readFromOutside = mem.ReadReqBuilder{}.
				WithSrc(localCache).
				WithDst(rdmaEngine.ToL2).
				WithAddress(0x100).
				WithByteSize(64).
				Build()
			read = mem.ReadReqBuilder{}.
				WithSrc(rdmaEngine.ToOutside).
				WithDst(remoteGPU).
				WithAddress(0x100).
				WithByteSize(64).
				Build()
			rsp = mem.DataReadyRspBuilder{}.
				WithSrc(remoteGPU).
				WithDst(rdmaEngine.ToOutside).
				WithRspTo(read.ID).
				Build()
			rdmaEngine.transactionsFromOutside = append(
				rdmaEngine.transactionsFromInside,
				transaction{
					fromOutside: readFromOutside,
					toInside:    read,
				})
		})

		It("should send rsp to outside", func() {
			toL2.EXPECT().PeekIncoming().Return(rsp)
			toL2.EXPECT().PeekIncoming().Return(nil)
			toOutside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.DataReadyRsp{})).
				Return(nil)
			toL2.EXPECT().RetrieveIncoming().Return(read)

			rdmaEngine.processFromL2()

			Expect(rdmaEngine.transactionsFromOutside).To(HaveLen(0))
		})

		It("should  not send rsp to outside", func() {
			toL2.EXPECT().PeekIncoming().Return(rsp)
			toOutside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.DataReadyRsp{})).
				Return(sim.NewSendError())

			rdmaEngine.processFromL2()

			Expect(rdmaEngine.transactionsFromOutside).To(HaveLen(1))
		})
	})
	Context("Drain related handling", func() {

		var (
			read       *mem.ReadReq
			drainReq   *DrainReq
			restartReq *RestartReq
		)

		BeforeEach(func() {
			read = mem.ReadReqBuilder{}.
				WithSrc(localCache).
				WithDst(rdmaEngine.ToOutside).
				WithAddress(0x100).
				WithByteSize(64).
				Build()
			drainReq = DrainReqBuilder{}.
				WithSrc(controllingComponent).
				WithDst(rdmaEngine.CtrlPort).Build()
			restartReq = RestartReqBuilder{}.
				WithSrc(controllingComponent).
				WithDst(rdmaEngine.CtrlPort).Build()

		})

		It("should handle drain req", func() {
			ctrlPort.EXPECT().PeekIncoming().Return(drainReq)
			ctrlPort.EXPECT().RetrieveIncoming().Return(drainReq)

			rdmaEngine.processFromCtrlPort()

			Expect(rdmaEngine.currentDrainReq).To(Equal(drainReq))
			Expect(rdmaEngine.isDraining).To(BeTrue())
			Expect(rdmaEngine.pauseIncomingReqsFromL1).To(BeTrue())

		})

		It("should send a drain complete rsp", func() {
			rdmaEngine.currentDrainReq = drainReq
			rdmaEngine.isDraining = true

			ctrlPort.EXPECT().
				Send(gomock.AssignableToTypeOf(&DrainRsp{})).
				Return(nil)
			rdmaEngine.drainRDMA()

			Expect(rdmaEngine.isDraining).To(BeFalse())

		})

		It("should not send a drain complete rsp if transactions pending", func() {
			rdmaEngine.transactionsFromInside = append(
				rdmaEngine.transactionsFromInside,
				transaction{
					fromInside: read,
					toOutside:  read,
				})
			rdmaEngine.currentDrainReq = drainReq
			rdmaEngine.isDraining = true

			rdmaEngine.drainRDMA()

			Expect(rdmaEngine.isDraining).To(BeTrue())

		})

		It("should handle drain restart req", func() {
			rdmaEngine.currentDrainReq = drainReq
			rdmaEngine.pauseIncomingReqsFromL1 = true

			ctrlPort.EXPECT().PeekIncoming().Return(restartReq)
			ctrlPort.EXPECT().RetrieveIncoming().Return(restartReq)
			ctrlPort.EXPECT().
				Send(gomock.AssignableToTypeOf(&RestartRsp{})).
				Return(nil)

			rdmaEngine.processFromCtrlPort()

			Expect(rdmaEngine.currentDrainReq).To(BeNil())
			Expect(rdmaEngine.pauseIncomingReqsFromL1).To(BeFalse())

		})

	})
})
