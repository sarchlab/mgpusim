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

		engine        *MockEngine
		rdmaEngine    *Engine
		toInside      *MockPort
		toOutside     *MockPort
		localModules  *cache.SingleLowModuleFinder
		remoteModules *cache.SingleLowModuleFinder
		localCache    *MockPort
		remoteGPU     *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)
		localCache = NewMockPort(mockCtrl)
		remoteGPU = NewMockPort(mockCtrl)
		localModules = new(cache.SingleLowModuleFinder)
		localModules.LowModule = localCache
		remoteModules = new(cache.SingleLowModuleFinder)
		remoteModules.LowModule = remoteGPU

		rdmaEngine = NewEngine("RDMAEngine", engine, localModules, remoteModules)

		toInside = NewMockPort(mockCtrl)
		toOutside = NewMockPort(mockCtrl)
		rdmaEngine.ToInside = toInside
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
			toInside.EXPECT().Peek().Return(read)
			toOutside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.ReadReq{})).
				Return(nil)
			toInside.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(read)

			rdmaEngine.processFromInside(10)

			Expect(rdmaEngine.transactionsFromInside).To(HaveLen(1))
		})

		It("should wait if outside connection is busy", func() {
			toInside.EXPECT().Peek().Return(read)
			toOutside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.ReadReq{})).
				Return(akita.NewSendError())

			rdmaEngine.processFromInside(10)

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
			toInside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.ReadReq{})).
				Return(nil)
			toOutside.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(read)

			rdmaEngine.processFromOutside(10)

			Expect(rdmaEngine.transactionsFromOutside).To(HaveLen(1))
		})

		It("should wait if outside connection is busy", func() {
			toOutside.EXPECT().Peek().Return(read)
			toInside.EXPECT().
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
				WithDst(rdmaEngine.ToInside).
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
			toInside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.DataReadyRsp{})).
				Return(nil)
			toOutside.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(read)

			rdmaEngine.processFromOutside(10)

			Expect(rdmaEngine.transactionsFromInside).To(HaveLen(0))
		})

		It("should send rsp to inside", func() {
			toOutside.EXPECT().Peek().Return(rsp)
			toInside.EXPECT().
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
				WithDst(rdmaEngine.ToInside).
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

		It("should send rsp to inside", func() {
			toInside.EXPECT().Peek().Return(rsp)
			toOutside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.DataReadyRsp{})).
				Return(nil)
			toInside.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(read)

			rdmaEngine.processFromInside(10)

			Expect(rdmaEngine.transactionsFromOutside).To(HaveLen(0))
		})

		It("should send rsp to inside", func() {
			toInside.EXPECT().Peek().Return(rsp)
			toOutside.EXPECT().
				Send(gomock.AssignableToTypeOf(&mem.DataReadyRsp{})).
				Return(akita.NewSendError())

			rdmaEngine.processFromInside(10)

			Expect(rdmaEngine.transactionsFromOutside).To(HaveLen(1))
		})
	})
})
