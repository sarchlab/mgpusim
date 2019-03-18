package driver

import (
	"github.com/golang/mock/gomock"
	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/mem/vm/mock_vm"

	"gitlab.com/akita/mem"
)

var _ = ginkgo.Describe("Driver", func() {
	var (
		mockCtrl *gomock.Controller
		driver   *Driver
		context  *Context
		mmu      *mock_vm.MockMMU
	)

	ginkgo.BeforeEach(func() {
		mockCtrl = gomock.NewController(ginkgo.GinkgoT())
		mmu = mock_vm.NewMockMMU(mockCtrl)

		driver = NewDriver(nil, mmu)
		driver.registerStorage(0, 4*mem.GB)

		context = driver.Init()
		context.pid = 1
	})

	ginkgo.AfterEach(func() {
		mockCtrl.Finish()
	})

	ginkgo.It("should allocate memory", func() {
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0,
				VAddr:    0x100000000,
				PageSize: 4096,
				Valid:    true,
			})

		ptr := driver.AllocateMemory(context, 8)
		Expect(ptr).To(Equal(GPUPtr(0x100000000)))

		ptr = driver.AllocateMemory(context, 24)
		Expect(ptr).To(Equal(GPUPtr(0x100000008)))
	})

	ginkgo.It("should remap pages", func() {
		page1 := &vm.Page{
			PID:      1,
			VAddr:    0x10000000,
			PAddr:    0x0,
			PageSize: 0x1000,
			Valid:    true,
		}
		page2 := &vm.Page{
			PID:      1,
			VAddr:    0x10001000,
			PAddr:    0x1000,
			PageSize: 0x1000,
			Valid:    true,
		}
		page3 := &vm.Page{
			PID:      1,
			VAddr:    0x10002000,
			PAddr:    0x2000,
			PageSize: 0x1000,
			Valid:    true,
		}
		driver.allocatedPages = make([][]*vm.Page, 2)
		driver.allocatedPages[0] = []*vm.Page{page1, page2, page3}
		driver.storageSizes = []uint64{0x10000000000, 0x10000000000}
		driver.initialAddresses = []uint64{0x0, 0x10000000000}

		mmu.EXPECT().
			Translate(vm.PID(1), uint64(0x10000400)).
			Return(uint64(0x400), page1)
		mmu.EXPECT().
			Translate(vm.PID(1), uint64(0x10001000)).
			Return(uint64(0x1000), page2)
		mmu.EXPECT().
			Translate(vm.PID(1), uint64(0x10002000)).
			Return(uint64(0x2000), page3)
		mmu.EXPECT().RemovePage(vm.PID(1), uint64(0x10000000))
		mmu.EXPECT().RemovePage(vm.PID(1), uint64(0x10001000))
		mmu.EXPECT().RemovePage(vm.PID(1), uint64(0x10002000))
		mmu.EXPECT().CreatePage(&vm.Page{
			PID:      1,
			VAddr:    0x10000000,
			PAddr:    0x0,
			PageSize: 0x400,
			Valid:    true,
		})
		mmu.EXPECT().CreatePage(&vm.Page{
			PID:      1,
			VAddr:    0x10000400,
			PAddr:    0x10000000000,
			PageSize: 0xC00,
			Valid:    true,
		})
		mmu.EXPECT().CreatePage(&vm.Page{
			PID:      1,
			VAddr:    0x10001000,
			PAddr:    0x10000001000,
			PageSize: 0x1000,
			Valid:    true,
		})
		mmu.EXPECT().CreatePage(&vm.Page{
			PID:      1,
			VAddr:    0x10002000,
			PAddr:    0x10000002000,
			PageSize: 0x400,
			Valid:    true,
		})
		mmu.EXPECT().CreatePage(&vm.Page{
			PID:      1,
			VAddr:    0x10002400,
			PAddr:    0x2400,
			PageSize: 0xC00,
			Valid:    true,
		})

		driver.Remap(context, 0x10000400, 0x2000, 1)
	})

	ginkgo.It("should distribute memory when page size is less than a page", func() {
		byteSize := uint64(1024)

		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0,
				VAddr:    0x100000000,
				PageSize: 4096,
				Valid:    true,
			})
		ptr := driver.AllocateMemory(context, byteSize)

		allocatedBytes := driver.Distribute(context, uint64(ptr), byteSize, []int{1, 2, 3})
		Expect(allocatedBytes).To(Equal([]uint64{1024, 0, 0}))
	})

	ginkgo.It("should distribute memory when the request size aligns with pages", func() {
		driver.storageSizes = []uint64{
			0x10000000000,
			0x10000000000,
			0x10000000000,
			0x10000000000,
		}
		driver.initialAddresses = []uint64{
			0x0,
			0x10000000000,
			0x20000000000,
			0x30000000000,
		}
		driver.allocatedPages = make([][]*vm.Page, 4)

		byteSize := uint64(4096 * 6)
		pages := []*vm.Page{}

		for i := 0; i < 6; i++ {
			page := &vm.Page{
				PID:      1,
				PAddr:    uint64(4096 * i),
				VAddr:    0x100000000 + uint64(4096*i),
				PageSize: 4096,
				Valid:    true,
			}
			pages = append(pages, page)
			mmu.EXPECT().CreatePage(page)
		}
		ptr := driver.AllocateMemory(context, byteSize)

		for i := 0; i < 6; i++ {
			mmu.EXPECT().
				Translate(vm.PID(1), uint64(0x100000000+4096*i)).
				Return(pages[i].PAddr, pages[i])

			mmu.EXPECT().RemovePage(vm.PID(1), uint64(0x100000000+4096*i))
		}

		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x10000000000,
				VAddr:    0x100000000,
				PageSize: 4096,
				Valid:    true,
			})
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x10000001000,
				VAddr:    0x100001000,
				PageSize: 4096,
				Valid:    true,
			})
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x20000000000,
				VAddr:    0x100002000,
				PageSize: 4096,
				Valid:    true,
			})
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x20000001000,
				VAddr:    0x100003000,
				PageSize: 4096,
				Valid:    true,
			})
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x30000000000,
				VAddr:    0x100004000,
				PageSize: 4096,
				Valid:    true,
			})
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x30000001000,
				VAddr:    0x100005000,
				PageSize: 4096,
				Valid:    true,
			})

		allocatedBytes := driver.Distribute(context,
			uint64(ptr), byteSize, []int{1, 2, 3})
		Expect(allocatedBytes).To(Equal([]uint64{8192, 8192, 8192}))

	})

	ginkgo.It("should distribute the remaining pages to the last GPU", func() {
		driver.storageSizes = []uint64{
			0x10000000000,
			0x10000000000,
			0x10000000000,
			0x10000000000,
		}
		driver.initialAddresses = []uint64{
			0x0,
			0x10000000000,
			0x20000000000,
			0x30000000000,
		}
		driver.allocatedPages = make([][]*vm.Page, 4)

		byteSize := uint64(4096 * 7)
		pages := []*vm.Page{}

		for i := 0; i < 7; i++ {
			page := &vm.Page{
				PID:      1,
				PAddr:    uint64(4096 * i),
				VAddr:    0x100000000 + uint64(4096*i),
				PageSize: 4096,
				Valid:    true,
			}
			pages = append(pages, page)
			mmu.EXPECT().CreatePage(page)
		}
		ptr := driver.AllocateMemory(context, byteSize)

		for i := 0; i < 7; i++ {
			mmu.EXPECT().
				Translate(vm.PID(1), uint64(0x100000000+4096*i)).
				Return(pages[i].PAddr, pages[i])

			mmu.EXPECT().RemovePage(vm.PID(1), uint64(0x100000000+4096*i))
		}

		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x10000000000,
				VAddr:    0x100000000,
				PageSize: 4096,
				Valid:    true,
			})
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x10000001000,
				VAddr:    0x100001000,
				PageSize: 4096,
				Valid:    true,
			})
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x20000000000,
				VAddr:    0x100002000,
				PageSize: 4096,
				Valid:    true,
			})
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x20000001000,
				VAddr:    0x100003000,
				PageSize: 4096,
				Valid:    true,
			})
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x30000000000,
				VAddr:    0x100004000,
				PageSize: 4096,
				Valid:    true,
			})
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x30000001000,
				VAddr:    0x100005000,
				PageSize: 4096,
				Valid:    true,
			})
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x30000002000,
				VAddr:    0x100006000,
				PageSize: 4096,
				Valid:    true,
			})

		allocatedBytes := driver.Distribute(context,
			uint64(ptr), byteSize, []int{1, 2, 3})
		Expect(allocatedBytes).To(Equal([]uint64{8192, 8192, 12288}))
	})

	ginkgo.It("should distribute 2.5 pages to 3 GPUs", func() {
		driver.storageSizes = []uint64{
			0x10000000000,
			0x10000000000,
			0x10000000000,
			0x10000000000,
		}
		driver.initialAddresses = []uint64{
			0x0,
			0x10000000000,
			0x20000000000,
			0x30000000000,
		}
		driver.allocatedPages = make([][]*vm.Page, 4)

		byteSize := uint64(4096*2 + 2048)
		pages := []*vm.Page{}

		for i := 0; i < 3; i++ {
			page := &vm.Page{
				PID:      1,
				PAddr:    uint64(4096 * i),
				VAddr:    0x100000000 + uint64(4096*i),
				PageSize: 4096,
				Valid:    true,
			}
			pages = append(pages, page)
			mmu.EXPECT().CreatePage(page)
		}
		ptr := driver.AllocateMemory(context, byteSize)

		for i := 0; i < 3; i++ {
			mmu.EXPECT().
				Translate(vm.PID(1), uint64(0x100000000+4096*i)).
				Return(pages[i].PAddr, pages[i])

			mmu.EXPECT().RemovePage(vm.PID(1), uint64(0x100000000+4096*i))
		}

		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x10000000000,
				VAddr:    0x100000000,
				PageSize: 4096,
				Valid:    true,
			})
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x20000000000,
				VAddr:    0x100001000,
				PageSize: 4096,
				Valid:    true,
			})
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x20000001000,
				VAddr:    0x100002000,
				PageSize: 4096,
				Valid:    true,
			})

		allocatedBytes := driver.Distribute(context,
			uint64(ptr), byteSize, []int{1, 2, 3})
		Expect(allocatedBytes).To(Equal([]uint64{4096, 4096 + 2048, 0}))
	})

	ginkgo.It("should allocate memory with alignment", func() {
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0,
				VAddr:    0x100000000,
				PageSize: 4096,
				Valid:    true,
			})

		ptr := driver.AllocateMemoryWithAlignment(context, 8, 64)
		Expect(ptr).To(Equal(GPUPtr(0x100000000)))
		Expect(driver.allocatedPages[0]).To(HaveLen(1))
		Expect(driver.memoryMasks[0]).To(HaveLen(2))

		ptr = driver.AllocateMemoryWithAlignment(context, 8, 64)
		Expect(ptr).To(Equal(GPUPtr(0x100000040)))
		Expect(driver.allocatedPages[0]).To(HaveLen(1))
		Expect(driver.memoryMasks[0]).To(HaveLen(4))
	})

	ginkgo.It("should allocate memory larger than a page", func() {
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0,
				VAddr:    0x100000000,
				PageSize: 4096,
				Valid:    true,
			})
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x1000,
				VAddr:    0x100001000,
				PageSize: 4096,
				Valid:    true,
			})
		mmu.EXPECT().CreatePage(
			&vm.Page{
				PID:      1,
				PAddr:    0x2000,
				VAddr:    0x100002000,
				PageSize: 4096,
				Valid:    true,
			})

		ptr := driver.AllocateMemory(context, 8196)
		Expect(ptr).To(Equal(GPUPtr(0x100000000)))
		Expect(driver.allocatedPages[0]).To(HaveLen(3))
	})

	ginkgo.It("should free memory", func() {
		//ptr := driver.AllocateMemory(4)
		//ptr2 := driver.AllocateMemory(16)
		//ptr3 := driver.AllocateMemory(8)
		//ptr4 := driver.AllocateMemory(12)
		//ptr5 := driver.AllocateMemory(24)
		//
		//driver.memoryMasks[0].Chunks[5].ByteSize = 36
		//
		//driver.FreeMemory(ptr)
		//Expect(driver.memoryMasks[0].Chunks[0].Occupied).To(Equal(false))
		//Expect(driver.memoryMasks[0].Chunks[0].ByteSize).To(Equal(uint64(4)))
		//
		//driver.FreeMemory(ptr2)
		//Expect(driver.memoryMasks[0].Chunks[0].Occupied).To(Equal(false))
		//Expect(driver.memoryMasks[0].Chunks[0].ByteSize).To(Equal(uint64(20)))
		//
		//driver.FreeMemory(ptr5)
		//Expect(driver.memoryMasks[0].Chunks[3].Occupied).To(Equal(false))
		//Expect(driver.memoryMasks[0].Chunks[3].ByteSize).To(Equal(uint64(60)))
		//
		//driver.FreeMemory(ptr4)
		//Expect(driver.memoryMasks[0].Chunks[2].Occupied).To(Equal(false))
		//Expect(driver.memoryMasks[0].Chunks[2].ByteSize).To(Equal(uint64(72)))
		//
		//driver.FreeMemory(ptr3)
		//Expect(driver.memoryMasks[0].Chunks[0].Occupied).To(Equal(false))
		//Expect(len(driver.memoryMasks[0].Chunks)).To(Equal(1))
	})

})
