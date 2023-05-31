package trans

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/signal"
	"github.com/sarchlab/mgpusim/v3/mem/mem"
)

var _ = Describe("Default SubTransSplitter", func() {

	It("should split", func() {
		read := mem.ReadReqBuilder{}.
			WithAddress(1020).
			WithByteSize(128).
			Build()
		transaction := &signal.Transaction{
			Read: read,
		}

		splitter := NewSubTransSplitter(6)

		splitter.Split(transaction)

		Expect(transaction.SubTransactions).To(HaveLen(3))
	})
})
