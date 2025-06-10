package pagemigrationcontroller

import (
	"log"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"go.uber.org/mock/gomock"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim Port,Engine

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
		localModules  *mem.SinglePortMapper
		remoteModules *mem.SinglePortMapper
		memCtrlFinder *mem.SinglePortMapper
		localCache    *MockPort
		remoteGPU     *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)
		localCache = NewMockPort(mockCtrl)
		remoteGPU = NewMockPort(mockCtrl)
		memCtrl = NewMockPort(mockCtrl)
		localCache.EXPECT().AsRemote().AnyTimes()
		remoteGPU.EXPECT().AsRemote().AnyTimes()
		memCtrl.EXPECT().AsRemote().AnyTimes()
		localModules = new(mem.SinglePortMapper)
		localModules.Port = localCache.AsRemote()
		remoteModules = new(mem.SinglePortMapper)
		remoteModules.Port = remoteGPU.AsRemote()
		memCtrlFinder = new(mem.SinglePortMapper)
		memCtrlFinder.Port = memCtrl.AsRemote()

		pmc = NewPageMigrationController("PMC", engine, memCtrlFinder, remoteModules)

		RemotePort = NewMockPort(mockCtrl)
		ctrlPort = NewMockPort(mockCtrl)
		LocalMemPort = NewMockPort(mockCtrl)
		pmc.remotePort = RemotePort
		pmc.ctrlPort = ctrlPort
		pmc.localMemPort = LocalMemPort
		RemotePort.EXPECT().AsRemote().AnyTimes()
		ctrlPort.EXPECT().AsRemote().AnyTimes()
		LocalMemPort.EXPECT().AsRemote().AnyTimes()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("should handle page migration req", func() {
		It("should receive a page migraiton req from Control port", func() {
			req := PageMigrationReqToPMCBuilder{}.
				WithSrc("").
				WithDst(pmc.ctrlPort.AsRemote()).
				WithPageSize(4 * mem.KB).
				Build()

			ctrlPort.EXPECT().RetrieveIncoming().Return(req)
			engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(10))

			madeProgress := pmc.processFromCtrlPort()

			Expect(pmc.currentMigrationRequest).To(BeEquivalentTo(req))
			Expect(madeProgress).To(BeTrue())
		})

		It("should process the page migration req from Control Port", func() {
			req := PageMigrationReqToPMCBuilder{}.
				WithSrc("").
				WithDst(pmc.ctrlPort.AsRemote()).
				WithPageSize(4 * mem.KB).
				Build()

			pmc.currentMigrationRequest = req

			madeProgress := pmc.processPageMigrationReqFromCtrlPort()

			Expect(pmc.toPullFromAnotherPMC).ToNot(BeNil())
			Expect(madeProgress).To(BeTrue())
		})

		It("should send a migration req to another PMC", func() {
			req := DataPullReqBuilder{}.
				WithSrc(pmc.remotePort.AsRemote()).
				WithDst("").
				WithReadFromPhyAddress(0x100).
				WithDataTransferSize(256).
				Build()

			pmc.toPullFromAnotherPMC = append(pmc.toPullFromAnotherPMC, req)

			RemotePort.EXPECT().Send(req).Return(nil)

			madeProgress := pmc.sendMigrationReqToAnotherPMC()

			Expect(madeProgress).To(BeTrue())
		})

		It("should receive a data request for page migration from another PMC", func() {
			req := DataPullReqBuilder{}.
				WithSrc(pmc.remotePort.AsRemote()).
				WithDst("").
				WithReadFromPhyAddress(0x100).
				WithDataTransferSize(256).
				Build()

			RemotePort.EXPECT().PeekIncoming().Return(req)
			RemotePort.EXPECT().RetrieveIncoming().Return(req)

			madeProgress := pmc.processFromOutside()

			Expect(madeProgress).To(BeTrue())
		})

		It("process a read page req from another PMC", func() {
			req := DataPullReqBuilder{}.
				WithSrc(pmc.remotePort.AsRemote()).
				WithDst("").
				WithReadFromPhyAddress(0x100).
				WithDataTransferSize(256).
				Build()
			pmc.currentPullReqFromAnotherPMC = append(pmc.currentPullReqFromAnotherPMC, req)

			madeProgress := pmc.processReadPageReqFromAnotherPMC()

			Expect(pmc.toSendLocalMemPort).ToNot(BeNil())
			Expect(pmc.toSendLocalMemPort[0].Address).To(BeEquivalentTo(uint64(0x100)))
			Expect(pmc.toSendLocalMemPort[0].AccessByteSize).To(BeEquivalentTo(uint64(0x100)))
			Expect(pmc.toSendLocalMemPort[0].ID).To(BeEquivalentTo(req.ID))
			Expect(madeProgress).To(BeTrue())
		})

		It("send the data request from page migration to MemCtrl", func() {
			req := mem.ReadReqBuilder{}.
				WithSrc(pmc.localMemPort.AsRemote()).
				WithDst(pmc.MemCtrlFinder.Find(0x100)).
				WithAddress(0x100).
				WithByteSize(0x04).
				Build()
			pmc.toSendLocalMemPort = append(pmc.toSendLocalMemPort, req)

			LocalMemPort.EXPECT().Send(req).Return(nil)

			madeProgress := pmc.sendReadReqLocalMemPort()

			Expect(madeProgress).To(BeTrue())
		})

		It("should receive a data ready rsp from MemCtrl", func() {
			req := mem.DataReadyRspBuilder{}.
				WithSrc("").
				WithDst(pmc.localMemPort.AsRemote()).
				Build()

			LocalMemPort.EXPECT().RetrieveIncoming().Return(req)

			madeProgress := pmc.processFromMemCtrl()

			Expect(madeProgress).To(BeTrue())
		})

		It("process a data ready rsp from Mem Ctrl", func() {
			data := make([]byte, 0)
			data = append(data, 0x04)
			req := mem.DataReadyRspBuilder{}.
				WithSrc("").
				WithDst(pmc.localMemPort.AsRemote()).
				WithData(data).
				Build()

			pmc.dataReadyRspFromMemCtrl = append(
				pmc.dataReadyRspFromMemCtrl, req)

			pmc.processDataReadyRspFromMemCtrl()

			Expect(pmc.toRspToAnotherPMC[0]).ToNot(BeNil())
			Expect(pmc.toRspToAnotherPMC[0].Data).To(HaveLen(1))
			Expect(pmc.toRspToAnotherPMC[0].Data[0]).
				To(BeEquivalentTo(uint64(0x4)))
		})

		It("should send a data ready rsp to requesting PMC", func() {
			data := make([]byte, 0)
			data = append(data, 0x04)
			req := DataPullRspBuilder{}.
				WithSrc(pmc.remotePort.AsRemote()).
				WithDst("").
				WithData(data).
				Build()

			pmc.toRspToAnotherPMC = append(pmc.toRspToAnotherPMC, req)

			RemotePort.EXPECT().Send(req).Return(nil)

			madeProgress := pmc.sendDataReadyRspToRequestingPMC()

			Expect(madeProgress).To(BeTrue())
			Expect(len(pmc.toRspToAnotherPMC)).To(BeEquivalentTo(0))
		})

		It("should receive a data ready rsp from the requested PMC", func() {
			data := []byte{1, 2, 3, 4}
			req := DataPullRspBuilder{}.
				WithSrc(pmc.remotePort.AsRemote()).
				WithDst("").
				WithData(data).
				Build()

			RemotePort.EXPECT().PeekIncoming().Return(req)
			RemotePort.EXPECT().RetrieveIncoming().Return(req)

			madeProgress := pmc.processFromOutside()

			Expect(madeProgress).To(BeTrue())
		})

		It("should process a data migration rsp from requested PMC", func() {
			data := []byte{1, 2}
			migrationReq := PageMigrationReqToPMCBuilder{}.
				WithSrc("").
				WithDst(pmc.ctrlPort.AsRemote()).
				WithPageSize(4 * mem.KB).
				Build()

			req := DataPullRspBuilder{}.
				WithSrc(pmc.remotePort.AsRemote()).
				WithDst("").
				WithData(data).
				Build()

			pmc.reqIDToWriteAddressMap[req.ID] = 0x100

			pmc.currentMigrationRequest = migrationReq
			pmc.receivedDataFromAnothePMC = append(pmc.receivedDataFromAnothePMC, req)

			madeProgress := pmc.processDataPullRsp()

			Expect(madeProgress).To(BeTrue())
			Expect(pmc.writeReqLocalMemPort[0].Data).To(BeEquivalentTo(data))
		})

		It("should send a write req to mem ctrl", func() {
			data := []byte{1, 2}
			req := mem.WriteReqBuilder{}.
				WithSrc(pmc.localMemPort.AsRemote()).
				WithDst(pmc.MemCtrlFinder.Find(0x100)).
				WithAddress(0x100).
				WithData(data).
				Build()

			pmc.writeReqLocalMemPort = append(pmc.writeReqLocalMemPort, req)

			LocalMemPort.EXPECT().Send(req).Return(nil)

			madeProgress := pmc.sendWriteReqLocalMemPort()

			Expect(madeProgress).To(BeTrue())
			Expect(pmc.toSendLocalMemPort).To(BeNil())
		})

		It("should process a write done rsp from memctrl", func() {
			req := mem.WriteDoneRspBuilder{}.
				WithSrc("").
				WithDst(pmc.localMemPort.AsRemote()).
				WithRspTo("xx").
				Build()

			pmc.receivedWriteDoneFromMemCtrl = req

			pmc.numDataRspPendingForPageMigration = 10

			madeProgress := pmc.processWriteDoneRspFromMemCtrl()

			Expect(madeProgress).To(BeTrue())
			Expect(pmc.numDataRspPendingForPageMigration).ToNot(BeNil())
			Expect(pmc.numDataRspPendingForPageMigration).To(BeEquivalentTo(uint64(9)))
		})

		It("should receive the last pending data for the page and prepare response for CP", func() {
			req := mem.WriteDoneRspBuilder{}.
				WithSrc("").
				WithDst(pmc.localMemPort.AsRemote()).
				WithRspTo("xx").
				Build()
			pageMigrationReq := PageMigrationReqToPMCBuilder{}.
				WithSrc("").
				WithDst(pmc.ctrlPort.AsRemote()).
				WithPageSize(4 * mem.KB).
				Build()
			pmc.currentMigrationRequest = pageMigrationReq
			pmc.receivedWriteDoneFromMemCtrl = req
			pmc.numDataRspPendingForPageMigration = 1

			madeProgress := pmc.processWriteDoneRspFromMemCtrl()

			Expect(madeProgress).To(BeTrue())
			Expect(pmc.numDataRspPendingForPageMigration).ToNot(BeNil())
			Expect(pmc.numDataRspPendingForPageMigration).To(BeEquivalentTo(-1))
			Expect(pmc.toSendToCtrlPort).ToNot(BeNil())
		})

		It("should send migration complete rsp to CP", func() {
			req := PageMigrationRspFromPMCBuilder{}.
				WithSrc(pmc.ctrlPort.AsRemote()).
				WithDst("").
				Build()

			pmc.toSendToCtrlPort = req
			pmc.isHandlingPageMigration = true

			ctrlPort.EXPECT().Send(req).Return(nil)
			engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(11))

			madeProgress := pmc.sendMigrationCompleteRspToCtrlPort()

			Expect(madeProgress).To(BeTrue())
			Expect(pmc.isHandlingPageMigration).To(BeFalse())
		})
	})
})
