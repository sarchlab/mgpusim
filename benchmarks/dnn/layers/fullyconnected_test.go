package layers

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/platform"
)

var _ = Describe("Fully Connected Layer", func() {
	var (
		gpuDriver *driver.Driver
		context   *driver.Context
		mo        *MatrixOperator
		layer     *FullyConnectedLayer
	)

	BeforeEach(func() {
		_, gpuDriver = platform.MakeR9NanoBuilder().Build()
		gpuDriver.Run()
		context = gpuDriver.Init()
		mo = NewMatrixOperator(gpuDriver, context)

		layer = NewFullyConnectedLayer(4, 2, gpuDriver, context, mo)
		layer.Randomize()

		gpuDriver.MemCopyH2D(context, layer.weight.ptr,
			[]float32{
				1, 2,
				3, 4,
				5, 6,
				7, 8,
			})
		gpuDriver.MemCopyH2D(context, layer.bias.ptr,
			[]float32{
				10, 11,
			})
	})

	It("should forward", func() {
		input := NewTensor(gpuDriver, context)
		input.Init([]float64{
			1, 2, 3, 4,
			5, 6, 7, 8,
		}, []int{2, 4})

		output := layer.Forward(input)

		Expect(output.Size()).To(Equal([]int{2, 2}))
		Expect(output.Vector()).To(Equal([]float64{60, 71, 124, 151}))
	})

	It("should backward", func() {
		forwardInput := NewTensor(gpuDriver, context)
		forwardInput.Init([]float64{
			1, 2, 3, 4,
			5, 6, 7, 8,
		}, []int{2, 4})
		layer.Forward(forwardInput)

		backwardInput := NewTensor(gpuDriver, context)
		backwardInput.Init([]float64{
			5, 6,
			7, 8,
		}, []int{2, 2})

		backwardOutput := layer.Backward(backwardInput)

		Expect(backwardOutput.Size()).To(Equal([]int{2, 4}))
		Expect(backwardOutput.Vector()).To(Equal([]float64{
			17, 39, 61, 83,
			23, 53, 83, 113,
		}))
		Expect(layer.weightGradients.Raw()).To(Equal([]float64{
			40, 46,
			52, 60,
			64, 74,
			76, 88,
		}))
		Expect(layer.biasGradients.Raw()).To(Equal([]float64{
			12, 14,
		}))
	})

})
