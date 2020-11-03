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

type im2ColDataSet struct {
	Input    tensorJSON `json:"input,omitempty"`
	Output   tensorJSON `json:"output,omitempty"`
	MaskSize [2]int     `json:"mask_size,omitempty"`
	Padding  [4]int     `json:"padding,omitempty"`
	Stride   [2]int     `json:"stride,omitempty"`
	Dilation [2]int     `json:"dilation,omitempty"`
}

func loadIm2ColDatasets(filename string) []im2ColDataSet {
	jsonFile, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	defer jsonFile.Close()

	var im2ColData []im2ColDataSet

	byteValue, _ := ioutil.ReadAll(jsonFile)
	err = json.Unmarshal(byteValue, &im2ColData)
	if err != nil {
		panic(err)
	}

	return im2ColData
}

type backwardDataSet struct {
	StrideSize     []int      `json:"stride_size,omitempty"`
	PaddingSize    []int      `json:"padding_size,omitempty"`
	Kernel         tensorJSON `json:"kernel,omitempty"`
	ForwardInput   tensorJSON `json:"forward_input,omitempty"`
	BackwardInput  tensorJSON `json:"backward_input,omitempty"`
	WeightGradient tensorJSON `json:"weight_gradient,omitempty"`
	InputGradient  tensorJSON `json:"input_gradient,omitempty"`
}

func loadBackwardDatasets(filename string) []backwardDataSet {
	jsonFile, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	defer jsonFile.Close()

	var backwardData []backwardDataSet

	byteValue, _ := ioutil.ReadAll(jsonFile)
	err = json.Unmarshal(byteValue, &backwardData)
	if err != nil {
		panic(err)
	}

	return backwardData
}

