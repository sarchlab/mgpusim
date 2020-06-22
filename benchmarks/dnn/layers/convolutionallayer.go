package layers

import (

	"math/rand"

	"gitlab.com/akita/dnn/tensor"

	// "fmt"
)

// A Conv2D can perform convolution operation on input data.
type Conv2D struct {
	inputSize, outputSize []int
	kernelSize            []int
	stride                []int
	padding               []int

	kernel       []float64
	parameters   []float64
	gradients    []float64
	forwardInput []float64
	inputWithPadding []float64
	weightGradient []float64
	inputGradient []float64
	outputGradient []float64
}

// NewConvolutionalLayer creates a new convolutional layer for the DNN network.
// The inputSize should be a 3-number array representing [channel, height,
// width]. The kernel size should be a 4-number array, representing [output
// channel, input channel, height, width]. Stride is a 2-number array
// representing [vertical stride, horizontal stride]. Padding is a 4-number
// array, representing [top padding, right padding, bottom padding, left
// padding].
func NewConvolutionalLayer(
	inputSize, kernelSize, stride, padding []int,
) *Conv2D {
	argumentsMustBeValid(inputSize, kernelSize, stride, padding)

	l := &Conv2D{
		inputSize:  inputSize,
		kernelSize: kernelSize,
		stride:     stride,
		padding:    padding,
	}
	l.calculateOutputSize()

	return l
}

func (l *Conv2D) calculateOutputSize() {
	width := (l.inputSize[2]-l.kernelSize[3]+l.padding[1]+l.padding[3])/
		l.stride[1] + 1
	height := (l.inputSize[1]-l.kernelSize[2]+l.padding[0]+l.padding[2])/
		l.stride[0] + 1
	channel := l.kernelSize[0]
	l.outputSize = []int{channel, height, width}
}

func argumentsMustBeValid(inputSize, kernelSize, stride, padding []int) {
	inputOutputMustBe3D(inputSize)
	kernelMustBe4D(kernelSize)
	inputChannelMustMatchKernelChannel(inputSize, kernelSize)
	inputImageShouldNotBeSmallerThanKernel(inputSize, kernelSize)
}

func inputOutputMustBe3D(size []int) {
	if len(size) != 3 {
		panic("input or output must be 3D (channel, height, width).")
	}
}

func kernelMustBe4D(size []int) {
	if len(size) != 4 {
		panic("kernel must be 4D (out channel, in channel, height, width)")
	}
}

func inputChannelMustMatchKernelChannel(inputSize, kernelSize []int) {
	if inputSize[0] != kernelSize[1] {
		panic("input channel size does not match the 2nd dimension of the kernel.")
	}
}

func strideMustBe2D(stride []int) {
	if len(stride) != 2 {
		panic("stride must be 2D (vertical stride, horizontal stride)")
	}
}

func paddingMustBeHave4Numbers(padding []int) {
	if len(padding) != 4 {
		panic("stride must have 4 numbers (top, right, bottom, left)")
	}
}

func inputImageShouldNotBeSmallerThanKernel(inputSize, kernelSize []int) {
	if inputSize[1] < kernelSize[2] {
		panic("input height is smaller than kernel height")
	}

	if inputSize[2] < kernelSize[3] {
		panic("input width is smaller than kernel width")
	}
}

func (l *Conv2D) numInputChannel() int {
	return l.inputSize[0]
}

func (l *Conv2D) numOutputChannel() int {
	return l.outputSize[0]
}

func (l *Conv2D) Randomize() {
	numParameters := l.kernelSize[0] * l.kernelSize[1] *
		l.kernelSize[2] * l.kernelSize[3]
	l.parameters = make([]float64, numParameters)

	bound := float64(1.0 / (l.kernelSize[2] * l.kernelSize[3]))
	for i := 0; i < numParameters; i++ {
		l.parameters[i] = rand.Float64() * bound
	}
	l.kernel = l.parameters
}

