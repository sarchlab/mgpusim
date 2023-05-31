package cache

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v3/mem/mem"
)

var _ = Describe("Directory", func() {

	var (
		mockCtrl     *gomock.Controller
		victimFinder *MockVictimFinder
		directory    *DirectoryImpl
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		victimFinder = NewMockVictimFinder(mockCtrl)
		directory = NewDirectory(1024, 4, 64, victimFinder)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should be able to get total size", func() {
		Expect(directory.TotalSize()).To(Equal(uint64(262144)))
	})

	It("should find victim", func() {
		block := &Block{}
		victimFinder.EXPECT().FindVictim(gomock.Any()).Return(block)
		Expect(directory.FindVictim(0x100)).To(BeIdenticalTo(block))
	})

	It("should lookup", func() {
		block := &Block{
			PID:     1,
			Tag:     0x100,
			IsValid: true,
		}
		set, _ := directory.getSet(0x100)
		set.Blocks[0] = block

		Expect(directory.Lookup(1, 0x100)).To(BeIdenticalTo(block))
	})

	It("should return nil when lookup cannot find block", func() {
		Expect(directory.Lookup(1, 0x100)).To(BeNil())
	})

	It("should return nil if block is invalid", func() {
		block := &Block{
			PID:     1,
			Tag:     0x100,
			IsValid: false,
		}
		set, _ := directory.getSet(0x100)
		set.Blocks[0] = block

		Expect(directory.Lookup(1, 0x100)).To(BeNil())
	})

	It("should return nil if PID does not match", func() {
		block := &Block{
			PID:     2,
			Tag:     0x100,
			IsValid: true,
		}
		set, _ := directory.getSet(0x100)
		set.Blocks[0] = block

		Expect(directory.Lookup(1, 0x100)).To(BeNil())
	})

	It("should update LRU queue when visiting a block", func() {
		set, _ := directory.getSet(0x100)

		directory.Visit(set.Blocks[1])

		Expect(set.LRUQueue[3]).To(BeIdenticalTo(set.Blocks[1]))
	})

	It("should get set considering interleaving", func() {
		directory.AddrConverter = &mem.InterleavingConverter{
			InterleavingSize:    128,
			TotalNumOfElements:  4,
			CurrentElementIndex: 1,
		}

		Expect(func() { directory.getSet(64) }).To(Panic())

		_, setID := directory.getSet(192)
		Expect(setID).To(Equal(1))

		_, setID = directory.getSet(640)
		Expect(setID).To(Equal(2))
	})
})
