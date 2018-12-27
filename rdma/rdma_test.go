package rdma

import (
	"log"
	"testing"

	"gitlab.com/akita/akita/mock_akita"

	"github.com/golang/mock/gomock"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
)

func TestRDMA(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "RDMA")
}

var _ = Describe("Engine", func() {
	var (
		mockCtrl *gomock.Controller

		engine        akita.Engine
		rdmaEngine    *Engine
		toInside      *mock_akita.MockPort
		toOutside     *mock_akita.MockPort
		localModules  *cache.SingleLowModuleFinder
		remoteModules *cache.SingleLowModuleFinder
		localCache    *akita.MockComponent
		remoteGPU     *akita.MockComponent
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = akita.NewMockEngine()
		localCache = akita.NewMockComponent("LocalCache")
		remoteGPU = akita.NewMockComponent("RemoveGPU")
		localModules = new(cache.SingleLowModuleFinder)
		localModules.LowModule = localCache.ToOutside
		remoteModules = new(cache.SingleLowModuleFinder)
		remoteModules.LowModule = remoteGPU.ToOutside

		rdmaEngine = NewEngine("RDMAEngine", engine, localModules, remoteModules)

		toInside = mock_akita.NewMockPort(mockCtrl)
		toOutside = mock_akita.NewMockPort(mockCtrl)
		rdmaEngine.ToInside = toInside
		rdmaEngine.ToOutside = toOutside
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("Read from inside", func() {
		var read *mem.ReadReq

		BeforeEach(func() {
			read = mem.NewReadReq(6,
				localCache.ToOutside, rdmaEngine.ToInside, 0x100, 64)
			// rdmaEngine.ToInside.Recv(read)
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
			read = mem.NewReadReq(6,
				remoteGPU.ToOutside, rdmaEngine.ToOutside, 0x100, 64)
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
			readFromInside = mem.NewReadReq(4,
				localCache.ToOutside, rdmaEngine.ToInside,
				0x100, 64)
			read = mem.NewReadReq(6,
				rdmaEngine.ToOutside, remoteGPU.ToOutside,
				0x100, 64)
			rsp = mem.NewDataReadyRsp(9,
				remoteGPU.ToOutside,
				rdmaEngine.ToOutside,
				read.GetID())
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
			readFromOutside = mem.NewReadReq(4,
				localCache.ToOutside, rdmaEngine.ToInside,
				0x100, 64)
			read = mem.NewReadReq(6,
				rdmaEngine.ToOutside, remoteGPU.ToOutside,
				0x100, 64)
			rsp = mem.NewDataReadyRsp(9,
				remoteGPU.ToOutside,
				rdmaEngine.ToOutside,
				read.GetID())
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
