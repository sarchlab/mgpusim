package layers

import (
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
)

// Conv2D is a regular convolutional layer.
type Conv2D struct {
	*Conv2DState
	to   tensor.Operator
	refs tensor.References
}

// NewConv2D creates a new Conv2D layer.
func NewConv2D(
	index int,
	to tensor.Operator,
	inputSize, kernelSize, stride, padding []int,
) *Conv2D {
	refs := tensor.NewReferences(to)
	argumentsMustBeValid(inputSize, kernelSize, stride, padding)

	l := &Conv2D{
		Conv2DState: &Conv2DState{
			LayerIndex: index,
			InputSize:  inputSize,
			KernelSize: kernelSize,
			Stride:     stride,
			Padding:    padding,
		},
		to:   to,
		refs: refs,
	}

	l.calculateOutputSize()
	l.allocateBuffers()

	return l
}

func (l *Conv2D) allocateBuffers() {
	l.ParametersID = l.refs.Replace(l.ParametersID, l.to.Create([]int{l.numParam()}))
	l.WeightsID = l.refs.Replace(l.WeightsID, l.to.Slice(l.refs.Resolve(l.ParametersID), 0, l.numWeight()))
	l.BiasID = l.refs.Replace(l.BiasID,
		l.to.Slice(l.refs.Resolve(l.ParametersID), l.numWeight(), l.numWeight()+l.numBias()))

	l.GradientsID = l.refs.Replace(l.GradientsID, l.to.Create([]int{l.numParam()}))
	l.WeightGradientsID = l.refs.Replace(l.WeightGradientsID, l.to.Slice(l.refs.Resolve(l.GradientsID), 0, l.numWeight()))
	l.BiasGradientsID = l.refs.Replace(l.BiasGradientsID, l.to.Slice(l.refs.Resolve(l.GradientsID),
		l.numWeight(),
		l.numWeight()+l.numBias()))
}

func (l *Conv2D) numParam() int {
	return l.numWeight() + l.numBias()
}

func (l *Conv2D) numWeight() int {
	return l.KernelSize[0] * l.KernelSize[1] * l.KernelSize[2] * l.KernelSize[3]
}

func (l *Conv2D) numBias() int {
	return l.KernelSize[0]
}

// Randomize will randomly initialize the layer parmeters.
func (l *Conv2D) Randomize() {
	// numWeightPerKernel := l.numWeight() / l.kernelSize[0]
	weights := make([]float64, l.numWeight())
	for i := 0; i < l.numWeight(); i++ {
		weights[i] = (randomFloat64(l.to) - 0.5) / float64(l.numWeight())
	}
	l.to.Init(l.refs.Resolve(l.WeightsID), weights)

	numBias := l.numBias()
	bias := make([]float64, numBias)
	for i := 0; i < numBias; i++ {
		bias[i] = randomFloat64(l.to)*2 - 1
	}
	l.to.Init(l.refs.Resolve(l.BiasID), bias)
}

// Gradients returns all the gradients of the layer.
func (l *Conv2D) Gradients() tensor.Tensor {
	return l.refs.Resolve(l.GradientsID)
}

// Parameters returns all the parameters of the layer.
func (l *Conv2D) Parameters() tensor.Tensor {
	return l.refs.Resolve(l.ParametersID)
}

