package trans

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/addressmapping"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/signal"
	"github.com/sarchlab/mgpusim/v3/mem/mem"
)

var _ = Describe("ClosePageCommandCreator", func() {
	var (
		mockCtrl   *gomock.Controller
		mapper     *MockMapper
		cmdCreator *ClosePageCommandCreator
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mapper = NewMockMapper(mockCtrl)
		cmdCreator = &ClosePageCommandCreator{
			AddrMapper: mapper,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should create read precharge commands", func() {
		read := mem.ReadReqBuilder{}.Build()
		trans := &signal.Transaction{Read: read}
		subTrans := &signal.SubTransaction{
			Transaction: trans,
			Address:     0x40,
		}

		mapper.EXPECT().Map(uint64(0x40)).Return(addressmapping.Location{
			Channel:   1,
			Rank:      2,
			BankGroup: 3,
			Bank:      4,
			Row:       5,
			Column:    6,
		})

		cmd := cmdCreator.Create(subTrans)

		Expect(cmd.Kind).To(Equal(signal.CmdKindReadPrecharge))
		Expect(cmd.SubTrans).To(BeIdenticalTo(subTrans))
	})
})
