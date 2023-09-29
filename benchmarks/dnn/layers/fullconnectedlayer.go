package layers

import (
	"math/rand"

	"github.com/sarchlab/mgpusim/v3/benchmarks/dnn/tensor"
)

// A FullyConnectedLayer implements a fully connected layer.
type FullyConnectedLayer struct {
	layerIndex int
	to         tensor.Operator

	InputSize  int
	OutputSize int

	parameters      tensor.Tensor
	weights         tensor.Tensor
	bias            tensor.Tensor
	gradients       tensor.Tensor
	weightGradients tensor.Tensor
	biasGradients   tensor.Tensor
	forwardInput    tensor.Tensor
}

// NewFullyConnectedLayer creates a fully connected layer.
func NewFullyConnectedLayer(
	index int,
	to tensor.Operator,
	inputSize, outputSize int,
) *FullyConnectedLayer {
	numWeight := inputSize * outputSize
	numBias := outputSize
	numParams := numWeight + numBias

	l := &FullyConnectedLayer{
		layerIndex: index,
		to:         to,
		InputSize:  inputSize,
		OutputSize: outputSize,
		parameters: to.Create([]int{numParams}),
		gradients:  to.Create([]int{numParams}),
	}

	l.weights = to.Slice(l.parameters, 0, numWeight)
	l.bias = to.Slice(l.parameters, numWeight, numParams)
	l.weightGradients = to.Slice(l.gradients, 0, numWeight)
	l.biasGradients = to.Slice(l.gradients, numWeight, numParams)

	return l
}

// Randomize initialize the parameters of the layer randomly.
func (l *FullyConnectedLayer) Randomize() {
	numWeight := l.InputSize * l.OutputSize
	weights := make([]float64, numWeight)
	for i := 0; i < numWeight; i++ {
		weights[i] = (rand.Float64() - 0.5) / float64(l.InputSize) * 2
	}
	l.to.Init(l.weights, weights)

	numBias := l.OutputSize
	bias := make([]float64, numBias)
	for i := 0; i < numBias; i++ {
		bias[i] = rand.Float64()*2 - 1
	}
	l.to.Init(l.bias, bias)
}

// Forward performs the forward propagation operation.
func (l *FullyConnectedLayer) Forward(
	input tensor.Tensor,
) tensor.Tensor {
	l.forwardInput = l.to.Clone(input)

	in := l.to.Reshape(input, []int{input.Size()[0], l.InputSize})
	weightMat := l.to.Reshape(l.weights, []int{l.InputSize, l.OutputSize})
	biasMat := l.to.Repeat(l.bias, input.Size()[0])
	biasMatReshape := l.to.Reshape(biasMat,
		[]int{input.Size()[0], l.OutputSize})

	out := l.to.Gemm(false, false, 1, 1, in, weightMat, biasMatReshape)

	l.to.Free(in)
	l.to.Free(weightMat)
	l.to.Free(biasMat)
	l.to.Free(biasMatReshape)

	return out
}

// Backward calculate the weight, bias, and input gradients.
func (l *FullyConnectedLayer) Backward(
	input tensor.Tensor,
) tensor.Tensor {
	l.to.Clear(l.gradients)

	l.calculateWeightGradients(input)
	l.calculateBiasGradients(input)
	var output tensor.Tensor

	if l.layerIndex > 0 {
		output = l.calculateInputGradients(input)
	}

	l.to.Free(l.forwardInput)

	return output
}

func (l *FullyConnectedLayer) calculateWeightGradients(
	input tensor.Tensor,
) {
	forwardInMatrix := l.to.Reshape(l.forwardInput,
		[]int{l.forwardInput.Size()[0], l.InputSize})
	backwardInMatrix := l.to.Reshape(input,
		[]int{input.Size()[0], l.OutputSize})
	zeroMatrix := l.to.Zeros([]int{l.InputSize, l.OutputSize})

	g := l.to.Gemm(
		true, false,
		1, 1,
		forwardInMatrix, backwardInMatrix,
		zeroMatrix,
	)

	l.to.Copy(l.weightGradients, g)

	l.to.Free(forwardInMatrix)
	l.to.Free(backwardInMatrix)
	l.to.Free(zeroMatrix)
	l.to.Free(g)
}

func (l *FullyConnectedLayer) calculateBiasGradients(
	input tensor.Tensor,
) {
	g := l.to.Sum(input, []int{0})
	l.to.Copy(l.biasGradients, g)
	l.to.Free(g)
}

func (l *FullyConnectedLayer) calculateInputGradients(
	input tensor.Tensor,
) tensor.Tensor {
	weightMatrix := l.to.Reshape(l.weights, []int{l.InputSize, l.OutputSize})
	inputMatrix := l.to.Reshape(input, []int{input.Size()[0], l.OutputSize})
	zeroMatrix := l.to.Zeros([]int{input.Size()[0], l.InputSize})

	out := l.to.Gemm(false, true, 1, 1, inputMatrix, weightMatrix, zeroMatrix)

	l.to.Free(weightMatrix)
	l.to.Free(inputMatrix)
	l.to.Free(zeroMatrix)

	return out
}

// Parameters returns the parameters of the layer.
func (l FullyConnectedLayer) Parameters() tensor.Tensor {
	return l.parameters
}

// Gradients returns the gradients of the layer.
func (l FullyConnectedLayer) Gradients() tensor.Tensor {
	return l.gradients
}
