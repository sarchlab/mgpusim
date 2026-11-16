package layers

import (
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
)

// A FullyConnectedLayer implements a fully connected layer.
type FullyConnectedLayer struct {
	*FullyConnectedLayerState
	to   tensor.Operator
	refs tensor.References
}

// NewFullyConnectedLayer creates a fully connected layer.
func NewFullyConnectedLayer(
	index int,
	to tensor.Operator,
	inputSize, outputSize int,
) *FullyConnectedLayer {
	refs := tensor.NewReferences(to)
	numWeight := inputSize * outputSize
	numBias := outputSize
	numParams := numWeight + numBias

	l := &FullyConnectedLayer{
		FullyConnectedLayerState: &FullyConnectedLayerState{
			LayerIndex: index,
			InputSize:  inputSize,
			OutputSize: outputSize,
			ParametersID: refs.Reference(to.Create([]int{
				numParams,
			})),
			GradientsID: refs.Reference(to.Create([]int{
				numParams,
			})),
		},
		to:   to,
		refs: refs,
	}

	l.WeightsID = l.refs.Replace(l.WeightsID, to.Slice(l.refs.Resolve(l.ParametersID), 0, numWeight))
	l.BiasID = l.refs.Replace(l.BiasID, to.Slice(l.refs.Resolve(l.ParametersID), numWeight, numParams))
	l.WeightGradientsID = l.refs.Replace(l.WeightGradientsID, to.Slice(l.refs.Resolve(l.GradientsID), 0, numWeight))
	l.BiasGradientsID = l.refs.Replace(l.BiasGradientsID, to.Slice(l.refs.Resolve(l.GradientsID), numWeight, numParams))

	return l
}

// Randomize initialize the parameters of the layer randomly.
func (l *FullyConnectedLayer) Randomize() {
	numWeight := l.InputSize * l.OutputSize
	weights := make([]float64, numWeight)
	for i := 0; i < numWeight; i++ {
		weights[i] = (randomFloat64(l.to) - 0.5) / float64(l.InputSize) * 2
	}
	l.to.Init(l.refs.Resolve(l.WeightsID), weights)

	numBias := l.OutputSize
	bias := make([]float64, numBias)
	for i := 0; i < numBias; i++ {
		bias[i] = randomFloat64(l.to)*2 - 1
	}
	l.to.Init(l.refs.Resolve(l.BiasID), bias)
}

// Forward performs the forward propagation operation.
func (l *FullyConnectedLayer) Forward(
	input tensor.Tensor,
) tensor.Tensor {
	l.ForwardInputID = l.refs.Replace(l.ForwardInputID, l.to.Clone(input))

	in := l.to.Reshape(input, []int{input.Size()[0], l.InputSize})
	weightMat := l.to.Reshape(l.refs.Resolve(l.WeightsID), []int{l.InputSize, l.OutputSize})
	biasMat := l.to.Repeat(l.refs.Resolve(l.BiasID), input.Size()[0])
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
	l.to.Clear(l.refs.Resolve(l.GradientsID))

	l.calculateWeightGradients(input)
	l.calculateBiasGradients(input)
	var output tensor.Tensor

	if l.LayerIndex > 0 {
		output = l.calculateInputGradients(input)
	}

	l.to.Free(l.refs.Resolve(l.ForwardInputID))

	return output
}

func (l *FullyConnectedLayer) calculateWeightGradients(
	input tensor.Tensor,
) {
	forwardInMatrix := l.to.Reshape(l.refs.Resolve(l.ForwardInputID),
		[]int{l.refs.Resolve(l.ForwardInputID).Size()[0], l.InputSize})
	backwardInMatrix := l.to.Reshape(input,
		[]int{input.Size()[0], l.OutputSize})
	zeroMatrix := l.to.Zeros([]int{l.InputSize, l.OutputSize})

	g := l.to.Gemm(
		true, false,
		1, 1,
		forwardInMatrix, backwardInMatrix,
		zeroMatrix,
	)

	l.to.Copy(l.refs.Resolve(l.WeightGradientsID), g)

	l.to.Free(forwardInMatrix)
	l.to.Free(backwardInMatrix)
	l.to.Free(zeroMatrix)
	l.to.Free(g)
}

func (l *FullyConnectedLayer) calculateBiasGradients(
	input tensor.Tensor,
) {
	g := l.to.Sum(input, []int{0})
	l.to.Copy(l.refs.Resolve(l.BiasGradientsID), g)
	l.to.Free(g)
}

func (l *FullyConnectedLayer) calculateInputGradients(
	input tensor.Tensor,
) tensor.Tensor {
	weightMatrix := l.to.Reshape(l.refs.Resolve(l.WeightsID), []int{l.InputSize, l.OutputSize})
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
	return l.refs.Resolve(l.ParametersID)
}

// Gradients returns the gradients of the layer.
func (l FullyConnectedLayer) Gradients() tensor.Tensor {
	return l.refs.Resolve(l.GradientsID)
}