var _ = Describe("Convolutional Layer", func() {

	var (
		gpuDriver *driver.Driver
		context   *driver.Context
		to        *TensorOperator
		input     *Tensor
		// kernel *Tensor
		convLayer *Conv2D
	)

	BeforeEach(func() {

		// kernel = NewTensor(gpuDriver, context)
		// ConvLayer = NewConvolutionalLayer([]int{1, 3, 3}, []int{1, 1, 3, 3}, []int{1, 1}, []int{1,1,1,1})

		_, gpuDriver = platform.MakeEmuBuilder().
			// WithISADebugging().
			WithoutProgressBar().
			Build()
		gpuDriver.Run()
		context = gpuDriver.Init()
		to = NewTensorOperator(gpuDriver, context)
		input = NewTensor(gpuDriver, context)

		convLayer = NewConvolutionalLayer(
			[]int{1, 3, 3}, []int{1, 1, 3, 3},
			[]int{1, 1}, []int{1, 1, 1, 1},
			gpuDriver, context, to)

		// ConvLayer.Randomize()

		gpuDriver.MemCopyH2D(context, convLayer.kernel.ptr,
			[]float32{
				1.0, 1.0, 1.0,
				2.0, 2.0, 2.0,
				3.0, 3.0, 3.0,
			})
	})

	It("should do im2col", func() {
		goldDatasets := loadIm2ColDatasets("im2col_test_data.json")

		for _, d := range goldDatasets {
			goldIn := d.Input
			goldOut := d.Output

			input.Init(goldIn.Data, goldIn.Size)
			input.descriptor = goldIn.Descriptor
			output := NewTensor(gpuDriver, context)
			output.Init(
				make([]float64, goldOut.Size[0]*goldOut.Size[1]),
				goldOut.Size)

			convLayer.im2Col(input, output,
				d.MaskSize, d.Padding, d.Stride, d.Dilation)

			outputV := output.Vector()
			for i := range goldOut.Data {
				Expect(outputV[i]).To(BeNumerically("~", goldOut.Data[i], 1e-3))
			}
		}
	})

	It("should forward", func() {
		goldDatasets := loadDatasets("conv_forward_test_data.json")

		for _, d := range goldDatasets {
			goldIn := d["input"]
			goldOut := d["output"]
			goldKernel := d["kernel"]
			goldStride := d["stride"]
			goldPadding := d["padding"]

			inputSize := []int{
				goldIn.Size[1],
				goldIn.Size[2],
				goldIn.Size[3],
			}
			if goldIn.Descriptor == "CNHW" {
				inputSize[0] = goldIn.Size[0]
			}

			layer := NewConvolutionalLayer(
				inputSize,
				goldKernel.Size,
				goldStride.Size,
				goldPadding.Size,
				gpuDriver, context, to)
			layer.kernel.Init(goldKernel.Data, goldKernel.Size)

			input.Init(goldIn.Data, goldIn.Size)
			input.descriptor = goldIn.Descriptor

			output := layer.Forward(input)

			Expect(output.Size()).To(Equal(goldOut.Size))
			Expect(output.Descriptor()).To(Equal(goldOut.Descriptor))

			outputV := output.Vector()
			for i := range outputV {
				Expect(outputV[i]).To(BeNumerically("~", goldOut.Data[i], 1e-3))
			}
		}
	})

	It("should do backward", func() {
		goldDatasets := loadBackwardDatasets("conv_backward_test_data.json")

		for _, d := range goldDatasets {
			forwardIn := d.ForwardInput

			layerInputSize := []int{
				forwardIn.Size[1],
				forwardIn.Size[2],
				forwardIn.Size[3],
			}

			layer := NewConvolutionalLayer(layerInputSize,
				d.Kernel.Size, d.StrideSize, d.PaddingSize,
				gpuDriver, context, to)
			layer.kernel.Init(d.Kernel.Data, d.Kernel.Size)

			forwardInputT := NewTensor(gpuDriver, context)
			forwardInputT.Init(forwardIn.Data, forwardIn.Size)
			forwardInputT.descriptor = forwardIn.Descriptor
			layer.forwardInput = forwardInputT

			backwardIn := d.BackwardInput
			backwardInT := NewTensor(gpuDriver, context)
			backwardInT.Init(backwardIn.Data, backwardIn.Size)
			backwardInT.descriptor = backwardIn.Descriptor

			out := layer.Backward(backwardInT)

			goldWeightGradients := d.WeightGradient
			weightGradients := layer.weightGradients.Raw()
			for i := range weightGradients {
				Expect(weightGradients[i]).
					To(BeNumerically("~", goldWeightGradients.Data[i], 1e-3))
			}

			goldInputGradients := d.InputGradient
			inputGradients := out.Vector()
			for i := range inputGradients {
				Expect(inputGradients[i]).To(
					BeNumerically("~", goldInputGradients.Data[i], 1e-3))
			}
			fmt.Printf("Passed one backward test\n")
		}
	})

	// It("Backward, 1 input channel, 1 output channel, stride 1", func() {
	// 	// ConvLayer = NewConvolutionalLayer([]int{1, 3, 3}, []int{1, 1, 3, 3}, []int{1, 1}, []int{1,1,1,1})

	// 	input.Init([]float64{
	// 		1.0, 1.0, 1.0,
	// 		2.0, 2.0, 2.0,
	// 		3.0, 3.0, 3.0,
	// 	},
	// 		[]int{1, 3, 3})
	// 	cpuOutput := make([]float32, 9)
	// 	convLayer.GPUDriver.MemCopyD2H(convLayer.GPUCtx, cpuOutput, input.ptr)
	// 	fmt.Println("TEST input.ptr: ", cpuOutput, " / ", input.ptr)

	// 	convLayer.Forward(input)

	// 	convLayer.Backward(input)

	// 	Expect(convLayer.inputGradients).To(Equal([]float64{
	// 		8, 12, 8,
	// 		20, 30, 20,
	// 		24, 36, 24,
	// 	}))
	// 	Expect(convLayer.weightGradients).To(Equal([]float64{
	// 		16, 24, 16,
	// 		28, 42, 28,
	// 		16, 24, 16,
	// 	}))

	// 	BGOutput := make([]float32, 3*3)
	// 	convLayer.GPUDriver.MemCopyD2H(
	// 		convLayer.GPUCtx, BGOutput, convLayer.biasGradients.ptr)
	// 	fmt.Println("BGoutput: ", BGOutput)
	// 	// Expect(ConvLayer.biasGradients).To(Equal([]float64{
	// 	// 	12, 14,
	// 	// }))
	// 	// Expect(output.Size()).To(Equal([]int{1, 3, 3}))
	// })

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
