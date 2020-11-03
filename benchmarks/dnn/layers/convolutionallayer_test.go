package layers

import (
	"encoding/json"
	"io/ioutil"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

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

type forwardDataset struct {
	StrideSize  []int      `json:"stride_size,omitempty"`
	PaddingSize []int      `json:"padding_size,omitempty"`
	Kernel      tensorJSON `json:"kernel,omitempty"`
	Bias        tensorJSON `json:"bias,omitempty"`
	Input       tensorJSON `json:"input,omitempty"`
	Output      tensorJSON `json:"output,omitempty"`
}

func loadForwardDatasets(filename string) []forwardDataset {
	jsonFile, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	defer jsonFile.Close()

	var forwardData []forwardDataset

	byteValue, _ := ioutil.ReadAll(jsonFile)
	err = json.Unmarshal(byteValue, &forwardData)
	if err != nil {
		panic(err)
	}

	return forwardData
}

type backwardDataset struct {
	StrideSize     []int      `json:"stride_size,omitempty"`
	PaddingSize    []int      `json:"padding_size,omitempty"`
	Kernel         tensorJSON `json:"kernel,omitempty"`
	ForwardInput   tensorJSON `json:"forward_input,omitempty"`
	BackwardInput  tensorJSON `json:"backward_input,omitempty"`
	WeightGradient tensorJSON `json:"weight_gradient,omitempty"`
	InputGradient  tensorJSON `json:"input_gradient,omitempty"`
	BiasGradient   tensorJSON `json:"bias_gradient,omitempty"`
}

func loadBackwardDatasets(filename string) []backwardDataset {
	jsonFile, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	defer jsonFile.Close()

	var backwardData []backwardDataset

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
		convLayer *Conv2D
	)

	BeforeEach(func() {
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
		goldDatasets := loadForwardDatasets("conv_forward_test_data.json")

		for _, d := range goldDatasets {
			goldIn := d.Input
			goldOut := d.Output
			goldKernel := d.Kernel
			goldBias := d.Bias
			goldStride := d.StrideSize
			goldPadding := d.PaddingSize

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
				goldStride,
				goldPadding,
				gpuDriver, context, to)
			layer.kernel.Init(goldKernel.Data, goldKernel.Size)
			layer.bias.Init(goldBias.Data, goldBias.Size)

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

			goldBiasGradients := d.BiasGradient
			biasGradients := layer.biasGradients.Vector()
			for i := range biasGradients {
				Expect(biasGradients[i]).To(
					BeNumerically("~", goldBiasGradients.Data[i], 1e-3))
			}
		}
	})
})
