package rdma

import (
	"log"
	"testing"

	"github.com/golang/mock/gomock"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
)

//go:generate mockgen -destination "mock_akita_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/akita Port,Engine

func TestRDMA(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "RDMA")
}

var _ = Describe("Engine", func() {
	var (
		mockCtrl *gomock.Controller

		engine               *MockEngine
		rdmaEngine           *Engine
		toL1                 *MockPort
		toL2                 *MockPort
		ctrlPort             *MockPort
		toOutside            *MockPort
		localModules         *cache.SingleLowModuleFinder
		remoteModules        *cache.SingleLowModuleFinder
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
		localModules = new(cache.SingleLowModuleFinder)
		localModules.LowModule = localCache
		remoteModules = new(cache.SingleLowModuleFinder)
		remoteModules.LowModule = remoteGPU

		rdmaEngine = NewEngine("RDMAEngine", engine, localModules, remoteModules)

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
				WithSendTime(6).
				WithSrc(localCache).
				WithDst(rdmaEngine.ToOutside).
				WithAddress(0x100).
				WithByteSize(64).
				Build()
		})

		It("should send read to outside", func() {
			toL1.EXPECT().Peek().Return(read)
			toOutside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.ReadReq{})).
				Return(nil)
			toL1.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(read)

			rdmaEngine.processFromL1(10)

			Expect(rdmaEngine.transactionsFromInside).To(HaveLen(1))
		})

		It("should wait if outside connection is busy", func() {
			toL1.EXPECT().Peek().Return(read)
			toOutside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.ReadReq{})).
				Return(akita.NewSendError())

			rdmaEngine.processFromL1(10)

			Expect(rdmaEngine.transactionsFromInside).To(HaveLen(0))
		})
	})

	Context("Read from outside", func() {
		var read *mem.ReadReq

		BeforeEach(func() {
			read = mem.ReadReqBuilder{}.
				WithSendTime(6).
				WithSrc(localCache).
				WithDst(rdmaEngine.ToOutside).
				WithAddress(0x100).
				WithByteSize(64).
				Build()
		})

		It("should send read to outside", func() {
			toOutside.EXPECT().Peek().Return(read)
			toL2.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.ReadReq{})).
				Return(nil)
			toOutside.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(read)

			rdmaEngine.processFromOutside(10)

			Expect(rdmaEngine.transactionsFromOutside).To(HaveLen(1))
		})

		It("should wait if outside connection is busy", func() {
			toOutside.EXPECT().Peek().Return(read)
			toL2.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.ReadReq{})).
				Return(akita.NewSendError())

			rdmaEngine.processFromOutside(10)

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
				WithSendTime(4).
				WithSrc(localCache).
				WithDst(rdmaEngine.ToL1).
				WithAddress(0x100).
				WithByteSize(64).
				Build()
			read = mem.ReadReqBuilder{}.
				WithSendTime(6).
				WithSrc(rdmaEngine.ToOutside).
				WithDst(remoteGPU).
				WithAddress(0x100).
				WithByteSize(64).
				Build()
			rsp = mem.DataReadyRspBuilder{}.
				WithSendTime(9).
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
			toOutside.EXPECT().Peek().Return(rsp)
			toL2.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.DataReadyRsp{})).
				Return(nil)
			toOutside.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(read)

			rdmaEngine.processFromOutside(10)

			Expect(rdmaEngine.transactionsFromInside).To(HaveLen(0))
		})

		It("should not send rsp to inside if busy", func() {
			toOutside.EXPECT().Peek().Return(rsp)
			toL2.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.DataReadyRsp{})).
				Return(akita.NewSendError())

			rdmaEngine.processFromOutside(10)

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
				WithSendTime(4).
				WithSrc(localCache).
				WithDst(rdmaEngine.ToL2).
				WithAddress(0x100).
				WithByteSize(64).
				Build()
			read = mem.ReadReqBuilder{}.
				WithSendTime(6).
				WithSrc(rdmaEngine.ToOutside).
				WithDst(remoteGPU).
				WithAddress(0x100).
				WithByteSize(64).
				Build()
			rsp = mem.DataReadyRspBuilder{}.
				WithSendTime(9).
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
			toL2.EXPECT().Peek().Return(rsp)
			toOutside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.DataReadyRsp{})).
				Return(nil)
			toL2.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(read)

			rdmaEngine.processFromL2(10)

			Expect(rdmaEngine.transactionsFromOutside).To(HaveLen(0))
		})

		It("should  not send rsp to outside", func() {
			toL2.EXPECT().Peek().Return(rsp)
			toOutside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.DataReadyRsp{})).
				Return(akita.NewSendError())

			rdmaEngine.processFromL2(10)

			Expect(rdmaEngine.transactionsFromOutside).To(HaveLen(1))
		})
	})
	Context("Drain related handling", func() {

		var (
			read       *mem.ReadReq
			drainReq   *RDMADrainReq
			restartReq *RDMARestartReq
		)

		BeforeEach(func() {
			read = mem.ReadReqBuilder{}.
				WithSendTime(6).
				WithSrc(localCache).
				WithDst(rdmaEngine.ToOutside).
				WithAddress(0x100).
				WithByteSize(64).
				Build()
			drainReq = RDMADrainReqBuilder{}.
				WithSendTime(6).
				WithSrc(controllingComponent).
				WithDst(rdmaEngine.CtrlPort).Build()
			restartReq = RDMARestartReqBuilder{}.
				WithSendTime(6).
				WithSrc(controllingComponent).
				WithDst(rdmaEngine.CtrlPort).Build()

		})

		It("should handle drain req", func() {
			ctrlPort.EXPECT().Peek().Return(drainReq)
			ctrlPort.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(drainReq)

			rdmaEngine.processFromCtrlPort(10)

			Expect(rdmaEngine.currentDrainReq).To(Equal(drainReq))
			Expect(rdmaEngine.isDraining).To(BeTrue())
			Expect(rdmaEngine.pauseIncomingReqsFromL1).To(BeTrue())

		})

		It("should send a drain complete rsp", func() {
			rdmaEngine.currentDrainReq = drainReq
			rdmaEngine.isDraining = true

			ctrlPort.EXPECT().
				Send(gomock.AssignableToTypeOf(&RDMADrainRsp{})).
				Return(nil)
			rdmaEngine.drainRDMA(10)

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

			rdmaEngine.drainRDMA(10)

			Expect(rdmaEngine.isDraining).To(BeTrue())

		})

		It("should handle drain restart req", func() {
			rdmaEngine.currentDrainReq = drainReq
			rdmaEngine.pauseIncomingReqsFromL1 = true

			ctrlPort.EXPECT().Peek().Return(restartReq)
			ctrlPort.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(restartReq)

			rdmaEngine.processFromCtrlPort(10)

			Expect(rdmaEngine.currentDrainReq).To(BeNil())
			Expect(rdmaEngine.pauseIncomingReqsFromL1).To(BeFalse())

		})

	})
})
