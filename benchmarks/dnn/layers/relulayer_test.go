package layers

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v4/benchmarks/dnn/tensor"
)

var _ = Describe("Relu Layer", func() {
	var (
		reluLayer *ReluLayer
		to        *tensor.CPUOperator
		input     tensor.Tensor
	)

	BeforeEach(func() {
		to = &tensor.CPUOperator{}
		reluLayer = NewReluLayer(to)
	})

	It("should forward", func() {
		input = to.CreateWithData(
			[]float64{
				1, -1,
				2, 3,
			}, []int{2, 2}, "")

		output := reluLayer.Forward(input)

		Expect(output.Size()).To(Equal([]int{2, 2}))
		Expect(output.Vector()).To(Equal([]float64{1, 0, 2, 3}))
		Expect(reluLayer.forwardInput).To(Equal(input))
	})

	It("should backward", func() {
		input = to.CreateWithData([]float64{10, 20, 3, 4}, []int{2, 2}, "")
		reluLayer.forwardInput = to.CreateWithData(
			[]float64{1, -1, 2, 3}, []int{2, 2}, "",
		)

		output := reluLayer.Backward(input)

		Expect(output.Size()).To(Equal([]int{2, 2}))
		Expect(output.Vector()).To(Equal([]float64{10, 0, 3, 4}))
	})
})
