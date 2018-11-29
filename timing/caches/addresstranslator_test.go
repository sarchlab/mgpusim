package caches

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/akita/mock_akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/vm"
)

var _ = Describe("Addresstranslator", func() {
	var (
		mockCtrl *gomock.Controller
		toTLB    *mock_akita.MockPort
		t        *addressTranslator
		l1v      *L1VCache
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		toTLB = mock_akita.NewMockPort(mockCtrl)

		t = new(addressTranslator)
		l1v = NewL1VCache("l1v", nil, 1*akita.GHz)
		l1v.ToTLB = toTLB
		t.l1vCache = l1v
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should send TranslationReq", func() {
		req := mem.NewReadReq(6, nil, nil, 0x1100, 64)
		transaction := &cacheTransaction{
			Req: req,
		}
		l1v.preAddrTranslationBuf = append(l1v.preAddrTranslationBuf,
			transaction)

		toTLB.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(nil)

		madeProgress := t.tick(10)

		Expect(t.toSendToTLB).To(HaveLen(1))
		Expect(madeProgress).To(BeTrue())
		Expect(t.pendingTranslation).To(BeIdenticalTo(transaction))
		Expect(l1v.preAddrTranslationBuf).To(HaveLen(0))
	})

	It("should stall if it is translating another request", func() {
		req := mem.NewReadReq(6, nil, nil, 0x1100, 64)
		transaction := &cacheTransaction{
			Req: req,
		}
		t.pendingTranslation = transaction
		l1v.preAddrTranslationBuf = append(l1v.preAddrTranslationBuf,
			transaction)

		toTLB.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(nil)

		madeProgress := t.tick(10)

		Expect(t.toSendToTLB).To(HaveLen(0))
		Expect(madeProgress).To(BeFalse())
	})

	It("should send translation req to TLB", func() {
		translationReq := vm.NewTranslateReq(6, nil, nil, 1, 0x1000)
		t.toSendToTLB = append(t.toSendToTLB, translationReq)

		toTLB.EXPECT().
			Send(gomock.AssignableToTypeOf(&vm.TranslationReq{})).
			Return(nil)
		toTLB.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(nil)

		madeProgress := t.tick(10)

		Expect(madeProgress).To(BeTrue())
		Expect(t.toSendToTLB).To(HaveLen(0))
	})

	It("should finish translation", func() {
		req := mem.NewReadReq(6, nil, nil, 0x1100, 64)
		transaction := &cacheTransaction{
			Req: req,
		}
		t.pendingTranslation = transaction
		rsp := vm.NewTranslateReadyRsp(9, nil, nil, req.ID, nil)
		rsp.Page = &vm.Page{
			PID:      1,
			VAddr:    0x1000,
			PAddr:    0x0,
			PageSize: 0x1000,
			Valid:    true,
		}

		toTLB.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(rsp)

		madeProgress := t.tick(10)

		Expect(madeProgress).To(BeTrue())
		Expect(t.pendingTranslation).To(BeNil())
		Expect(transaction.Page).To(BeIdenticalTo(rsp.Page))
		Expect(l1v.postAddrTranslationBuf).To(HaveLen(1))
		Expect(l1v.postAddrTranslationBuf[0]).To(BeIdenticalTo(transaction))

	})

})
