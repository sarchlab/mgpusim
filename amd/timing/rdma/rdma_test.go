package rdma

import (
	"log"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
	"go.uber.org/mock/gomock"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/sim Port,Engine

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
		RDMARequestInside    *MockPort
		RDMADataInside       *MockPort
		ctrlPort             *MockPort
		RDMARequestOutside   *MockPort
		RDMADataOutside      *MockPort
		localModules         *mem.SinglePortMapper
		remoteModules        *mem.SinglePortMapper
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
		localCache.EXPECT().AsRemote().AnyTimes()
		controllingComponent.EXPECT().AsRemote().AnyTimes()
		remoteGPU.EXPECT().AsRemote().AnyTimes()
		localModules = new(mem.SinglePortMapper)
		localModules.Port = localCache.AsRemote()
		remoteModules = new(mem.SinglePortMapper)
		remoteModules.Port = remoteGPU.AsRemote()

		// rdmaEngine = NewEngine("RDMAEngine", engine, localModules, remoteModules)
		rdmaEngine = MakeBuilder().
			WithEngine(engine).
			WithLocalModules(localModules).
			WithRemoteModules(remoteModules).
			Build("RDMAEngine")

		RDMARequestInside = NewMockPort(mockCtrl)
		RDMADataInside = NewMockPort(mockCtrl)
		ctrlPort = NewMockPort(mockCtrl)
		RDMARequestOutside = NewMockPort(mockCtrl)
		RDMADataOutside = NewMockPort(mockCtrl)
		rdmaEngine.RDMARequestInside = RDMARequestInside
		rdmaEngine.RDMADataInside = RDMADataInside
		rdmaEngine.CtrlPort = ctrlPort
		RDMARequestInside.EXPECT().AsRemote().AnyTimes()
		RDMADataInside.EXPECT().AsRemote().AnyTimes()
		ctrlPort.EXPECT().AsRemote().AnyTimes()
		RDMARequestOutside.EXPECT().AsRemote().AnyTimes()
		RDMADataOutside.EXPECT().AsRemote().AnyTimes()

		rdmaEngine.RDMARequestOutside = RDMARequestOutside
		rdmaEngine.RDMADataOutside = RDMADataOutside
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("Read from inside", func() {
		var read *mem.ReadReq

		BeforeEach(func() {
			read = &mem.ReadReq{Address: 0x100, AccessByteSize: 64}
			read.ID = sim.GetIDGenerator().Generate()
			read.Src = localCache.AsRemote()
			read.Dst = rdmaEngine.RDMARequestOutside.AsRemote()
		})

		It("should send read to outside", func() {
			RDMARequestInside.EXPECT().PeekIncoming().Return(read)
			RDMARequestOutside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.ReadReq{})).
				Return(nil)
			RDMARequestInside.EXPECT().RetrieveIncoming().Return(read)
			RDMARequestInside.EXPECT().PeekIncoming().Return(nil)

			rdmaEngine.processFromL1()

			Expect(rdmaEngine.transactionsFromInside).To(HaveLen(1))
		})

		It("should wait if outside connection is busy", func() {
			RDMARequestInside.EXPECT().PeekIncoming().Return(read)
			RDMARequestOutside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.ReadReq{})).
				Return(sim.NewSendError())

			rdmaEngine.processFromL1()

			Expect(rdmaEngine.transactionsFromInside).To(HaveLen(0))
		})
	})

	Context("Read from outside", func() {
		var read *mem.ReadReq

		BeforeEach(func() {
			read = &mem.ReadReq{Address: 0x100, AccessByteSize: 64}
			read.ID = sim.GetIDGenerator().Generate()
			read.Src = localCache.AsRemote()
			read.Dst = rdmaEngine.RDMADataOutside.AsRemote()
		})

		It("should send read to outside", func() {
			RDMADataOutside.EXPECT().PeekIncoming().Return(read)
			RDMADataInside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.ReadReq{})).
				Return(nil)
			RDMADataOutside.EXPECT().RetrieveIncoming().Return(read)

			rdmaEngine.processIncomingReq()

			Expect(rdmaEngine.transactionsFromOutside).To(HaveLen(1))
		})

		It("should wait if outside connection is busy", func() {
			RDMADataOutside.EXPECT().PeekIncoming().Return(read)
			RDMADataInside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.ReadReq{})).
				Return(sim.NewSendError())

			rdmaEngine.processIncomingReq()

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
			readFromInside = &mem.ReadReq{Address: 0x100, AccessByteSize: 64}
			readFromInside.ID = sim.GetIDGenerator().Generate()
			readFromInside.Src = localCache.AsRemote()
			readFromInside.Dst = rdmaEngine.RDMARequestInside.AsRemote()

			read = &mem.ReadReq{Address: 0x100, AccessByteSize: 64}
			read.ID = sim.GetIDGenerator().Generate()
			read.Src = rdmaEngine.RDMARequestOutside.AsRemote()
			read.Dst = remoteGPU.AsRemote()

			rsp = &mem.DataReadyRsp{}
			rsp.ID = sim.GetIDGenerator().Generate()
			rsp.Src = remoteGPU.AsRemote()
			rsp.Dst = rdmaEngine.RDMARequestOutside.AsRemote()
			rsp.RspTo = read.ID

			rdmaEngine.transactionsFromInside = append(
				rdmaEngine.transactionsFromInside,
				transaction{
					fromInside: readFromInside,
					toOutside:  read,
				})
		})

		It("should send rsp to inside", func() {
			RDMARequestOutside.EXPECT().PeekIncoming().Return(rsp)
			RDMARequestInside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.DataReadyRsp{})).
				Return(nil)
			RDMARequestOutside.EXPECT().RetrieveIncoming().Return(read)

			rdmaEngine.processIncomingRsp()

			Expect(rdmaEngine.transactionsFromInside).To(HaveLen(0))
		})

		It("should not send rsp to inside if busy", func() {
			RDMARequestOutside.EXPECT().PeekIncoming().Return(rsp)
			RDMARequestInside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.DataReadyRsp{})).
				Return(sim.NewSendError())

			rdmaEngine.processIncomingRsp()

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
			readFromOutside = &mem.ReadReq{Address: 0x100, AccessByteSize: 64}
			readFromOutside.ID = sim.GetIDGenerator().Generate()
			readFromOutside.Src = localCache.AsRemote()
			readFromOutside.Dst = rdmaEngine.RDMADataInside.AsRemote()

			read = &mem.ReadReq{Address: 0x100, AccessByteSize: 64}
			read.ID = sim.GetIDGenerator().Generate()
			read.Src = rdmaEngine.RDMADataOutside.AsRemote()
			read.Dst = remoteGPU.AsRemote()

			rsp = &mem.DataReadyRsp{}
			rsp.ID = sim.GetIDGenerator().Generate()
			rsp.Src = remoteGPU.AsRemote()
			rsp.Dst = rdmaEngine.RDMADataOutside.AsRemote()
			rsp.RspTo = read.ID
			rdmaEngine.transactionsFromOutside = append(
				rdmaEngine.transactionsFromInside,
				transaction{
					fromOutside: readFromOutside,
					toInside:    read,
				})
		})

		It("should send rsp to outside", func() {
			RDMADataInside.EXPECT().PeekIncoming().Return(rsp)
			RDMADataOutside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.DataReadyRsp{})).
				Return(nil)
			RDMADataInside.EXPECT().RetrieveIncoming().Return(read)

			rdmaEngine.processFromL2()

			Expect(rdmaEngine.transactionsFromOutside).To(HaveLen(0))
		})

		It("should  not send rsp to outside", func() {
			RDMADataInside.EXPECT().PeekIncoming().Return(rsp)
			RDMADataOutside.EXPECT().
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
			read = &mem.ReadReq{Address: 0x100, AccessByteSize: 64}
			read.ID = sim.GetIDGenerator().Generate()
			read.Src = localCache.AsRemote()
			read.Dst = rdmaEngine.RDMARequestOutside.AsRemote()

			drainReq = DrainReqBuilder{}.
				WithSrc(controllingComponent.AsRemote()).
				WithDst(rdmaEngine.CtrlPort.AsRemote()).Build()
			restartReq = RestartReqBuilder{}.
				WithSrc(controllingComponent.AsRemote()).
				WithDst(rdmaEngine.CtrlPort.AsRemote()).Build()

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
