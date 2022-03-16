package pagemigrationcontroller

import (
	"log"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/mem/v3/mem"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/akita/v3/sim Port,Engine

func TestPMC(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "PMC")
}

var _ = Describe("PMC", func() {
	var (
		mockCtrl *gomock.Controller

		engine        *MockEngine
		pmc           *PageMigrationController
		RemotePort    *MockPort
		LocalMemPort  *MockPort
		memCtrl       *MockPort
		ctrlPort      *MockPort
		localModules  *mem.SingleLowModuleFinder
		remoteModules *mem.SingleLowModuleFinder
		memCtrlFinder *mem.SingleLowModuleFinder
		localCache    *MockPort
		remoteGPU     *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)
		localCache = NewMockPort(mockCtrl)
		remoteGPU = NewMockPort(mockCtrl)
		memCtrl = NewMockPort(mockCtrl)
		localModules = new(mem.SingleLowModuleFinder)
		localModules.LowModule = localCache
		remoteModules = new(mem.SingleLowModuleFinder)
		remoteModules.LowModule = remoteGPU
		memCtrlFinder = new(mem.SingleLowModuleFinder)
		memCtrlFinder.LowModule = memCtrl

		pmc = NewPageMigrationController("PMC", engine, memCtrlFinder, remoteModules)

		RemotePort = NewMockPort(mockCtrl)
		ctrlPort = NewMockPort(mockCtrl)
		LocalMemPort = NewMockPort(mockCtrl)
		pmc.remotePort = RemotePort
		pmc.ctrlPort = ctrlPort
		pmc.localMemPort = LocalMemPort
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("should handle page migration req", func() {
		It("should receive a page migraiton req from Control port", func() {
			req := PageMigrationReqToPMCBuilder{}.
				WithSendTime(10).
				WithSrc(nil).
				WithDst(pmc.ctrlPort).
				WithPageSize(4 * mem.KB).
				Build()

			ctrlPort.EXPECT().Retrieve(sim.VTimeInSec(11)).Return(req)

			madeProgress := pmc.processFromCtrlPort(11)

			Expect(pmc.currentMigrationRequest).To(BeEquivalentTo(req))
			Expect(madeProgress).To(BeTrue())
		})

		It("should process the page migration req from Control Port", func() {
			req := PageMigrationReqToPMCBuilder{}.
				WithSendTime(10).
				WithSrc(nil).
				WithDst(pmc.ctrlPort).
				WithPageSize(4 * mem.KB).
				Build()

			pmc.currentMigrationRequest = req

			madeProgress := pmc.processPageMigrationReqFromCtrlPort(11)

			Expect(pmc.toPullFromAnotherPMC).ToNot(BeNil())
			Expect(madeProgress).To(BeTrue())
		})

		It("should send a migration req to another PMC", func() {
			req := DataPullReqBuilder{}.
				WithSendTime(10).
				WithSrc(pmc.remotePort).
				WithDst(nil).
				WithReadFromPhyAddress(0x100).
				WithDataTransferSize(256).
				Build()

			pmc.toPullFromAnotherPMC = append(pmc.toPullFromAnotherPMC, req)

			RemotePort.EXPECT().Send(req).Return(nil)

			madeProgress := pmc.sendMigrationReqToAnotherPMC(11)

			Expect(madeProgress).To(BeTrue())
		})

		It("should receive a data request for page migration from another PMC", func() {
			req := DataPullReqBuilder{}.
				WithSendTime(10).
				WithSrc(pmc.remotePort).
				WithDst(nil).
				WithReadFromPhyAddress(0x100).
				WithDataTransferSize(256).
				Build()

			RemotePort.EXPECT().Peek().Return(req)
			RemotePort.EXPECT().Retrieve(sim.VTimeInSec(11)).Return(req)

			madeProgress := pmc.processFromOutside(11)

			Expect(madeProgress).To(BeTrue())
		})

		It("process a read page req from another PMC", func() {
			req := DataPullReqBuilder{}.
				WithSendTime(10).
				WithSrc(pmc.remotePort).
				WithDst(nil).
				WithReadFromPhyAddress(0x100).
				WithDataTransferSize(256).
				Build()
			pmc.currentPullReqFromAnotherPMC = append(pmc.currentPullReqFromAnotherPMC, req)

			madeProgress := pmc.processReadPageReqFromAnotherPMC(11)

			Expect(pmc.toSendLocalMemPort).ToNot(BeNil())
			Expect(pmc.toSendLocalMemPort[0].Address).To(BeEquivalentTo(uint64(0x100)))
			Expect(pmc.toSendLocalMemPort[0].AccessByteSize).To(BeEquivalentTo(uint64(0x100)))
			Expect(pmc.toSendLocalMemPort[0].ID).To(BeEquivalentTo(req.ID))
			Expect(madeProgress).To(BeTrue())
		})

		It("send the data request from page migration to MemCtrl", func() {
			req := mem.ReadReqBuilder{}.
				WithSendTime(10).
				WithSrc(pmc.localMemPort).
				WithDst(pmc.MemCtrlFinder.Find(0x100)).
				WithAddress(0x100).
				WithByteSize(0x04).
				Build()
			pmc.toSendLocalMemPort = append(pmc.toSendLocalMemPort, req)

			LocalMemPort.EXPECT().Send(req).Return(nil)

			madeProgress := pmc.sendReadReqLocalMemPort(11)

			Expect(madeProgress).To(BeTrue())
		})

		It("should receive a data ready rsp from MemCtrl", func() {
			req := mem.DataReadyRspBuilder{}.
				WithSendTime(10).
				WithSrc(nil).
				WithDst(pmc.localMemPort).
				Build()

			LocalMemPort.EXPECT().Retrieve(sim.VTimeInSec(11)).Return(req)

			madeProgress := pmc.processFromMemCtrl(11)

			Expect(madeProgress).To(BeTrue())
		})

		It("process a data ready rsp from Mem Ctrl", func() {
			data := make([]byte, 0)
			data = append(data, 0x04)
			req := mem.DataReadyRspBuilder{}.
				WithSendTime(10).
				WithSrc(nil).
				WithDst(pmc.localMemPort).
				WithData(data).
				Build()

			pmc.dataReadyRspFromMemCtrl = append(
				pmc.dataReadyRspFromMemCtrl, req)

			pmc.processDataReadyRspFromMemCtrl(11)

			Expect(pmc.toRspToAnotherPMC[0]).ToNot(BeNil())
			Expect(pmc.toRspToAnotherPMC[0].Data).To(HaveLen(1))
			Expect(pmc.toRspToAnotherPMC[0].Data[0]).
				To(BeEquivalentTo(uint64(0x4)))
		})

		It("should send a data ready rsp to requesting PMC", func() {
			data := make([]byte, 0)
			data = append(data, 0x04)
			req := DataPullRspBuilder{}.
				WithSendTime(10).
				WithSrc(pmc.remotePort).
				WithDst(nil).
				WithData(data).
				Build()

			pmc.toRspToAnotherPMC = append(pmc.toRspToAnotherPMC, req)

			RemotePort.EXPECT().Send(req).Return(nil)

			madeProgress := pmc.sendDataReadyRspToRequestingPMC(11)

			Expect(madeProgress).To(BeTrue())
			Expect(len(pmc.toRspToAnotherPMC)).To(BeEquivalentTo(0))
		})

		It("should receive a data ready rsp from the requested PMC", func() {
			data := []byte{1, 2, 3, 4}
			req := DataPullRspBuilder{}.
				WithSendTime(10).
				WithSrc(pmc.remotePort).
				WithDst(nil).
				WithData(data).
				Build()

			RemotePort.EXPECT().Peek().Return(req)
			RemotePort.EXPECT().Retrieve(sim.VTimeInSec(11)).Return(req)

			madeProgress := pmc.processFromOutside(11)

			Expect(madeProgress).To(BeTrue())
		})

		It("should process a data migration rsp from requested PMC", func() {
			data := []byte{1, 2}
			migrationReq := PageMigrationReqToPMCBuilder{}.
				WithSendTime(10).
				WithSrc(nil).
				WithDst(pmc.ctrlPort).
				WithPageSize(4 * mem.KB).
				Build()

			req := DataPullRspBuilder{}.
				WithSendTime(10).
				WithSrc(pmc.remotePort).
				WithDst(nil).
				WithData(data).
				Build()

			pmc.reqIDToWriteAddressMap[req.ID] = 0x100

			pmc.currentMigrationRequest = migrationReq
			pmc.receivedDataFromAnothePMC = append(pmc.receivedDataFromAnothePMC, req)

			madeProgress := pmc.processDataPullRsp(11)

			Expect(madeProgress).To(BeTrue())
			Expect(pmc.writeReqLocalMemPort[0].Data).To(BeEquivalentTo(data))
		})

		It("should send a write req to mem ctrl", func() {
			data := []byte{1, 2}
			req := mem.WriteReqBuilder{}.
				WithSendTime(10).
				WithSrc(pmc.localMemPort).
				WithDst(pmc.MemCtrlFinder.Find(0x100)).
				WithAddress(0x100).
				WithData(data).
				Build()

			pmc.writeReqLocalMemPort = append(pmc.writeReqLocalMemPort, req)

			LocalMemPort.EXPECT().Send(req).Return(nil)

			madeProgress := pmc.sendWriteReqLocalMemPort(11)

			Expect(madeProgress).To(BeTrue())
			Expect(pmc.toSendLocalMemPort).To(BeNil())
		})

		It("should process a write done rsp from memctrl", func() {
			req := mem.WriteDoneRspBuilder{}.
				WithSendTime(10).
				WithSrc(nil).
				WithDst(pmc.localMemPort).
				WithRspTo("xx").
				Build()

			pmc.receivedWriteDoneFromMemCtrl = req

			pmc.numDataRspPendingForPageMigration = 10

			madeProgress := pmc.processWriteDoneRspFromMemCtrl(11)

			Expect(madeProgress).To(BeTrue())
			Expect(pmc.numDataRspPendingForPageMigration).ToNot(BeNil())
			Expect(pmc.numDataRspPendingForPageMigration).To(BeEquivalentTo(uint64(9)))
		})

		It("should receive the last pending data for the page and prepare response for CP", func() {
			req := mem.WriteDoneRspBuilder{}.
				WithSendTime(10).
				WithSrc(nil).
				WithDst(pmc.localMemPort).
				WithRspTo("xx").
				Build()
			pageMigrationReq := PageMigrationReqToPMCBuilder{}.
				WithSendTime(10).
				WithSrc(nil).
				WithDst(pmc.ctrlPort).
				WithPageSize(4 * mem.KB).
				Build()
			pmc.currentMigrationRequest = pageMigrationReq
			pmc.receivedWriteDoneFromMemCtrl = req
			pmc.numDataRspPendingForPageMigration = 1

			madeProgress := pmc.processWriteDoneRspFromMemCtrl(11)

			Expect(madeProgress).To(BeTrue())
			Expect(pmc.numDataRspPendingForPageMigration).ToNot(BeNil())
			Expect(pmc.numDataRspPendingForPageMigration).To(BeEquivalentTo(-1))
			Expect(pmc.toSendToCtrlPort).ToNot(BeNil())
		})

		It("should send migration complete rsp to CP", func() {
			req := PageMigrationRspFromPMCBuilder{}.
				WithSendTime(10).
				WithSrc(pmc.ctrlPort).
				WithDst(nil).
				Build()

			pmc.toSendToCtrlPort = req
			pmc.isHandlingPageMigration = true

			ctrlPort.EXPECT().Send(req).Return(nil)

			madeProgress := pmc.sendMigrationCompleteRspToCtrlPort(11)

			Expect(madeProgress).To(BeTrue())
			Expect(pmc.isHandlingPageMigration).To(BeFalse())
		})
	})
})