func (l *Conv2D) Forward(inputTensor tensor.Tensor) tensor.Tensor {
	// TODO: check input condition
	if (inputTensor.Size()[0] != l.inputSize[0] || 
		inputTensor.Size()[1] != l.inputSize[1] || 
		inputTensor.Size()[2] != l.inputSize[2]) {
			panic("input dimension not correct")
		}
	input := inputTensor.(*tensor.SimpleTensor)
	output := &tensor.SimpleTensor{}

	l.saveForwardInput(input)
	// l.AddInputPadding()
	l.inputWithPadding = AddPadding(l.forwardInput, l.inputSize, l.padding, []int{1,1})

	outputHeight := l.outputSize[1]
	outputWidth := l.outputSize[2]
	outputChannelSize := outputHeight * outputWidth
	outputTotalSize := l.outputSize[0] * outputChannelSize
	
	cpuOutput := make([]float64, outputTotalSize)

	inputHeight := l.inputSize[1] + l.padding[0] + l.padding[2]
	inputWidth := l.inputSize[2] + l.padding[1] + l.padding[3]
	for m := 0; m < l.outputSize[0]; m++ { // for each output channel
		for h := 0; h < inputHeight; h += l.stride[0] {
			for w := 0; w < inputWidth; w += l.stride[1] { //for each input element
				if (h > inputHeight - l.kernelSize[2] || w > inputWidth - l.kernelSize[3]){
					break
				}
				outputIndex := m * (outputChannelSize) + h * (outputWidth) / l.stride[0] + w / l.stride[1]
				cpuOutput[outputIndex] = l.ApplyKernel(m, h, w)
				
			}
		}
	}
	output.Init(cpuOutput, l.outputSize)
	return output
}

func (l *Conv2D) Backward(inputTensor tensor.Tensor) tensor.Tensor {
	//TODO: add bias

	for i := range l.gradients {
		l.gradients[i] = 0
	}
	input := inputTensor.(*tensor.SimpleTensor)
	output := &tensor.SimpleTensor{}

	l.calculateWeightGradients(input)
	// l.calculateBiasGradients(input)
	l.calculateInputGradients(input)
	output.Init(l.inputGradient, l.inputSize)
	return output
}

func (l *Conv2D) Parameters() tensor.Vector {
	return tensor.CPUVector(l.parameters)
}

func (l *Conv2D) Gradients() tensor.Vector {
	return tensor.CPUVector(l.gradients)
}

func (l *Conv2D) saveForwardInput(input *tensor.SimpleTensor) {
	l.forwardInput = input.Vector()
}

func (l *Conv2D) SetKernel(input *tensor.SimpleTensor) { 
	if (input.Size()[2] != l.kernelSize[2] || input.Size()[3] != l.kernelSize[3]) {
		panic("kernel dimension not correct")
	}
	l.kernel = input.Vector()
}

func (l *Conv2D) ApplyKernel(m int, h int, w int) float64 { 
	inputHeight := l.inputSize[1] + l.padding[0] + l.padding[2]
	inputWidth := l.inputSize[2] + l.padding[1] + l.padding[3]
	inputChannelNum := l.inputSize[0]
	inputChannelSize := inputHeight * inputWidth

	kernelHeight := l.kernelSize[2]
	kernelWidth := l.kernelSize[3]

	sum := float64(0)
	for c := 0; c < inputChannelNum; c++ { // for each output channel
		for x := 0; x < kernelHeight; x++ {
			for y := 0; y < kernelWidth; y++ { //for each input element
				inputIndex := c * inputChannelSize + (h + y) * inputWidth + (w + x)
				kernelIndex := y * kernelWidth + x

				sum += l.inputWithPadding[inputIndex] * l.kernel[kernelIndex] 
			}
		}
	}
	return sum
}


func AddPadding(input []float64, inputSize, padding, stride []int) []float64{
	height := (inputSize[1]-1) * stride[0] + 1 + padding[0] + padding[2]   // (H-1) * S_h + 1 + P_t + P_b
	width := (inputSize[2]-1) * stride[1] + 1 + padding[1] + padding[3]   // (W-1) * S_w + 1 + P_r + P_l
	channelNum := inputSize[0]
	channelSize := height * width
	totalSize := channelNum * channelSize
	
	inputWithPadding := make([]float64, totalSize)

	index := 0
	inputIndex := 0
	for c := 0; c < channelNum; c++ { // for each channel
		for h := 0; h < height; h++ {
			if (h >= padding[0] && h < height - padding[2] && (h-padding[0]) % stride[0] != 0 ){
				for w := 0; w < width; w++ { //for each element
					inputWithPadding[index] = float64(0)
					index++
				}
			} else {
				for w := 0; w < width; w++ { //for each element
				
					if ( h < padding[0] || h >= height - padding[2] ||
						 w < padding[3] || w >=  width - padding[1] ||
						 (w - padding[3]) % stride[1] != 0 ) { //add padding element 0
						
						inputWithPadding[index] = float64(0)	

					} else {
						inputWithPadding[index] = input[inputIndex]
						inputIndex += 1
					}
					index += 1
				}
			}
		}
	}
	return inputWithPadding
}

