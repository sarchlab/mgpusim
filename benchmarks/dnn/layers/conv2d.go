package layers

import (
	"math/rand"

	"github.com/sarchlab/mgpusim/v4/benchmarks/dnn/tensor"
)

// Conv2D is a regular convolutional layer.
type Conv2D struct {
	layerIndex int
	to         tensor.Operator

	inputSize, outputSize []int
	kernelSize            []int
	stride                []int
	padding               []int

	parameters      tensor.Tensor
	weights         tensor.Tensor
	bias            tensor.Tensor
	gradients       tensor.Tensor
	weightGradients tensor.Tensor
	biasGradients   tensor.Tensor

	forwardInput tensor.Tensor
}

// NewConv2D creates a new Conv2D layer.
func NewConv2D(
	index int,
	to tensor.Operator,
	inputSize, kernelSize, stride, padding []int,
) *Conv2D {
	argumentsMustBeValid(inputSize, kernelSize, stride, padding)

	l := &Conv2D{
		layerIndex: index,
		to:         to,
		inputSize:  inputSize,
		kernelSize: kernelSize,
		stride:     stride,
		padding:    padding,
	}

	l.calculateOutputSize()
	l.allocateBuffers()

	return l
}

func (l *Conv2D) allocateBuffers() {
	l.parameters = l.to.Create([]int{l.numParam()})
	l.weights = l.to.Slice(l.parameters, 0, l.numWeight())
	l.bias = l.to.Slice(l.parameters, l.numWeight(), l.numWeight()+l.numBias())

	l.gradients = l.to.Create([]int{l.numParam()})
	l.weightGradients = l.to.Slice(l.gradients, 0, l.numWeight())
	l.biasGradients = l.to.Slice(l.gradients,
		l.numWeight(),
		l.numWeight()+l.numBias())
}

func (l *Conv2D) numParam() int {
	return l.numWeight() + l.numBias()
}

func (l *Conv2D) numWeight() int {
	return l.kernelSize[0] * l.kernelSize[1] * l.kernelSize[2] * l.kernelSize[3]
}

func (l *Conv2D) numBias() int {
	return l.kernelSize[0]
}

// Randomize will randomly initialize the layer parmeters.
func (l *Conv2D) Randomize() {
	// numWeightPerKernel := l.numWeight() / l.kernelSize[0]
	weights := make([]float64, l.numWeight())
	for i := 0; i < l.numWeight(); i++ {
		weights[i] = (rand.Float64() - 0.5) / float64(l.numWeight())
	}
	l.to.Init(l.weights, weights)

	numBias := l.numBias()
	bias := make([]float64, numBias)
	for i := 0; i < numBias; i++ {
		bias[i] = rand.Float64()*2 - 1
	}
	l.to.Init(l.bias, bias)
}

// Gradients returns all the gradients of the layer.
func (l *Conv2D) Gradients() tensor.Tensor {
	return l.gradients
}

// Parameters returns all the parameters of the layer.
func (l *Conv2D) Parameters() tensor.Tensor {
	return l.parameters
}

func (l *Conv2D) calculateOutputSize() {
	height := (l.inputSize[1]-l.kernelSize[2]+2*l.padding[0])/l.stride[0] + 1
	width := (l.inputSize[2]-l.kernelSize[3]+2*l.padding[1])/l.stride[1] + 1
	channel := l.kernelSize[0]
	l.outputSize = []int{channel, height, width}
}

