package layers_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.com/akita/mgpusim/benchmarks/dnn/layers"
	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/platform"
)

var _ = Describe("Relulayer", func() {
	var (
		gpuDriver *driver.Driver
		context   *driver.Context
		layer     *layers.ReluLayer
	)

	BeforeEach(func() {
		_, gpuDriver = platform.MakeR9NanoBuilder().Build()
		gpuDriver.Run()
		context = gpuDriver.Init()

		layer = layers.NewReluLayer(gpuDriver, context)
	})

	It("should forward", func() {
		input := layers.NewTensor(gpuDriver, context)
		input.Init([]float64{
			2, 3, -4,
			5, -6, 7,
		}, []int{2, 3})

		output := layer.Forward(input)
		outputV := output.Vector()

		expectedOutput := []float64{
			2, 3, 0,
			5, 0, 7,
		}

		Expect(output.Size()).To(Equal([]int{2, 3}))
		for i := range expectedOutput {
			Expect(outputV[i]).To(BeNumerically("~", expectedOutput[i], 0.01))
		}
	})

	It("should backward", func() {
		input := layers.NewTensor(gpuDriver, context)
		input.Init([]float64{
			2, 3, -4,
			5, -6, 7,
		}, []int{2, 3})
		layer.Forward(input)

		backInput := layers.NewTensor(gpuDriver, context)
		backInput.Init([]float64{
			-2, 3, 8,
			7, 6, 3,
		}, []int{2, 3})
		backOutput := layer.Backward(backInput)

		expectedOutput := []float64{
			-2, 3, 0,
			7, 0, 3,
		}

		Expect(backOutput.Size()).To(Equal([]int{2, 3}))
		backOutputV := backOutput.Vector()
		for i := range expectedOutput {
			Expect(backOutputV[i]).To(
				BeNumerically("~", expectedOutput[i], 0.01))
		}
	})
})
