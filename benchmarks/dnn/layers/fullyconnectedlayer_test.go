package layers

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/tensor"
)

var _ = Describe("Fully Connected Layer", func() {

	var (
		input   tensor.Tensor
		to      *tensor.CPUOperator
		fcLayer *FullyConnectedLayer
	)

	BeforeEach(func() {
		to = &tensor.CPUOperator{}
		fcLayer = NewFullyConnectedLayer(1, to, 4, 2)
	})

	It("should forward", func() {
		to.Init(fcLayer.weights, []float64{
			1, 2,
			3, 4,
			5, 6,
			7, 8,
		})
		to.Init(fcLayer.bias, []float64{10, 11})

		input = to.CreateWithData([]float64{
			1, 2, 3, 4,
			5, 6, 7, 8,
		}, []int{2, 4}, "")

		output := fcLayer.Forward(input)

		Expect(output.Size()).To(Equal([]int{2, 2}))
		Expect(output.Vector()).To(Equal([]float64{60, 71, 124, 151}))
		Expect(fcLayer.forwardInput).To(Equal(input))
	})

	It("should backward", func() {
		to.Init(fcLayer.weights, []float64{
			1, 2,
			3, 4,
			5, 6,
			7, 8,
		})
		to.Init(fcLayer.bias, []float64{10, 11})
		fcLayer.forwardInput = to.CreateWithData(
			[]float64{
				1, 2, 3, 4,
				5, 6, 7, 8,
			}, []int{2, 4}, "",
		)
		input = to.CreateWithData([]float64{
			5, 6,
			7, 8,
		}, []int{2, 2}, "")

		output := fcLayer.Backward(input)

		Expect(output.Size()).To(Equal([]int{2, 4}))
		Expect(output.Vector()).To(Equal([]float64{
			17, 39, 61, 83,
			23, 53, 83, 113,
		}))
		Expect(fcLayer.weightGradients.Vector()).To(Equal([]float64{
			40, 46,
			52, 60,
			64, 74,
			76, 88,
		}))
		Expect(fcLayer.biasGradients.Vector()).To(Equal([]float64{
			12, 14,
		}))
	})
})
