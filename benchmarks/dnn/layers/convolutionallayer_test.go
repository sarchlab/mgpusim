package layers

import (
	// "fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	// "gitlab.com/akita/dnn/tensor"
	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/platform"
)

var _ = Describe("Convolutional Layer", func() {

	var (
		gpuDriver *driver.Driver
		context   *driver.Context
		mo        *MatrixOperator
		input     *Tensor
		// kernel *Tensor
		ConvLayer *Conv2D
	)

	BeforeEach(func() {

		// kernel = NewTensor(gpuDriver, context)
		// ConvLayer = NewConvolutionalLayer([]int{1, 3, 3}, []int{1, 1, 3, 3}, []int{1, 1}, []int{1,1,1,1})

		_, gpuDriver = platform.MakeEmuBuilder().WithoutProgressBar().Build()
		gpuDriver.Run()
		context = gpuDriver.Init()
		mo = NewMatrixOperator(gpuDriver, context)
		input = NewTensor(gpuDriver, context)

		ConvLayer = NewConvolutionalLayer(
			[]int{1, 3, 3}, []int{1, 1, 3, 3},
			[]int{1, 1}, []int{1, 1, 1, 1},
			gpuDriver, context, mo)

		// ConvLayer.Randomize()

		gpuDriver.MemCopyH2D(context, ConvLayer.kernel.ptr,
			[]float64{
				1.0, 1.0, 1.0,
				2.0, 2.0, 2.0,
				3.0, 3.0, 3.0,
			})
	})

	It("Forward, 1 input channel, 1 output channel, stride 1", func() {
		// ConvLayer = NewConvolutionalLayer([]int{1, 3, 3}, []int{1, 1, 3, 3}, []int{1, 1}, []int{1,1,1,1})

		input.Init([]float64{
			1.0, 1.0, 1.0,
			2.0, 2.0, 2.0,
			3.0, 3.0, 3.0,
		},
			[]int{1, 3, 3})

		output := ConvLayer.Forward(input)

		// fmt.Println(ConvLayer.inputWithPadding)

		Expect(output.Size()).To(Equal([]int{1, 3, 3}))
		Expect(output.Vector()).To(Equal([]float64{16, 24, 16, 28, 42, 28, 16, 24, 16}))
		Expect(ConvLayer.forwardInput).To(Equal(input.Vector()))
	})

	It("Backward, 1 input channel, 1 output channel, stride 1", func() {
		// ConvLayer = NewConvolutionalLayer([]int{1, 3, 3}, []int{1, 1, 3, 3}, []int{1, 1}, []int{1,1,1,1})

		input.Init([]float64{
			1.0, 1.0, 1.0,
			2.0, 2.0, 2.0,
			3.0, 3.0, 3.0,
		},
			[]int{1, 3, 3})

		output := ConvLayer.Forward(input)

		ConvLayer.Backward(input)

		Expect(ConvLayer.inputGradients).To(Equal([]float64{
			8, 12, 8,
			20, 30, 20,
			24, 36, 24,
		}))
		Expect(ConvLayer.weightGradients).To(Equal([]float64{
			16, 24, 16,
			28, 42, 28,
			16, 24, 16,
		}))
		// Expect(ConvLayer.biasGradients).To(Equal([]float64{
		// 	12, 14,
		// }))
		Expect(output.Size()).To(Equal([]int{1, 3, 3}))
	})
	// It("Forward, 2 input channel, 1 output channel, stride 1", func() {
	// 	ConvLayer = NewConvolutionalLayer([]int{2, 3, 3}, []int{1, 2, 3, 3}, []int{1, 1}, []int{1,1,1,1})

	// 	kernel.Init([]float64{
	// 		1.0, 1.0, 1.0,
	// 		2.0, 2.0, 2.0,
	// 		3.0, 3.0, 3.0,

	// 		1.0, 1.0, 1.0,
	// 		2.0, 2.0, 2.0,
	// 		3.0, 3.0, 3.0,
	// 	}, []int{1,2,3,3})
	// 	ConvLayer.SetKernel(kernel)

	// 	input.Init([]float64{
	// 		1.0, 1.0, 1.0,
	// 		2.0, 2.0, 2.0,
	// 		3.0, 3.0, 3.0,

	// 		1.0, 1.0, 1.0,
	// 		2.0, 2.0, 2.0,
	// 		3.0, 3.0, 3.0,
	// 	 },
	// 	[]int{2, 3, 3})

	// 	output := ConvLayer.Forward(input)

	// 	// fmt.Println(ConvLayer.inputWithPadding)

	// 	Expect(output.Size()).To(Equal([]int{1, 3, 3}))
	// 	Expect(output.Vector()).To(Equal([]float64{32, 48, 32, 56, 84, 56, 32, 48, 32,}))
	// 	Expect(ConvLayer.forwardInput).To(Equal(input.Vector()))
	// })

	// It("Backward, 2 input channel, 1 output channel, stride 1", func() {
	// 	ConvLayer = NewConvolutionalLayer([]int{2, 3, 3}, []int{1, 2, 3, 3}, []int{1, 1}, []int{1,1,1,1})
	// 	kernel.Init([]float64{
	// 		1.0, 1.0, 1.0,
	// 		2.0, 2.0, 2.0,
	// 		3.0, 3.0, 3.0,

	// 		1.0, 1.0, 1.0,
	// 		2.0, 2.0, 2.0,
	// 		3.0, 3.0, 3.0,
	// 	}, []int{1,2,3,3})
	// 	ConvLayer.SetKernel(kernel)

	// 	input.Init([]float64{
	// 		1.0, 1.0, 1.0,
	// 		2.0, 2.0, 2.0,
	// 		3.0, 3.0, 3.0,

	// 		1.0, 1.0, 1.0,
	// 		2.0, 2.0, 2.0,
	// 		3.0, 3.0, 3.0,
	// 	 },
	// 	[]int{2, 3, 3})

	// 	output := ConvLayer.Forward(input)

	// 	output = ConvLayer.Backward(input)

	// 	Expect(ConvLayer.inputGradient).To(Equal([]float64{
	// 		 8, 12,  8,
	// 		20, 30, 20,
	// 		24, 36, 24,

	// 		8, 12,  8,
	// 		20, 30, 20,
	// 		24, 36, 24,
	// 	}))
	// 	Expect(ConvLayer.weightGradient).To(Equal([]float64{
	// 		16, 24, 16,
	// 		28, 42, 28,
	// 		16, 24, 16,

	// 		16, 24, 16,
	// 		28, 42, 28,
	// 		16, 24, 16,
	// 	}))
	// 	// Expect(ConvLayer.biasGradients).To(Equal([]float64{
	// 	// 	12, 14,
	// 	// }))
	// 	Expect(output.Size()).To(Equal([]int{2, 3, 3}))
	// })

	// It("Forward + Backward, 1 input channel, 1 output channel, stride 2", func() {
	// 	ConvLayer = NewConvolutionalLayer([]int{1, 4, 4}, []int{1, 1, 2, 2}, []int{2, 2}, []int{0,0,0,0})

	// 	kernel.Init([]float64{
	// 		1.0, 1.0,
	// 		2.0, 2.0,
	// 	}, []int{1,1,2,2})
	// 	ConvLayer.SetKernel(kernel)

	// 	input.Init([]float64{
	// 		1.0, 1.0, 1.0, 1.0,
	// 		2.0, 2.0, 2.0, 2.0,
	// 		3.0, 3.0, 3.0, 3.0,
	// 		4.0, 4.0, 4.0, 4.0,
	// 	 },
	// 	[]int{1, 4, 4})

	// 	output := ConvLayer.Forward(input)

	// 	// fmt.Println(ConvLayer.inputWithPadding)

	// 	Expect(output.Size()).To(Equal([]int{1, 2, 2}))
	// 	Expect(output.Vector()).To(Equal([]float64{
	// 		10, 10,
	// 		22, 22,
	// 	}))

	// 	input.Init([]float64{
	// 		1.0, 1.0,
	// 		2.0, 2.0,
	// 	 },
	// 	[]int{1, 2, 2})

	// 	output = ConvLayer.Backward(input)

	// 	Expect(output.Size()).To(Equal([]int{1, 4, 4}))
	// 	Expect(ConvLayer.inputGradient).To(Equal([]float64{
	// 		1, 1, 1, 1,
	// 		2, 2, 2, 2,
	// 		2, 2, 2, 2,
	// 		4, 4, 4, 4,
	//    }))
	//    Expect(ConvLayer.weightGradient).To(Equal([]float64{
	// 		10, 10,
	// 		22, 22,
	//    }))
	// })

})
