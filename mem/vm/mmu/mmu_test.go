package mmu

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/vm"
)

var _ = Describe("MMU", func() {

	var (
		mockCtrl      *gomock.Controller
		engine        *MockEngine
		toTop         *MockPort
		migrationPort *MockPort
		topSender     *MockBufferedSender
		pageTable     *MockPageTable
		mmu           *MMU
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)
		toTop = NewMockPort(mockCtrl)
		migrationPort = NewMockPort(mockCtrl)
		topSender = NewMockBufferedSender(mockCtrl)
		pageTable = NewMockPageTable(mockCtrl)

		builder := MakeBuilder().WithEngine(engine)
		mmu = builder.Build("MMU")
		mmu.topPort = toTop
		mmu.topSender = topSender
		mmu.migrationPort = migrationPort
		mmu.pageTable = pageTable
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("parse top", func() {
		It("should process translation request", func() {
			translationReq := vm.TranslationReqBuilder{}.
				WithSendTime(10).
				WithDst(mmu.topPort).
				WithPID(1).
				WithVAddr(0x100000100).
				WithDeviceID(0).
				Build()
			toTop.EXPECT().
				Retrieve(sim.VTimeInSec(10)).
				Return(translationReq)

			mmu.parseFromTop(10)

			Expect(mmu.walkingTranslations).To(HaveLen(1))

		})

		It("should stall parse from top if MMU is servicing max requests", func() {
			mmu.walkingTranslations = make([]transaction, 16)

			madeProgress := mmu.parseFromTop(10)

			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("walk page table", func() {
		It("should reduce translation cycles", func() {
			req := vm.TranslationReqBuilder{}.
				WithSendTime(10).
				WithDst(toTop).
				WithPID(1).
				WithVAddr(0x1020).
				WithDeviceID(0).
				Build()
			walking := transaction{req: req, cycleLeft: 10}
			mmu.walkingTranslations = append(mmu.walkingTranslations, walking)

			madeProgress := mmu.walkPageTable(11)

			Expect(mmu.walkingTranslations[0].cycleLeft).To(Equal(9))
			Expect(madeProgress).To(BeTrue())
		})

		It("should send rsp to top if hit", func() {
			page := vm.Page{
				PID:      1,
				VAddr:    0x1000,
				PAddr:    0x0,
				PageSize: 4096,
				Valid:    true,
			}
			req := vm.TranslationReqBuilder{}.
				WithSendTime(10).
				WithDst(mmu.topPort).
				WithPID(1).
				WithVAddr(0x1000).
				WithDeviceID(0).
				Build()
			walking := transaction{req: req, cycleLeft: 0}
			mmu.walkingTranslations = append(mmu.walkingTranslations, walking)

			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x1000)).
				Return(page, true)
			topSender.EXPECT().CanSend(1).Return(true)
			topSender.EXPECT().
				Send(gomock.Any()).
				Do(func(rsp *vm.TranslationRsp) {
					Expect(rsp.Page).To(Equal(page))
				})

			madeProgress := mmu.walkPageTable(11)

			Expect(madeProgress).To(BeTrue())
			Expect(mmu.walkingTranslations).To(HaveLen(0))
		})

		It("should stall if cannot send to top", func() {
			page := vm.Page{
				PID:      1,
				VAddr:    0x1000,
				PAddr:    0x0,
				PageSize: 4096,
				Valid:    true,
			}
			req := vm.TranslationReqBuilder{}.
				WithSendTime(10).
				WithDst(mmu.topPort).
				WithPID(1).
				WithVAddr(0x1000).
				WithDeviceID(0).
				Build()
			walking := transaction{req: req, cycleLeft: 0}
			mmu.walkingTranslations =
				append(mmu.walkingTranslations, walking)

			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x1000)).
				Return(page, true)
			topSender.EXPECT().CanSend(1).Return(false)

			madeProgress := mmu.walkPageTable(11)

			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("migration", func() {
		var (
			page    vm.Page
			req     *vm.TranslationReq
			walking transaction
		)

		BeforeEach(func() {
			page = vm.Page{
				PID:      1,
				VAddr:    0x1000,
				PAddr:    0x0,
				PageSize: 4096,
				Valid:    true,
				DeviceID: 2,
				Unified:  true,
			}
			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x1000)).
				Return(page, true).
				AnyTimes()
			req = vm.TranslationReqBuilder{}.
				WithSendTime(10).
				WithDst(mmu.topPort).
				WithPID(1).
				WithVAddr(0x1000).
				WithDeviceID(0).
				Build()
			walking = transaction{
				req:       req,
				page:      page,
				cycleLeft: 0,
			}
		})

		It("should be placed in the migration queue", func() {
			mmu.walkingTranslations = append(mmu.walkingTranslations, walking)

			updatedPage := page
			updatedPage.IsMigrating = true
			pageTable.EXPECT().Update(updatedPage)

			madeProgress := mmu.walkPageTable(11)

			Expect(madeProgress).To(BeTrue())
			Expect(mmu.walkingTranslations).To(HaveLen(0))
			Expect(mmu.migrationQueue).To(HaveLen(1))
		})

		It("should place the page in the migration queue if the page is being migrated", func() {
			req.PID = 2
			page.PID = 2
			page.IsMigrating = true
			pageTable.EXPECT().
				Find(vm.PID(2), uint64(0x1000)).
				Return(page, true)
			mmu.walkingTranslations = append(mmu.walkingTranslations, walking)

			pageTable.EXPECT().Update(page)

			madeProgress := mmu.walkPageTable(11)

			Expect(madeProgress).To(BeTrue())
			Expect(mmu.walkingTranslations).To(HaveLen(0))
			Expect(mmu.migrationQueue).To(HaveLen(1))
		})

		It("should not send to driver if migration queue is empty", func() {
			madeProgress := mmu.sendMigrationToDriver(11)

			Expect(madeProgress).To(BeFalse())
		})

		It("should wait if mmu is waiting for a migration to finish", func() {
			mmu.migrationQueue = append(mmu.migrationQueue, walking)
			mmu.isDoingMigration = true

			madeProgress := mmu.sendMigrationToDriver(11)

			Expect(madeProgress).To(BeFalse())
			Expect(mmu.migrationQueue).To(ContainElement(walking))
		})

		It("should stall if send failed", func() {
			mmu.migrationQueue = append(mmu.migrationQueue, walking)

			migrationPort.EXPECT().
				Send(gomock.Any()).
				Return(sim.NewSendError())

			madeProgress := mmu.sendMigrationToDriver(11)

			Expect(madeProgress).To(BeFalse())
			Expect(mmu.migrationQueue).To(ContainElement(walking))
		})

		It("should send migration request", func() {
			mmu.migrationQueue = append(mmu.migrationQueue, walking)

			migrationPort.EXPECT().
				Send(gomock.Any()).
				Return(nil)
			updatedPage := page
			updatedPage.IsMigrating = true
			pageTable.EXPECT().Update(updatedPage)

			madeProgress := mmu.sendMigrationToDriver(11)

			Expect(madeProgress).To(BeTrue())
			Expect(mmu.migrationQueue).NotTo(ContainElement(walking))
			Expect(mmu.isDoingMigration).To(BeTrue())
		})

		It("should reply to the GPU if the page is already on the destination GPU", func() {
			walking.req.DeviceID = 2
			mmu.migrationQueue = append(mmu.migrationQueue, walking)

			updatedPage := page
			updatedPage.IsMigrating = false
			pageTable.EXPECT().Update(updatedPage)
			topSender.EXPECT().Send(gomock.Any())

			madeProgress := mmu.sendMigrationToDriver(11)

			Expect(madeProgress).To(BeTrue())
			Expect(mmu.migrationQueue).NotTo(ContainElement(walking))
			Expect(mmu.isDoingMigration).To(BeFalse())
		})
	})

	Context("when received migrated page information", func() {
		var (
			page          vm.Page
			req           *vm.TranslationReq
			migrating     transaction
			migrationDone *vm.PageMigrationRspFromDriver
		)

		BeforeEach(func() {
			page = vm.Page{
				PID:         1,
				VAddr:       0x1000,
				PAddr:       0x0,
				PageSize:    4096,
				Valid:       true,
				DeviceID:    1,
				Unified:     true,
				IsMigrating: true,
			}
			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x1000)).
				Return(page, true).
				AnyTimes()
			req = vm.TranslationReqBuilder{}.
				WithSendTime(10).
				WithDst(mmu.topPort).
				WithPID(1).
				WithVAddr(0x1000).
				WithDeviceID(0).
				Build()
			migrating = transaction{req: req, cycleLeft: 0}
			mmu.currentOnDemandMigration = migrating
			migrationDone = vm.NewPageMigrationRspFromDriver(0, nil, nil)
		})

		It("should do nothing if no respond", func() {
			migrationPort.EXPECT().Peek().Return(nil)

			madeProgress := mmu.processMigrationReturn(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if send to top failed", func() {
			migrationPort.EXPECT().Peek().Return(migrationDone)
			topSender.EXPECT().CanSend(1).Return(false)

			madeProgress := mmu.processMigrationReturn(10)

			Expect(madeProgress).To(BeFalse())
			Expect(mmu.isDoingMigration).To(BeFalse())
		})

		It("should send rsp to top", func() {
			migrationPort.EXPECT().Peek().Return(migrationDone)
			topSender.EXPECT().CanSend(1).Return(true)
			topSender.EXPECT().Send(gomock.Any()).
				Do(func(rsp *vm.TranslationRsp) {
					Expect(rsp.Page).To(Equal(page))
				})
			migrationPort.EXPECT().Retrieve(gomock.Any())

			updatedPage := page
			updatedPage.IsMigrating = false
			pageTable.EXPECT().Update(updatedPage)

			updatedPage.IsPinned = true
			pageTable.EXPECT().Update(updatedPage)

			madeProgress := mmu.processMigrationReturn(10)

			Expect(madeProgress).To(BeTrue())
			Expect(mmu.isDoingMigration).To(BeFalse())
		})

	})
})

var _ = Describe("MMU Integration", func() {
	var (
		mockCtrl   *gomock.Controller
		engine     sim.Engine
		mmu        *MMU
		agent      *MockPort
		connection sim.Connection
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = sim.NewSerialEngine()

		builder := MakeBuilder().WithEngine(engine)
		mmu = builder.Build("MMU")
		agent = NewMockPort(mockCtrl)
		connection = sim.NewDirectConnection("Conn", engine, 1*sim.GHz)

		agent.EXPECT().SetConnection(connection)
		connection.PlugIn(agent, 10)
		connection.PlugIn(mmu.topPort, 10)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should lookup", func() {
		page := vm.Page{
			PID:      1,
			VAddr:    0x1000,
			PAddr:    0x2000,
			PageSize: 4096,
			Valid:    true,
			DeviceID: 1,
		}
		mmu.pageTable.Insert(page)

		req := vm.TranslationReqBuilder{}.
			WithSendTime(10).
			WithSrc(agent).
			WithDst(mmu.topPort).
			WithPID(1).
			WithVAddr(0x1000).
			WithDeviceID(0).
			Build()
		req.RecvTime = 10
		mmu.topPort.Recv(req)

		agent.EXPECT().Recv(gomock.Any()).
			Do(func(rsp *vm.TranslationRsp) {
				Expect(rsp.Page).To(Equal(page))
				Expect(rsp.RespondTo).To(Equal(req.ID))
			})

		engine.Run()
	})
})
