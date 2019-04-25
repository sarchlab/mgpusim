package l1v

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Directory", func() {
	var (
		mockCtrl *gomock.Controller
		inBuf    *MockBuffer
		d        *directory
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		inBuf = NewMockBuffer(mockCtrl)
		d = &directory{
			inBuf: inBuf,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no transaction", func() {
		inBuf.EXPECT().Peek().Return(nil)
		madeProgress := d.Tick(10)
		Expect(madeProgress).To(BeFalse())
	})

})
