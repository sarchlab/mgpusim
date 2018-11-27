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

	Context("Read from inside", func() {
		var read *mem.ReadReq

		BeforeEach(func() {
			read = mem.NewReadReq(6,
				localCache.ToOutside, rdmaEngine.ToInside, 0x100, 64)
			// rdmaEngine.ToInside.Recv(read)
		})

		It("should send read to outside", func() {
			expectRead := mem.NewReadReq(10,
				rdmaEngine.ToOutside, remoteGPU.ToOutside, 0x100, 64)
			toInside.EXPECT().Peek().Return(read)
			toOutside.EXPECT().Send(gomock.AssignableToTypeOf(expectRead)).Return(nil)
			toInside.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(read)

			rdmaEngine.processReqFromInside(10)

			Expect(rdmaEngine.originalSrc[read.ID]).To(
				BeIdenticalTo(localCache.ToOutside))
		})

		// It("should wait if outside connection is busy", func() {
		// 	expectRead := mem.NewReadReq(10,
		// 		rdmaEngine.ToOutside, remoteGPU.ToOutside, 0x100, 64)
		// 	outsideConn.ExpectSend(expectRead, akita.NewSendError())

		// 	rdmaEngine.processReqFromInside(10)

		// 	Expect(outsideConn.AllExpectedSent()).To(BeTrue())
		// 	Expect(rdmaEngine.ToInside.Buf).To(HaveLen(1))
		// 	Expect(rdmaEngine.ToInside.Buf[0].Src()).To(
		// 		BeIdenticalTo(localCache.ToOutside))
		// 	Expect(rdmaEngine.ToInside.Buf[0].Dst()).To(
		// 		BeIdenticalTo(rdmaEngine.ToInside))
		// })
	})

	// Context("Write from inside", func() {
	// 	var write *mem.WriteReq

	// 	BeforeEach(func() {
	// 		write = mem.NewWriteReq(6, localCache.ToOutside, rdmaEngine.ToInside, 0x100)
	// 		rdmaEngine.ToInside.Recv(write)
	// 	})

	// 	It("should send write to outside", func() {
	// 		expectRead := mem.NewWriteReq(10,
	// 			rdmaEngine.ToOutside, remoteGPU.ToOutside, 0x100)
	// 		outsideConn.ExpectSend(expectRead, nil)

	// 		rdmaEngine.processReqFromInside(10)

	// 		Expect(outsideConn.AllExpectedSent()).To(BeTrue())
	// 		Expect(rdmaEngine.ToInside.Buf).To(HaveLen(0))
	// 		Expect(rdmaEngine.originalSrc[write.ID]).To(
	// 			BeIdenticalTo(localCache.ToOutside))
	// 	})

	// 	It("should wait if outside connection is busy", func() {
	// 		expectRead := mem.NewWriteReq(10,
	// 			rdmaEngine.ToOutside, remoteGPU.ToOutside, 0x100)
	// 		outsideConn.ExpectSend(expectRead, akita.NewSendError())

	// 		rdmaEngine.processReqFromInside(10)

	// 		Expect(outsideConn.AllExpectedSent()).To(BeTrue())
	// 		Expect(rdmaEngine.ToInside.Buf).To(HaveLen(1))
	// 		Expect(rdmaEngine.ToInside.Buf[0].Src()).To(
	// 			BeIdenticalTo(localCache.ToOutside))
	// 		Expect(rdmaEngine.ToInside.Buf[0].Dst()).To(
	// 			BeIdenticalTo(rdmaEngine.ToInside))
	// 	})
	// })

	// Context("DataReady from inside", func() {
	// 	var (
	// 		read      *mem.ReadReq
	// 		dataReady *mem.DataReadyRsp
	// 	)

	// 	BeforeEach(func() {
	// 		// Send a read from outside
	// 		read = mem.NewReadReq(6,
	// 			remoteGPU.ToOutside, rdmaEngine.ToOutside, 0x100, 64)
	// 		rdmaEngine.ToOutside.Recv(read)
	// 		expectRead := mem.NewReadReq(7,
	// 			rdmaEngine.ToInside, localCache.ToOutside, 0x100, 64)
	// 		insideConn.ExpectSend(expectRead, nil)
	// 		rdmaEngine.processReqFromOutside(7)

	// 		dataReady = mem.NewDataReadyRsp(12,
	// 			localCache.ToOutside, rdmaEngine.ToInside, read.ID)
	// 		rdmaEngine.ToInside.Recv(dataReady)
	// 	})

	// 	It("should send data ready to outside", func() {
	// 		expectDataReady := mem.NewDataReadyRsp(10,
	// 			rdmaEngine.ToOutside, remoteGPU.ToOutside, read.ID)
	// 		outsideConn.ExpectSend(expectDataReady, nil)

	// 		rdmaEngine.processReqFromInside(10)

	// 		Expect(insideConn.AllExpectedSent()).To(BeTrue())
	// 		Expect(rdmaEngine.ToInside.Buf).To(HaveLen(0))
	// 		Expect(rdmaEngine.originalSrc).NotTo(HaveKey(read.ID))
	// 	})

	// 	It("should wait if outside connection is busy", func() {
	// 		expectDataReady := mem.NewDataReadyRsp(10,
	// 			rdmaEngine.ToOutside, remoteGPU.ToOutside, read.ID)
	// 		outsideConn.ExpectSend(expectDataReady, akita.NewSendError())

	// 		rdmaEngine.processReqFromInside(10)

	// 		Expect(insideConn.AllExpectedSent()).To(BeTrue())
	// 		Expect(rdmaEngine.ToInside.Buf).To(HaveLen(1))
	// 		Expect(rdmaEngine.originalSrc).To(HaveKey(read.ID))
	// 		Expect(rdmaEngine.ToInside.Buf[0].Src()).To(
	// 			BeIdenticalTo(localCache.ToOutside))
	// 		Expect(rdmaEngine.ToInside.Buf[0].Dst()).To(
	// 			BeIdenticalTo(rdmaEngine.ToInside))
	// 	})
	// })

	// Context("write-done from inside", func() {
	// 	var (
	// 		write *mem.WriteReq
	// 		done  *mem.DoneRsp
	// 	)

	// 	BeforeEach(func() {
	// 		// Send a write from inside
	// 		write = mem.NewWriteReq(6,
	// 			remoteGPU.ToOutside, rdmaEngine.ToOutside, 0x100)
	// 		rdmaEngine.ToOutside.Recv(write)
	// 		expectWrite := mem.NewWriteReq(7,
	// 			rdmaEngine.ToInside, localCache.ToOutside, 0x100)
	// 		insideConn.ExpectSend(expectWrite, nil)
	// 		rdmaEngine.processReqFromOutside(7)

	// 		done = mem.NewDoneRsp(9,
	// 			localCache.ToOutside, rdmaEngine.ToInside, write.ID)
	// 		rdmaEngine.ToInside.Recv(done)
	// 	})

	// 	It("should send done to outside", func() {
	// 		expectDone := mem.NewDoneRsp(10,
	// 			rdmaEngine.ToOutside, remoteGPU.ToOutside, write.ID)
	// 		outsideConn.ExpectSend(expectDone, nil)

	// 		rdmaEngine.processReqFromInside(10)

	// 		Expect(insideConn.AllExpectedSent()).To(BeTrue())
	// 		Expect(rdmaEngine.ToInside.Buf).To(HaveLen(0))
	// 		Expect(rdmaEngine.originalSrc).NotTo(HaveKey(write.ID))
	// 	})

	// 	It("should wait if outside connection is busy", func() {
	// 		expectDone := mem.NewDoneRsp(10,
	// 			rdmaEngine.ToOutside, remoteGPU.ToOutside, write.ID)
	// 		outsideConn.ExpectSend(expectDone, akita.NewSendError())

	// 		rdmaEngine.processReqFromInside(10)

	// 		Expect(outsideConn.AllExpectedSent()).To(BeTrue())
	// 		Expect(rdmaEngine.ToInside.Buf).To(HaveLen(1))
	// 		Expect(rdmaEngine.originalSrc).To(HaveKey(write.ID))
	// 		Expect(rdmaEngine.ToInside.Buf[0].Src()).To(
	// 			BeIdenticalTo(localCache.ToOutside))
	// 		Expect(rdmaEngine.ToInside.Buf[0].Dst()).To(
	// 			BeIdenticalTo(rdmaEngine.ToInside))
	// 	})
	// })

	// Context("Read from outside", func() {
	// 	var read *mem.ReadReq

	// 	BeforeEach(func() {
	// 		read = mem.NewReadReq(6,
	// 			remoteGPU.ToOutside, rdmaEngine.ToOutside, 0x100, 64)
	// 		rdmaEngine.ToOutside.Recv(read)
	// 	})

	// 	It("should send read to inside", func() {
	// 		expectRead := mem.NewReadReq(10,
	// 			rdmaEngine.ToInside, localCache.ToOutside, 0x100, 64)
	// 		insideConn.ExpectSend(expectRead, nil)

	// 		rdmaEngine.processReqFromOutside(10)

	// 		Expect(insideConn.AllExpectedSent()).To(BeTrue())
	// 		Expect(rdmaEngine.ToOutside.Buf).To(HaveLen(0))
	// 		Expect(rdmaEngine.originalSrc[read.ID]).To(
	// 			BeIdenticalTo(remoteGPU.ToOutside))
	// 	})

	// 	It("should wait if outside connection is busy", func() {
	// 		expectRead := mem.NewReadReq(10,
	// 			rdmaEngine.ToInside, localCache.ToOutside, 0x100, 64)
	// 		insideConn.ExpectSend(expectRead, akita.NewSendError())

	// 		rdmaEngine.processReqFromOutside(10)

	// 		Expect(insideConn.AllExpectedSent()).To(BeTrue())
	// 		Expect(rdmaEngine.ToOutside.Buf).To(HaveLen(1))
	// 		Expect(rdmaEngine.ToOutside.Buf[0].Src()).To(
	// 			BeIdenticalTo(remoteGPU.ToOutside))
	// 		Expect(rdmaEngine.ToOutside.Buf[0].Dst()).To(
	// 			BeIdenticalTo(rdmaEngine.ToOutside))
	// 	})
	// })

	// Context("Write from outside", func() {
	// 	var write *mem.WriteReq

	// 	BeforeEach(func() {
	// 		write = mem.NewWriteReq(6,
	// 			remoteGPU.ToOutside, rdmaEngine.ToOutside, 0x100)
	// 		rdmaEngine.ToOutside.Recv(write)
	// 	})

	// 	It("should send write to inside", func() {
	// 		expectRead := mem.NewWriteReq(10,
	// 			rdmaEngine.ToInside, localCache.ToOutside, 0x100)
	// 		insideConn.ExpectSend(expectRead, nil)

	// 		rdmaEngine.processReqFromOutside(10)

	// 		Expect(insideConn.AllExpectedSent()).To(BeTrue())
	// 		Expect(rdmaEngine.ToOutside.Buf).To(HaveLen(0))
	// 		Expect(rdmaEngine.originalSrc[write.ID]).To(
	// 			BeIdenticalTo(remoteGPU.ToOutside))
	// 	})

	// 	It("should wait if outside connection is busy", func() {
	// 		expectRead := mem.NewWriteReq(10,
	// 			rdmaEngine.ToInside, localCache.ToOutside, 0x100)
	// 		insideConn.ExpectSend(expectRead, akita.NewSendError())

	// 		rdmaEngine.processReqFromOutside(10)

	// 		Expect(insideConn.AllExpectedSent()).To(BeTrue())
	// 		Expect(rdmaEngine.ToOutside.Buf).To(HaveLen(1))
	// 		Expect(rdmaEngine.ToOutside.Buf[0].Src()).To(
	// 			BeIdenticalTo(remoteGPU.ToOutside))
	// 		Expect(rdmaEngine.ToOutside.Buf[0].Dst()).To(
	// 			BeIdenticalTo(rdmaEngine.ToOutside))
	// 	})
	// })

	// Context("DataReady from outside", func() {
	// 	var (
	// 		read      *mem.ReadReq
	// 		dataReady *mem.DataReadyRsp
	// 	)

	// 	BeforeEach(func() {
	// 		// Send a read from inside
	// 		read = mem.NewReadReq(6,
	// 			localCache.ToOutside, rdmaEngine.ToInside, 0x100, 64)
	// 		rdmaEngine.ToInside.Recv(read)

	// 		expectRead := mem.NewReadReq(7,
	// 			rdmaEngine.ToOutside, remoteGPU.ToOutside, 0x100, 64)
	// 		outsideConn.ExpectSend(expectRead, nil)
	// 		rdmaEngine.processReqFromInside(7)

	// 		dataReady = mem.NewDataReadyRsp(12,
	// 			remoteGPU.ToOutside, rdmaEngine.ToOutside, read.ID)
	// 		rdmaEngine.ToOutside.Recv(dataReady)
	// 	})

	// 	It("should send data ready to outside", func() {
	// 		expectDataReady := mem.NewDataReadyRsp(10,
	// 			rdmaEngine.ToInside, localCache.ToOutside, read.ID)
	// 		insideConn.ExpectSend(expectDataReady, nil)

	// 		rdmaEngine.processReqFromOutside(10)

	// 		Expect(insideConn.AllExpectedSent()).To(BeTrue())
	// 		Expect(rdmaEngine.ToOutside.Buf).To(HaveLen(0))
	// 		Expect(rdmaEngine.originalSrc).NotTo(HaveKey(read.ID))
	// 	})

	// 	It("should wait if outside connection is busy", func() {
	// 		expectDataReady := mem.NewDataReadyRsp(10,
	// 			rdmaEngine.ToInside, localCache.ToOutside, read.ID)
	// 		insideConn.ExpectSend(expectDataReady, akita.NewSendError())

	// 		rdmaEngine.processReqFromOutside(10)

	// 		Expect(insideConn.AllExpectedSent()).To(BeTrue())
	// 		Expect(rdmaEngine.ToOutside.Buf).To(HaveLen(1))
	// 		Expect(rdmaEngine.originalSrc).To(HaveKey(read.ID))
	// 		Expect(rdmaEngine.ToOutside.Buf[0].Src()).To(
	// 			BeIdenticalTo(remoteGPU.ToOutside))
	// 		Expect(rdmaEngine.ToOutside.Buf[0].Dst()).To(
	// 			BeIdenticalTo(rdmaEngine.ToOutside))
	// 	})
	// })

	// Context("write-done from outside", func() {
	// 	var (
	// 		write *mem.WriteReq
	// 		done  *mem.DoneRsp
	// 	)

	// 	BeforeEach(func() {
	// 		// Send a write from inside
	// 		write = mem.NewWriteReq(6,
	// 			localCache.ToOutside, rdmaEngine.ToInside, 0x100)
	// 		rdmaEngine.ToInside.Recv(write)
	// 		expectWrite := mem.NewWriteReq(7,
	// 			rdmaEngine.ToOutside, remoteGPU.ToOutside, 0x100)
	// 		outsideConn.ExpectSend(expectWrite, nil)
	// 		rdmaEngine.processReqFromInside(7)

	// 		done = mem.NewDoneRsp(9,
	// 			remoteGPU.ToOutside, rdmaEngine.ToOutside, write.ID)
	// 		rdmaEngine.ToOutside.Recv(done)
	// 	})

	// 	It("should send data ready to outside", func() {
	// 		expectDone := mem.NewDoneRsp(10,
	// 			rdmaEngine.ToInside, localCache.ToOutside, write.ID)
	// 		insideConn.ExpectSend(expectDone, nil)

	// 		rdmaEngine.processReqFromOutside(10)

	// 		Expect(insideConn.AllExpectedSent()).To(BeTrue())
	// 		Expect(rdmaEngine.ToOutside.Buf).To(HaveLen(0))
	// 		Expect(rdmaEngine.originalSrc).NotTo(HaveKey(write.ID))
	// 	})

	// 	It("should wait if outside connection is busy", func() {
	// 		expectDone := mem.NewDoneRsp(10,
	// 			rdmaEngine.ToInside, localCache.ToOutside, write.ID)
	// 		insideConn.ExpectSend(expectDone, akita.NewSendError())

	// 		rdmaEngine.processReqFromOutside(10)

	// 		Expect(insideConn.AllExpectedSent()).To(BeTrue())
	// 		Expect(rdmaEngine.ToOutside.Buf).To(HaveLen(1))
	// 		Expect(rdmaEngine.originalSrc).To(HaveKey(write.ID))
	// 		Expect(rdmaEngine.ToOutside.Buf[0].Src()).To(
	// 			BeIdenticalTo(remoteGPU.ToOutside))
	// 		Expect(rdmaEngine.ToOutside.Buf[0].Dst()).To(
	// 			BeIdenticalTo(rdmaEngine.ToOutside))
	// 	})
	// })
})
