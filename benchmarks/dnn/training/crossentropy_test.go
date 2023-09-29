package training_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/tensor"
	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/training"
)

var _ = Describe("Cross Entropy", func() {
	var (
		to tensor.CPUOperator
	)

	BeforeEach(func() {
		to = tensor.CPUOperator{}
	})

	It("should calculate loss", func() {
		output := to.CreateWithData(
			[]float64{
				0.1, 0.2, 0.7,
				0.2, 0.6, 0.2,
			}, []int{2, 3}, "")
		label := []int{1, 1}

		lossFunction := training.NewCrossEntropy(to)

		loss, derivative := lossFunction.Loss(output, label)

		Expect(loss).
			To(BeNumerically("~", 1.06, 0.01))
		expectedDerivative := []float64{
			0, -5, 0,
			0, -1.667, 0,
		}
		derivativeVector := derivative.Vector()
		Expect(derivative.Size()).To(Equal([]int{2, 3}))
		for i, d := range derivativeVector {
			Expect(d).To(
				BeNumerically("~", expectedDerivative[i], 0.01))
		}
	})
})
