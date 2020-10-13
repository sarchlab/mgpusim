package layers

import (
	// "fmt"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

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
		convLayer *Conv2D
	)

	BeforeEach(func() {

		// kernel = NewTensor(gpuDriver, context)
		// ConvLayer = NewConvolutionalLayer([]int{1, 3, 3}, []int{1, 1, 3, 3}, []int{1, 1}, []int{1,1,1,1})

		_, gpuDriver = platform.MakeEmuBuilder().
			WithISADebugging().
			WithoutProgressBar().Build()
		gpuDriver.Run()
		context = gpuDriver.Init()
		mo = NewMatrixOperator(gpuDriver, context)
		input = NewTensor(gpuDriver, context)

		convLayer = NewConvolutionalLayer(
			[]int{1, 3, 3}, []int{1, 1, 3, 3},
			[]int{1, 1}, []int{1, 1, 1, 1},
			gpuDriver, context, mo)

		// ConvLayer.Randomize()

		gpuDriver.MemCopyH2D(context, convLayer.kernel.ptr,
			[]float32{
				1.0, 1.0, 1.0,
				2.0, 2.0, 2.0,
				3.0, 3.0, 3.0,
			})
	})

	FIt("should do im2col, single channel, single batch", func() {
		jsonFile, err := os.Open("im2col_test_data.json")
		if err != nil {
			fmt.Println(err)
		}
		defer jsonFile.Close()

		type dataJSON struct {
			Data []float64
			Size []int
		}

		type inputOutputPair struct {
			Input  dataJSON
			Output dataJSON
		}

		var pairs []inputOutputPair

		byteValue, _ := ioutil.ReadAll(jsonFile)
		json.Unmarshal(byteValue, &pairs)

		fmt.Println(pairs)

		for _, p := range pairs {
			input.Init(p.Input.Data, p.Input.Size)
			output := NewTensor(gpuDriver, context)
			output.Init(
				make([]float64, p.Output.Size[0]*p.Output.Size[1]),
				p.Output.Size)

			convLayer.im2col(input.ptr, output.ptr, p.Input.Size[1], p.Input.Size[0], p.Output.Size[1])

			Expect(output.Vector()).To(Equal(p.Output.Data))
		}
	})

	It("should do im2col, multi channel, multi batch", func() {
		input.Init([]float64{
			111, 111, 111, 112, 112, 122, 113, 113, 113, 121, 121, 121, 122, 122,
			122, 123, 123, 123, 131, 131, 131, 132, 132, 132, 133, 133, 133, 211,
			211, 211, 212, 212, 222, 213, 213, 213, 221, 221, 221, 222, 222, 222,
			223, 223, 223, 231, 231, 231, 232, 232, 232, 233, 233, 233,
		}, []int{1, 3, 3})

		output := NewTensor(gpuDriver, context)
		output.Init(make([]float64, 27*18), []int{27, 18})

		convLayer.im2col(input.ptr, output.ptr, 3, 2, 18)

		Expect(output.Vector()).To(Equal([]float64{
			0.00, 0.00, 0.00, 0.00, 111.00, 111.00, 0.00, 112.00, 112.00, 0.00, 0.00, 0.00, 0.00, 211.00, 211.00, 0.00, 212.00, 212.00,
			0.00, 0.00, 0.00, 111.00, 111.00, 111.00, 112.00, 112.00, 122.00, 0.00, 0.00, 0.00, 211.00, 211.00, 211.00, 212.00, 212.00, 222.00,
			0.00, 0.00, 0.00, 111.00, 111.00, 0.00, 112.00, 122.00, 0.00, 0.00, 0.00, 0.00, 211.00, 211.00, 0.00, 212.00, 222.00, 0.00,
			0.00, 111.00, 111.00, 0.00, 112.00, 112.00, 0.00, 113.00, 113.00, 0.00, 211.00, 211.00, 0.00, 212.00, 212.00, 0.00, 213.00, 213.00,
			111.00, 111.00, 111.00, 112.00, 112.00, 122.00, 113.00, 113.00, 113.00, 211.00, 211.00, 211.00, 212.00, 212.00, 222.00, 213.00, 213.00, 213.00,
			111.00, 111.00, 0.00, 112.00, 122.00, 0.00, 113.00, 113.00, 0.00, 211.00, 211.00, 0.00, 212.00, 222.00, 0.00, 213.00, 213.00, 0.00,
			0.00, 112.00, 112.00, 0.00, 113.00, 113.00, 0.00, 0.00, 0.00, 0.00, 212.00, 212.00, 0.00, 213.00, 213.00, 0.00, 0.00, 0.00,
			112.00, 112.00, 122.00, 113.00, 113.00, 113.00, 0.00, 0.00, 0.00, 212.00, 212.00, 222.00, 213.00, 213.00, 213.00, 0.00, 0.00, 0.00,
			112.00, 122.00, 0.00, 113.00, 113.00, 0.00, 0.00, 0.00, 0.00, 212.00, 222.00, 0.00, 213.00, 213.00, 0.00, 0.00, 0.00, 0.00,
			0.00, 0.00, 0.00, 0.00, 121.00, 121.00, 0.00, 122.00, 122.00, 0.00, 0.00, 0.00, 0.00, 221.00, 221.00, 0.00, 222.00, 222.00,
			0.00, 0.00, 0.00, 121.00, 121.00, 121.00, 122.00, 122.00, 122.00, 0.00, 0.00, 0.00, 221.00, 221.00, 221.00, 222.00, 222.00, 222.00,
			0.00, 0.00, 0.00, 121.00, 121.00, 0.00, 122.00, 122.00, 0.00, 0.00, 0.00, 0.00, 221.00, 221.00, 0.00, 222.00, 222.00, 0.00,
			0.00, 121.00, 121.00, 0.00, 122.00, 122.00, 0.00, 123.00, 123.00, 0.00, 221.00, 221.00, 0.00, 222.00, 222.00, 0.00, 223.00, 223.00,
			121.00, 121.00, 121.00, 122.00, 122.00, 122.00, 123.00, 123.00, 123.00, 221.00, 221.00, 221.00, 222.00, 222.00, 222.00, 223.00, 223.00, 223.00,
			121.00, 121.00, 0.00, 122.00, 122.00, 0.00, 123.00, 123.00, 0.00, 221.00, 221.00, 0.00, 222.00, 222.00, 0.00, 223.00, 223.00, 0.00,
			0.00, 122.00, 122.00, 0.00, 123.00, 123.00, 0.00, 0.00, 0.00, 0.00, 222.00, 222.00, 0.00, 223.00, 223.00, 0.00, 0.00, 0.00,
			122.00, 122.00, 122.00, 123.00, 123.00, 123.00, 0.00, 0.00, 0.00, 222.00, 222.00, 222.00, 223.00, 223.00, 223.00, 0.00, 0.00, 0.00,
			122.00, 122.00, 0.00, 123.00, 123.00, 0.00, 0.00, 0.00, 0.00, 222.00, 222.00, 0.00, 223.00, 223.00, 0.00, 0.00, 0.00, 0.00,
			0.00, 0.00, 0.00, 0.00, 131.00, 131.00, 0.00, 132.00, 132.00, 0.00, 0.00, 0.00, 0.00, 231.00, 231.00, 0.00, 232.00, 232.00,
			0.00, 0.00, 0.00, 131.00, 131.00, 131.00, 132.00, 132.00, 132.00, 0.00, 0.00, 0.00, 231.00, 231.00, 231.00, 232.00, 232.00, 232.00,
			0.00, 0.00, 0.00, 131.00, 131.00, 0.00, 132.00, 132.00, 0.00, 0.00, 0.00, 0.00, 231.00, 231.00, 0.00, 232.00, 232.00, 0.00,
			0.00, 131.00, 131.00, 0.00, 132.00, 132.00, 0.00, 133.00, 133.00, 0.00, 231.00, 231.00, 0.00, 232.00, 232.00, 0.00, 233.00, 233.00,
			131.00, 131.00, 131.00, 132.00, 132.00, 132.00, 133.00, 133.00, 133.00, 231.00, 231.00, 231.00, 232.00, 232.00, 232.00, 233.00, 233.00, 233.00,
			131.00, 131.00, 0.00, 132.00, 132.00, 0.00, 133.00, 133.00, 0.00, 231.00, 231.00, 0.00, 232.00, 232.00, 0.00, 233.00, 233.00, 0.00,
			0.00, 132.00, 132.00, 0.00, 133.00, 133.00, 0.00, 0.00, 0.00, 0.00, 232.00, 232.00, 0.00, 233.00, 233.00, 0.00, 0.00, 0.00,
			132.00, 132.00, 132.00, 133.00, 133.00, 133.00, 0.00, 0.00, 0.00, 232.00, 232.00, 232.00, 233.00, 233.00, 233.00, 0.00, 0.00, 0.00,
			132.00, 132.00, 0.00, 133.00, 133.00, 0.00, 0.00, 0.00, 0.00, 232.00, 232.00, 0.00, 233.00, 233.00, 0.00, 0.00, 0.00, 0.00,
		}))

	})

	It("Forward, 1 input channel, 1 output channel, stride 1", func() {
		// ConvLayer = NewConvolutionalLayer([]int{1, 3, 3}, []int{1, 1, 3, 3}, []int{1, 1}, []int{1,1,1,1})

		input.Init([]float64{
			1.0, 1.0, 1.0,
			2.0, 2.0, 2.0,
			3.0, 3.0, 3.0,
		}, []int{1, 3, 3})

		output := convLayer.Forward(input)

		// fmt.Println(ConvLayer.inputWithPadding)

		Expect(output.Size()).To(Equal([]int{1, 3, 3}))
		Expect(output.Vector()).To(Equal([]float64{16, 24, 16, 28, 42, 28, 16, 24, 16}))
		Expect(convLayer.forwardInput).To(Equal(input.Vector()))
	})

	It("Backward, 1 input channel, 1 output channel, stride 1", func() {
		// ConvLayer = NewConvolutionalLayer([]int{1, 3, 3}, []int{1, 1, 3, 3}, []int{1, 1}, []int{1,1,1,1})

		input.Init([]float64{
			1.0, 1.0, 1.0,
			2.0, 2.0, 2.0,
			3.0, 3.0, 3.0,
		},
			[]int{1, 3, 3})
		cpuOutput := make([]float32, 9)
		convLayer.GPUDriver.MemCopyD2H(convLayer.GPUCtx, cpuOutput, input.ptr)
		fmt.Println("TEST input.ptr: ", cpuOutput, " / ", input.ptr)

		convLayer.Forward(input)

		convLayer.Backward(input)

		Expect(convLayer.inputGradients).To(Equal([]float64{
			8, 12, 8,
			20, 30, 20,
			24, 36, 24,
		}))
		Expect(convLayer.weightGradients).To(Equal([]float64{
			16, 24, 16,
			28, 42, 28,
			16, 24, 16,
		}))

		BGOutput := make([]float32, 3*3)
		convLayer.GPUDriver.MemCopyD2H(
			convLayer.GPUCtx, BGOutput, convLayer.biasGradients.ptr)
		fmt.Println("BGoutput: ", BGOutput)
		// Expect(ConvLayer.biasGradients).To(Equal([]float64{
		// 	12, 14,
		// }))
		// Expect(output.Size()).To(Equal([]int{1, 3, 3}))
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
