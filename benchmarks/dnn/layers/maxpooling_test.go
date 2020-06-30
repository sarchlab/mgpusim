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
		stride := [2]int{2, 2}     //[H, W]
		padding := [2]int{0, 0}    //[H, W]
		kernelSize := [2]int{2, 2} //[H, W]
		layer = layers.NewMaxPoolingLayer(stride, padding, kernelSize, gpuDriver, context)
		layer.EnableVerification()
	})

	AfterEach(func() {
		gpuDriver.Terminate()
	})

	// It("should forward", func() {
	// 	input := layers.NewTensor(gpuDriver, context)
	// 	input.Init([]float64{
	// 		1, 2, 3, 4,
	// 		7, 8, 9, 10,
	// 	}, []int{1, 1, 2, 4})

	// 	output := layer.Forward(input)

	// 	Expect(output.Size()).To(Equal([]int{1, 1, 2, 2}))
	// 	Expect(output.Vector()).To(Equal([]float64{
	// 		2, 4,
	// 		8, 10,
	// 	}))
	// })

	It("should backward", func() {
		input := layers.NewTensor(gpuDriver, context)
		input.Init([]float64{
			1, 2, 3, 4,
			7, 8, 9, 10,
		}, []int{1, 1, 2, 4})
		layer.Forward(input)
		//Forward then Backward
		inputB := layers.NewTensor(gpuDriver, context)
		inputB.Init([]float64{
			10, 11,
		}, []int{1, 1, 1, 2})

		output := layer.Backward(inputB)

		Expect(output.Size()).To(Equal([]int{1, 1, 2, 4}))
		Expect(output.Vector()).To(Equal([]float64{
			0, 0, 0, 0,
			0, 10, 0, 11,
		}))
	})

})