func (l *Conv2D) calculateOutputSize() {
	height := (l.InputSize[1]-l.KernelSize[2]+2*l.Padding[0])/l.Stride[0] + 1
	width := (l.InputSize[2]-l.KernelSize[3]+2*l.Padding[1])/l.Stride[1] + 1
	channel := l.KernelSize[0]
	l.OutputSize = []int{channel, height, width}
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
	l.ForwardInputID = l.refs.Replace(l.ForwardInputID, l.to.Clone(input))

	im2ColMatrix := l.to.Im2Col(input,
		[]int{l.KernelSize[2], l.KernelSize[3]},
		l.Padding, l.Stride, []int{1, 1})
	weightMatrix := l.to.Reshape(l.refs.Resolve(l.WeightsID),
		[]int{l.KernelSize[0], im2ColMatrix.Size()[0]})

	biasMatrix := l.to.Repeat(l.refs.Resolve(l.BiasID), im2ColMatrix.Size()[1])
	biasMatrix.SetSize([]int{im2ColMatrix.Size()[1], l.KernelSize[0]})
	biasMatrixTranspose := l.to.Transpose(biasMatrix, []int{1, 0})
	// biasMatrixTranspose := l.to.Zeros(
	// []int{l.kernelSize[0], im2ColMatrix.Size()[1]})

	outputMatrix := l.to.Gemm(false, false, 1.0, 1.0,
		weightMatrix, im2ColMatrix, biasMatrixTranspose)

	outputMatrix.SetSize(
		[]int{
			l.KernelSize[0],
			input.Size()[0],
			l.OutputSize[1],
			l.OutputSize[2],
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

	if l.LayerIndex > 0 {
		output = l.calculateInputGradient(input)
	}

	l.to.Free(l.refs.Resolve(l.ForwardInputID))

	return output
}

func (l *Conv2D) calculateWeightGradient(dy tensor.Tensor) {
	numBatch := dy.Size()[0]
	dyChannel := dy.Size()[1]
	dyHeight := dy.Size()[2]
	dyWidth := dy.Size()[3]

	xTensor := l.refs.Resolve(l.ForwardInputID)
	xTensorTrans := l.to.Transpose(xTensor, []int{1, 0, 2, 3})

	dyTrans := l.to.Transpose(dy, []int{1, 0, 2, 3})
	dyTrans.SetSize([]int{dyChannel, numBatch * dyHeight * dyWidth})

	xTensorCols := l.to.Im2Col(xTensorTrans,
		[]int{dyHeight, dyWidth},
		l.Padding,
		[]int{1, 1},
		l.Stride,
	)

	zeros := l.to.Zeros([]int{dyTrans.Size()[0], xTensorCols.Size()[1]})

	dW := l.to.Gemm(false, false, 1, 0,
		dyTrans, xTensorCols, zeros)
	dW.SetSize([]int{dW.NumElement()})

	l.to.Copy(l.refs.Resolve(l.WeightGradientsID), dW)

	l.to.Free(dW)
	l.to.Free(xTensorTrans)
	l.to.Free(dyTrans)
	l.to.Free(xTensorCols)
	l.to.Free(zeros)
}

func (l *Conv2D) calculateBiasGradient(input tensor.Tensor) {
	sum := l.to.Sum(input, []int{0, 2, 3})
	l.to.Copy(l.refs.Resolve(l.BiasGradientsID), sum)
	l.to.Free(sum)
}

func (l *Conv2D) calculateInputGradient(input tensor.Tensor) tensor.Tensor {
	inputDilate := l.to.Dilate(input, l.Stride)

	im2ColMatrix := l.to.Im2Col(inputDilate,
		[]int{l.KernelSize[2], l.KernelSize[3]},
		[]int{
			l.KernelSize[2] - 1 - l.Padding[0],
			l.KernelSize[3] - 1 - l.Padding[1],
		},
		[]int{1, 1}, []int{1, 1},
	)

	l.refs.Resolve(l.WeightsID).SetSize(l.KernelSize)
	kernelRot := l.to.Rotate180(l.refs.Resolve(l.WeightsID))
	kernelMatrix := l.to.Transpose(kernelRot, []int{1, 0, 2, 3})
	kernelMatrix.SetSize(
		[]int{
			l.KernelSize[1],
			l.KernelSize[0] * l.KernelSize[2] * l.KernelSize[3],
		})

	zeros := l.to.Zeros([]int{l.KernelSize[1], im2ColMatrix.Size()[1]})

	outputMatrix := l.to.Gemm(
		false, false,
		1, 1,
		kernelMatrix, im2ColMatrix, zeros,
	)

	outputMatrix.SetSize([]int{
		l.KernelSize[1],
		input.Size()[0],
		l.InputSize[1],
		l.InputSize[2],
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