func RemovePadding(input []float64, inputSize []int, padding []int) []float64{
	height := inputSize[1] - padding[0] - padding[2]
	width := inputSize[2] - padding[1] - padding[3]
	channelNum := inputSize[0]
	channelSize := height * width
	totalSize := channelNum * channelSize
	
	output := make([]float64, totalSize)

	index := 0
	inputIndex := 0
	for c := 0; c < channelNum; c++ { // for each channel
		for h := 0; h < inputSize[1]; h++ {
			for w := 0; w < inputSize[2]; w++ { //for each element
				
				if (h >=  padding[0]         &&
					h < height + padding[0]  &&
					w >=  padding[3]         && 
					w < width + padding[3]  ) { 
					
					output[index] = input[inputIndex]
					index += 1	
				} 
				inputIndex += 1
			}
		}
	}
	return output
}


func (l *Conv2D) calculateWeightGradients(input *tensor.SimpleTensor) {
	outputGradient := input.Vector()

	inputHeight := l.inputSize[1] + l.padding[0] + l.padding[1]
	inputWidth := l.inputSize[2] + l.padding[1] + l.padding[3]
	inputChannelNum := l.inputSize[0]
	inputChannelSize := inputHeight * inputWidth

	outputHeight := l.outputSize[1]
	outputWidth := l.outputSize[2]
	outputChannelNum := l.outputSize[0]
	outputChannelSize := outputHeight * outputWidth

	kernelHeight := l.kernelSize[2]
	kernelWidth := l.kernelSize[3]
	kernelChannelSize := kernelHeight * kernelWidth

	weightGradient := make([]float64, outputChannelNum * inputChannelNum * kernelChannelSize)

	for m := 0; m < outputChannelNum; m++ { // for each output channel
		for x := 0; x < kernelHeight; x++ {
			for y := 0; y < kernelWidth; y++ { 
				
				for c := 0; c < inputChannelNum; c++ { // for each input channel
					for h := 0; (h < outputHeight); h++ { 
						for w := 0; (w < outputWidth); w++ {

							inputIdx := c * inputChannelSize + (x * l.stride[0] + h) * inputWidth + (y * l.stride[1] + w)	// inputwithPadding[c, x+h, y+w]
							outputIdx := m * outputChannelSize + h * outputWidth + w  	// outputGradient[m, h, w]
							kernelIdx := m * (inputChannelNum * kernelChannelSize) + c * kernelChannelSize + x * kernelWidth + y	//weightGradient[m, c, x, y]
			
							weightGradient[kernelIdx] += l.inputWithPadding[inputIdx] * outputGradient[outputIdx] 
							
						}
					}
				}
			}
		}
	}
	l.weightGradient = weightGradient
}

func (l *Conv2D) calculateInputGradients( input *tensor.SimpleTensor) {

	inputHeight := l.inputSize[1] + l.padding[0] + l.padding[1]
	inputWidth := l.inputSize[2] + l.padding[1] + l.padding[3]
	inputChannelNum := l.inputSize[0]
	inputChannelSize := inputHeight * inputWidth

	kernelHeight := l.kernelSize[2]
	kernelWidth := l.kernelSize[3]
	kernelChannelSize := kernelHeight * kernelWidth

	outputHeight := (l.outputSize[1] - 1) * l.stride[0] + 1 + 2 * (kernelHeight - 1)
	outputWidth := (l.outputSize[2] - 1) * l.stride[1] + 1 + 2 * (kernelWidth - 1)
	outputChannelNum := l.outputSize[0]
	outputChannelSize := outputHeight * outputWidth

	inputGradient := make([]float64, inputChannelNum * inputChannelSize)
	outputGradient := AddPadding(input.Vector(), l.outputSize, []int{kernelHeight-1, kernelWidth-1, kernelHeight-1, kernelWidth-1}, l.stride)
	// fmt.Println(outputGradient)

	for m := 0; m < outputChannelNum; m++ { // for each output channel
		for h := 0; h < inputHeight; h++ {
			for w := 0; w < inputWidth; w++ { 
				
				for c := 0; c < inputChannelNum; c++ { // for each input channel
					for x := 0; x < kernelHeight; x++ { 
						for y := 0; y < kernelWidth; y++ {

							inputIdx := c * inputChannelSize + h * inputWidth + w	// inputGradient[c, h, w]
							outputIdx := m * outputChannelSize + (h + x) * outputWidth + (w + y)  	// outputGradient[m, h+x, w+y]
							kernelIdx := m * (inputChannelNum * kernelChannelSize) + c * kernelChannelSize + 
													(kernelHeight - x - 1) * kernelWidth + (kernelWidth - y - 1)	//weightGradient[m, c, (kH-x-1), (kW-y-1)]
			
							inputGradient[inputIdx] += outputGradient[outputIdx] * l.kernel[kernelIdx]
							
						}
					}
				}
			}
		}
	}
	l.inputGradient = RemovePadding(inputGradient, []int{inputChannelNum, inputHeight, inputWidth}, l.padding)
	// output.Init(l.inputGradient, l.inputSize)
}