func argumentsMustBeValid(inputSize, kernelSize, stride, padding []int) {
	inputOutputMustBe3D(inputSize)
	kernelMustBe4D(kernelSize)
	inputChannelMustMatchKernelChannel(inputSize, kernelSize)
	inputImageShouldNotBeSmallerThanKernel(inputSize, kernelSize)
	strideMustBe2D(stride)
	paddingMustBe2D(padding)
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

func paddingMustBe2D(padding []int) {
	if len(padding) != 2 {
		panic("stride must have 2 numbers (vertical padding, horizontal padding)")
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

// Forward calculates the forward propagation results of the layer.
func (l *Conv2D) Forward(input tensor.Tensor) tensor.Tensor {
	l.forwardInput = l.to.Clone(input)

	im2ColMatrix := l.to.Im2Col(input,
		[]int{l.kernelSize[2], l.kernelSize[3]},
		l.padding, l.stride, []int{1, 1})
	weightMatrix := l.to.Reshape(l.weights,
		[]int{l.kernelSize[0], im2ColMatrix.Size()[0]})

	biasMatrix := l.to.Repeat(l.bias, im2ColMatrix.Size()[1])
	biasMatrix.SetSize([]int{im2ColMatrix.Size()[1], l.kernelSize[0]})
	biasMatrixTranspose := l.to.Transpose(biasMatrix, []int{1, 0})
	// biasMatrixTranspose := l.to.Zeros(
	// []int{l.kernelSize[0], im2ColMatrix.Size()[1]})

	outputMatrix := l.to.Gemm(false, false, 1.0, 1.0,
		weightMatrix, im2ColMatrix, biasMatrixTranspose)

	outputMatrix.SetSize(
		[]int{
			l.kernelSize[0],
			input.Size()[0],
			l.outputSize[1],
			l.outputSize[2],
		})
	outputTranspose := l.to.Transpose(outputMatrix, []int{1, 0, 2, 3})
	outputTranspose.SetDescriptor("NCHW")

	l.to.Free(im2ColMatrix)
	l.to.Free(weightMatrix)
	l.to.Free(biasMatrix)
	l.to.Free(biasMatrixTranspose)
	l.to.Free(outputMatrix)

	return outputTranspose
}

// Backward calculates the gradients of the parameters and the gradient of
// the input.
func (l *Conv2D) Backward(input tensor.Tensor) tensor.Tensor {
	l.calculateWeightGradient(input)
	l.calculateBiasGradient(input)
	var output tensor.Tensor

	if l.layerIndex > 0 {
		output = l.calculateInputGradient(input)
	}

	l.to.Free(l.forwardInput)

	return output
}

func (l *Conv2D) calculateWeightGradient(dy tensor.Tensor) {
	numBatch := dy.Size()[0]
	dyChannel := dy.Size()[1]
	dyHeight := dy.Size()[2]
	dyWidth := dy.Size()[3]

	xTensor := l.forwardInput
	xTensorTrans := l.to.Transpose(xTensor, []int{1, 0, 2, 3})

	dyTrans := l.to.Transpose(dy, []int{1, 0, 2, 3})
	dyTrans.SetSize([]int{dyChannel, numBatch * dyHeight * dyWidth})

	xTensorCols := l.to.Im2Col(xTensorTrans,
		[]int{dyHeight, dyWidth},
		l.padding,
		[]int{1, 1},
		l.stride,
	)

	zeros := l.to.Zeros([]int{dyTrans.Size()[0], xTensorCols.Size()[1]})

	dW := l.to.Gemm(false, false, 1, 0,
		dyTrans, xTensorCols, zeros)
	dW.SetSize([]int{dW.NumElement()})

	l.to.Copy(l.weightGradients, dW)

	l.to.Free(dW)
	l.to.Free(xTensorTrans)
	l.to.Free(dyTrans)
	l.to.Free(xTensorCols)
	l.to.Free(zeros)
}

func (l *Conv2D) calculateBiasGradient(input tensor.Tensor) {
	sum := l.to.Sum(input, []int{0, 2, 3})
	l.to.Copy(l.biasGradients, sum)
	l.to.Free(sum)
}

func (l *Conv2D) calculateInputGradient(input tensor.Tensor) tensor.Tensor {
	inputDilate := l.to.Dilate(input, l.stride)

	im2ColMatrix := l.to.Im2Col(inputDilate,
		[]int{l.kernelSize[2], l.kernelSize[3]},
		[]int{
			l.kernelSize[2] - 1 - l.padding[0],
			l.kernelSize[3] - 1 - l.padding[1],
		},
		[]int{1, 1}, []int{1, 1},
	)

	l.weights.SetSize(l.kernelSize)
	kernelRot := l.to.Rotate180(l.weights)
	kernelMatrix := l.to.Transpose(kernelRot, []int{1, 0, 2, 3})
	kernelMatrix.SetSize(
		[]int{
			l.kernelSize[1],
			l.kernelSize[0] * l.kernelSize[2] * l.kernelSize[3],
		})

	zeros := l.to.Zeros([]int{l.kernelSize[1], im2ColMatrix.Size()[1]})

	outputMatrix := l.to.Gemm(
		false, false,
		1, 1,
		kernelMatrix, im2ColMatrix, zeros,
	)

	outputMatrix.SetSize([]int{
		l.kernelSize[1],
		input.Size()[0],
		l.inputSize[1],
		l.inputSize[2],
	})

	out := l.to.Transpose(outputMatrix, []int{1, 0, 2, 3})

	l.to.Free(inputDilate)
	l.to.Free(im2ColMatrix)
	l.to.Free(kernelRot)
	l.to.Free(kernelMatrix)
	l.to.Free(zeros)
	l.to.Free(outputMatrix)

	return out
}
