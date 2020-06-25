package layers_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.com/akita/mgpusim/benchmarks/dnn/layers"

	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/platform"
)

var _ = FDescribe("Max Pooling Layer", func() {
	var (
		gpuDriver *driver.Driver
		context   *driver.Context
		layer     *layers.MaxPoolingLayer
	)

	BeforeEach(func() {
		_, gpuDriver = platform.MakeEmuBuilder().
			WithoutProgressBar().
			WithISADebugging().
			Build()
		gpuDriver.Run()
		context = gpuDriver.Init()
		stride := [2]int{2, 2}
		padding := [2]int{0, 0}
		kernelSize := [2]int{2, 2}
		layer = layers.NewMaxPoolingLayer(stride, padding, kernelSize, gpuDriver, context)
		layer.EnableVerification()
	})

	AfterEach(func() {
		gpuDriver.Terminate()
	})

	It("should forward", func() {
		input := layers.NewTensor(gpuDriver, context)
		input.Init([]float64{
			1, 2, 3, 10,
			7, 8, 9, 4,
		}, []int{1, 1, 2, 4})

		output := layer.Forward(input)

		Expect(output.Size()).To(Equal([]int{1, 1, 1, 2}))
		Expect(output.Vector()).To(Equal([]float64{8, 10}))
	})

})
