package optimization

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("SGD", func() {
	var (
		mockCtrl          *gomock.Controller
		to                *MockOperator
		layer             *MockLayer
		params, gradients *MockTensor
		sgd               *SGD
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		layer = NewMockLayer(mockCtrl)
		to = NewMockOperator(mockCtrl)
		params = NewMockTensor(mockCtrl)
		gradients = NewMockTensor(mockCtrl)
		sgd = NewSGD(to, 0.3)

		layer.EXPECT().Parameters().Return(params).AnyTimes()
		layer.EXPECT().Gradients().Return(gradients).AnyTimes()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should run SGD", func() {
		to.EXPECT().ScaleAdd(1.0, -0.3, params, gradients)
		to.EXPECT().Copy(params, gomock.Any())
		to.EXPECT().Free(gomock.Any())

		sgd.UpdateParameters(layer)
	})
})
