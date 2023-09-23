package training

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	"gitlab.com/akita/dnn/layers"
	"gitlab.com/akita/dnn/tensor"
)

var _ = Describe("Trainer", func() {
	var (
		mockCtrl        *gomock.Controller
		dataSource      *MockDataSource
		layer1, layer2  *MockLayer
		network         Network
		lossFunc        *MockLossFunction
		optimizationAlg *MockAlg
		trainer         Trainer
		to              *tensor.CPUOperator
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		to = &tensor.CPUOperator{}

		layer1 = NewMockLayer(mockCtrl)
		layer2 = NewMockLayer(mockCtrl)
		network = Network{
			Layers: []layers.Layer{layer1, layer2},
		}
		dataSource = NewMockDataSource(mockCtrl)
		lossFunc = NewMockLossFunction(mockCtrl)
		optimizationAlg = NewMockAlg(mockCtrl)
		trainer = Trainer{
			DataSource:      dataSource,
			Network:         network,
			LossFunc:        lossFunc,
			OptimizationAlg: optimizationAlg,
			Epoch:           2,
			BatchSize:       32,
		}

	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should train", func() {
		tensor1 := to.Create([]int{32, 100})
		label1 := make([]int, 32)

		tensor2 := to.Create([]int{16, 100})
		label2 := make([]int, 16)

		tensor3 := to.Create([]int{100})

		dataSource.EXPECT().Rewind()
		dataSource.EXPECT().NextBatch(32).Return(tensor1, label1)
		dataSource.EXPECT().NextBatch(32).Return(tensor2, label2)
		dataSource.EXPECT().NextBatch(32).Return(tensor3, nil)

		dataSource.EXPECT().Rewind()
		dataSource.EXPECT().NextBatch(32).Return(tensor1, label1)
		dataSource.EXPECT().NextBatch(32).Return(tensor2, label2)
		dataSource.EXPECT().NextBatch(32).Return(tensor3, nil)

		layer1.EXPECT().
			Forward(gomock.Any()).
			Times(4)
		layer1.EXPECT().
			Backward(gomock.Any()).
			Times(4)
		layer2.EXPECT().
			Forward(gomock.Any()).
			Times(4)
		layer2.EXPECT().
			Backward(gomock.Any()).
			Times(4)
		lossFunc.EXPECT().Loss(gomock.Any(), gomock.Any()).Times(4)
		optimizationAlg.EXPECT().UpdateParameters(layer1).Times(4)
		optimizationAlg.EXPECT().UpdateParameters(layer2).Times(4)

		trainer.Train()
	})
})
