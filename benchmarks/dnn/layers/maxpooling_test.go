package layers_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.com/akita/mgpusim/benchmarks/dnn/layers"

	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/platform"
)

var _ = Describe("Max Pooling Layer", func() {
	var (
		gpuDriver *driver.Driver
		context   *driver.Context
		layer     *layers.MaxPoolingLayer
	)

	BeforeEach(func() {
		_, gpuDriver = platform.MakeEmuBuilder().
			WithNumGPU(1).
			WithoutProgressBar().
			//WithISADebugging().
			Build()
		gpuDriver.Run()
		context = gpuDriver.Init()
		stride := [2]int{2, 2}     //[H, W]
		padding := [2]int{1, 0}    //[H, W]
		kernelSize := [2]int{2, 2} //[H, W]
		layer = layers.NewMaxPoolingLayer(stride, padding, kernelSize, gpuDriver, context)
		layer.EnableVerification()
	})

	AfterEach(func() {
		gpuDriver.Terminate()
	})

	It("should forward", func() {
		input := layers.NewTensor(gpuDriver, context)
		input.Init([]float64{
			1, 2, 3, 4,
			50, 6.6, 7, 8.8,
			2, 3, 40, 5,
			11, 12, 13, 14,
		}, []int{1, 2, 2, 4})

		output := layer.Forward(input)

		Expect(output.Size()).To(Equal([]int{1, 2, 2, 2})) //Batch * Channel * Height * Width
		expectedOutput := []float64{
			2, 4,
			50, 8.8,
			3, 40,
			12, 14,
		}
		for i := range expectedOutput {
			Expect(output.Vector()[i]).To(BeNumerically("~", expectedOutput[i], 0.01))
		}
	})

	It("should backward", func() {
		input := layers.NewTensor(gpuDriver, context)
		input.Init([]float64{
			1, 2, 3, 4,
			5, 6, 7, 8,
			2, 3, 4, 5,
			11, 12, 13, 14,
		}, []int{1, 2, 2, 4})
		layer.Forward(input)

		// Forward then Backward
		inputB := layers.NewTensor(gpuDriver, context)
		inputB.Init([]float64{
			3, 4,
			5, 6,
			10, 11,
			12, 13,
		}, []int{1, 2, 2, 2})

		output := layer.Backward(inputB)

		Expect(output.Size()).To(Equal([]int{1, 2, 2, 4}))
		expectedOutput := []float64{
			0, 3, 0, 4,
			0, 5, 0, 6,
			0, 10, 0, 11,
			0, 12, 0, 13,
		}
		for i := range expectedOutput {
			Expect(output.Vector()[i]).To(BeNumerically("~", expectedOutput[i], 0.01))
		}
	})
})
