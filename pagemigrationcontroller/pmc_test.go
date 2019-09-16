package pagemigrationcontroller



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
//A PMC is a component that handles page migration transfers from one device to another.
func TestPMC(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "PMC")
}

var _ = Describe("PMC", func() {
	var (
		mockCtrl *gomock.Controller

		engine        *MockEngine
		pmc  *PageMigrationController
		RemotePort     *MockPort
		LocalMemPort     *MockPort
		memCtrl       *MockPort
		ctrlPort          *MockPort
		localModules  *cache.SingleLowModuleFinder
		remoteModules *cache.SingleLowModuleFinder
		memCtrlFinder *cache.SingleLowModuleFinder
		localCache    *MockPort
		remoteGPU     *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)
		localCache = NewMockPort(mockCtrl)
		remoteGPU = NewMockPort(mockCtrl)
		memCtrl = NewMockPort(mockCtrl)
		localModules = new(cache.SingleLowModuleFinder)
		localModules.LowModule = localCache
		remoteModules = new(cache.SingleLowModuleFinder)
		remoteModules.LowModule = remoteGPU
		memCtrlFinder = new(cache.SingleLowModuleFinder)
		memCtrlFinder.LowModule = memCtrl

		pmc = NewPageMigrationController("PMC", engine, memCtrlFinder)

		RemotePort = NewMockPort(mockCtrl)
		ctrlPort = NewMockPort(mockCtrl)
		LocalMemPort = NewMockPort(mockCtrl)
		pmc.RemotePort = RemotePort
		pmc.CtrlPort = ctrlPort
		pmc.LocalMemPort = LocalMemPort
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})



	Context("should handle page migration req", func() {
		It("should receive a page migraiton req from Control port", func() {
			req := PageMigrationReqToPMCBuilder{}. 
				   WithSendTime(10).
				   WithSrc(nil).
				   WithDst(pmc.CtrlPort).
				   WithPageSize(4*mem.KB).
				   Build()

			ctrlPort.EXPECT().Retrieve(akita.VTimeInSec(11)).Return(req)

			pmc.processFromCtrlPort(11)

			Expect(pmc.currentMigrationRequest).To(BeEquivalentTo(req))
			Expect(pmc.needTick).To(BeTrue())

		})

		It("should process the page migration req from Control Port", func() {
			req := PageMigrationReqToPMCBuilder{}.
				WithSendTime(10).
				WithSrc(nil).
				WithDst(pmc.CtrlPort).
				WithPageSize(4*mem.KB).
				Build()


			pmc.currentMigrationRequest = req

			pmc.processPageMigrationReqFromCtrlPort(11)

			Expect(pmc.toPullFromAnotherPMC).ToNot(BeNil())
			Expect(pmc.needTick).To(BeTrue())

		})

		It("should send a migration req to another PMC", func() {

			req := DataPullReqBuilder{}.
				   WithSendTime(10).
				   WithSrc(pmc.RemotePort).
				   WithDst(nil).
				   WithReadFromPhyAddress(0x100).
				   WithDataTransferSize(256).
				   Build()

			pmc.toPullFromAnotherPMC = append(pmc.toPullFromAnotherPMC, req)

			RemotePort.EXPECT().Send(req).Return(nil)

			pmc.sendMigrationReqToAnotherPMC(11)

			Expect(pmc.needTick).To(BeTrue())
		})

		It("should receive a data request for page migration from another PMC", func() {

			req := DataPullReqBuilder{}.
				WithSendTime(10).
				WithSrc(pmc.RemotePort).
				WithDst(nil).
				WithReadFromPhyAddress(0x100).
				WithDataTransferSize(256).
				Build()

			RemotePort.EXPECT().Peek().Return(req)
			RemotePort.EXPECT().Retrieve(akita.VTimeInSec(11)).Return(req)

			pmc.processFromOutside(11)

			Expect(pmc.needTick).To(BeTrue())

		})

		It("process a read page req from another PMC", func() {
			req := DataPullReqBuilder{}.
				WithSendTime(10).
				WithSrc(pmc.RemotePort).
				WithDst(nil).
				WithReadFromPhyAddress(0x100).
				WithDataTransferSize(256).
				Build()
			pmc.currentPullReqFromAnotherPMC = append(pmc.currentPullReqFromAnotherPMC, req)

			pmc.processReadPageReqFromAnotherPMC(11)

			Expect(pmc.toSendLocalMemPort).ToNot(BeNil())
			Expect(pmc.toSendLocalMemPort[0].Address).To(BeEquivalentTo(uint64(0x100)))
			Expect(pmc.toSendLocalMemPort[0].AccessByteSize).To(BeEquivalentTo(uint64(0x100)))
			Expect(pmc.toSendLocalMemPort[0].ID).To(BeEquivalentTo(req.ID))

			Expect(pmc.needTick).To(BeTrue())

		})
		It("send the data request from page migration to MemCtrl", func() {
			req := mem.ReadReqBuilder{}.
				 WithSendTime(10).
				 WithSrc(pmc.LocalMemPort).
				 WithDst(pmc.MemCtrlFinder.Find(0x100)).
				WithAddress(0x100).
				  WithByteSize(0x04).
				  Build()
			pmc.toSendLocalMemPort = append(pmc.toSendLocalMemPort, req)

			LocalMemPort.EXPECT().Send(req).Return(nil)

			pmc.sendReadReqLocalMemPort(11)

			Expect(pmc.needTick).To(BeTrue())

		})

		It("should receive a data ready rsp from MemCtrl", func() {
			req := mem.DataReadyRspBuilder{}.
				   WithSendTime(10).
				   WithSrc(nil).
				   WithDst(pmc.LocalMemPort).
				   Build()

			LocalMemPort.EXPECT().Retrieve(akita.VTimeInSec(11)).Return(req)

			pmc.processFromMemCtrl(11)

			Expect(pmc.needTick).To(BeTrue())

		})

		It("process a data ready rsp from Mem Ctrl", func() {
			data := make([]byte, 0)
			data = append(data, 0x04)
			req := mem.DataReadyRspBuilder{}.
				WithSendTime(10).
				WithSrc(nil).
				WithDst(pmc.LocalMemPort).
				WithData(data).
				Build()

			pmc.dataReadyRspFromMemCtrl = append(pmc.dataReadyRspFromMemCtrl, req)

			pmc.processDataReadyRspFromMemCtrl(11)

			Expect(pmc.toRspToAnotherPMC[0]).ToNot(BeNil())
			Expect(pmc.toRspToAnotherPMC[0].Data).To(HaveLen(1))
			Expect(pmc.toRspToAnotherPMC[0].Data[0]).To(BeEquivalentTo(uint64(0x4)))

		})

		It("should send a data ready rsp to requesting PMC", func() {
			data := make([]byte, 0)
			data = append(data, 0x04)
			req := DataPullRspBuilder{}.
				   WithSendTime(10).
				   WithSrc(pmc.RemotePort).
				   WithDst(nil).
				   WithData(data).
				   Build()

			pmc.toRspToAnotherPMC = append(pmc.toRspToAnotherPMC, req)

			RemotePort.EXPECT().Send(req).Return(nil)

			pmc.sendDataReadyRspToRequestingPMC(11)

			Expect(pmc.needTick).To(BeTrue())
			Expect(len(pmc.toRspToAnotherPMC)).To(BeEquivalentTo(0))

		})

		It("should receive a data ready rsp from the requested PMC", func() {
			data := []byte{1, 2, 3, 4}
			req := DataPullRspBuilder{}.
				WithSendTime(10).
				WithSrc(pmc.RemotePort).
				WithDst(nil).
				WithData(data).
				Build()

			RemotePort.EXPECT().Peek().Return(req)
			RemotePort.EXPECT().Retrieve(akita.VTimeInSec(11)).Return(req)

			pmc.processFromOutside(11)

			Expect(pmc.needTick).To(BeTrue())

		})

		It("should process a data migration rsp from requested PMC", func() {
			data := []byte{1, 2}
			migrationReq := PageMigrationReqToPMCBuilder{}.
				            WithSendTime(10).
				            WithSrc(nil).
				            WithDst(pmc.CtrlPort).
				            WithPageSize(4*mem.KB).
				            Build()

			req := DataPullRspBuilder{}.
				WithSendTime(10).
				WithSrc(pmc.RemotePort).
				WithDst(nil).
				WithData(data).
				Build()

			pmc.reqIDToWriteAddressMap[req.ID]=0x100



			pmc.currentMigrationRequest = migrationReq
			pmc.receivedDataFromAnothePMC = append(pmc.receivedDataFromAnothePMC, req)

			pmc.processDataPullRsp(11)

			Expect(pmc.needTick).To(BeTrue())
			Expect(pmc.writeReqLocalMemPort[0].Data).To(BeEquivalentTo(data))

		})
		It("should send a write req to mem ctrl", func() {
			data := []byte{1, 2}
			req := mem.WriteReqBuilder{}.
				   WithSendTime(10).
				   WithSrc(pmc.LocalMemPort).
				   WithDst(pmc.MemCtrlFinder.Find(0X100)).
				   WithAddress(0x100).
				   WithData(data).
				   Build()


			pmc.writeReqLocalMemPort = append(pmc.writeReqLocalMemPort, req)

			LocalMemPort.EXPECT().Send(req).Return(nil)

			pmc.sendWriteReqLocalMemPort(11)

			Expect(pmc.needTick).To(BeTrue())
			Expect(pmc.toSendLocalMemPort).To(BeNil())

		})
		It("should process a write done rsp from memctrl", func() {
			req := mem.WriteDoneRspBuilder{}.
				   WithSendTime(10).
				   WithSrc(nil).
				   WithDst(pmc.LocalMemPort).
				   WithRspTo("xx").
				   Build()

			pmc.receivedWriteDoneFromMemCtrl = req

			pmc.numDataRspPendingForPageMigration = 10

			pmc.processWriteDoneRspFromMemCtrl(11)

			Expect(pmc.needTick).To(BeTrue())
			Expect(pmc.numDataRspPendingForPageMigration).ToNot(BeNil())
			Expect(pmc.numDataRspPendingForPageMigration).To(BeEquivalentTo(uint64(9)))

		})

		It("should receive the last pending data for the page and prepare response for CP", func() {
			req := mem.WriteDoneRspBuilder{}.
				WithSendTime(10).
				WithSrc(nil).
				WithDst(pmc.LocalMemPort).
				WithRspTo("xx").
				Build()
			pageMigrationReq := PageMigrationReqToPMCBuilder{}.
				WithSendTime(10).
				WithSrc(nil).
				WithDst(pmc.CtrlPort).
				WithPageSize(4*mem.KB).
				Build()
			pmc.currentMigrationRequest = pageMigrationReq


			pmc.receivedWriteDoneFromMemCtrl = req

			pmc.numDataRspPendingForPageMigration = 1

			pmc.processWriteDoneRspFromMemCtrl(11)

			Expect(pmc.needTick).To(BeTrue())
			Expect(pmc.numDataRspPendingForPageMigration).ToNot(BeNil())
			Expect(pmc.numDataRspPendingForPageMigration).To(BeEquivalentTo(-1))
			Expect(pmc.toSendToCtrlPort).ToNot(BeNil())

		})

		It("should send migration complete rsp to CP", func() {
			req := PageMigrationRspFromPMCBuilder{}.
				   WithSendTime(10).
				   WithSrc(pmc.CtrlPort).
				   WithDst(nil).
				   Build()

			pmc.toSendToCtrlPort = req
			pmc.isHandlingPageMigration = true

			ctrlPort.EXPECT().Send(req).Return(nil)

			pmc.sendMigrationCompleteRspToCtrlPort(11)

			Expect(pmc.needTick).To(BeTrue())
			Expect(pmc.isHandlingPageMigration).To(BeFalse())

		})

	})
})

