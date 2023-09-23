package training

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.com/akita/dnn/tensor"
)

var _ = Describe("Cross Entropy", func() {
	var (
		to tensor.Operator
	)

	BeforeEach(func() {
		to = tensor.CPUOperator{}
	})

	It("should calculate loss", func() {
		output := to.CreateWithData([]float64{
			1, 3, 2,
			5, 7, 9,
		}, []int{2, 3}, "")
		label := []int{1, 1}

		lossFunction := NewSoftmaxCrossEntropy(to)

		loss, derivative := lossFunction.Loss(output, label)

		Expect(loss).
			To(BeNumerically("~", 1.275, 0.01))
		expectedDerivative := []float64{
			1, 2, 2,
			5, 6, 9,
		}
		derivativeVector := derivative.Vector()
		Expect(derivative.Size()).To(Equal([]int{2, 3}))
		for i, d := range derivativeVector {
			Expect(d).To(
				BeNumerically("~", expectedDerivative[i], 0.01))
		}

	})
})
