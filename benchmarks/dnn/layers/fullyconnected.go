package layers

import (
	"log"
	"math"
	"math/rand"

	"gitlab.com/akita/dnn/layers"
	"gitlab.com/akita/dnn/tensor"
	"gitlab.com/akita/mgpusim/driver"
)

// FullyConnectedLayer represents a fully-connected layer.
type FullyConnectedLayer struct {
	InputSize, OutputSize int

	GPUDriver      *driver.Driver
	GPUCtx         *driver.Context
	MatrixOperator *MatrixOperator

	verifyForward  bool
	verifyBackward bool
	cpuLayer       *layers.FullyConnectedLayer

	parameters      *Vector
	weight          *Vector
	bias            *Vector
	gradients       *Vector
	weightGradients *Vector
	biasGradients   *Vector

	forwardInput driver.GPUPtr
}

// NewFullyConnectedLayer creates a new fully connected layer.
func NewFullyConnectedLayer(
	inputSize, outputSize int,
	driver *driver.Driver,
	ctx *driver.Context,
	operator *MatrixOperator,
) *FullyConnectedLayer {
	return &FullyConnectedLayer{
		InputSize:      inputSize,
		OutputSize:     outputSize,
		GPUDriver:      driver,
		GPUCtx:         ctx,
		MatrixOperator: operator,
	}
}

// EnableVerification runs a CPU pass for every forward and backward propagation
// though the fully connected layer to make sure the simulator is correct.
func (f *FullyConnectedLayer) EnableVerification() {
	f.verifyForward = true
	f.verifyBackward = true
	f.cpuLayer = layers.NewFullyConnectedLayer(f.InputSize, f.OutputSize)
}

// Randomize initialized the parameters randomly.
func (f *FullyConnectedLayer) Randomize() {
	f.allocateMemory()
	f.initWeights()
	f.initBias()
}

func (f *FullyConnectedLayer) initBias() {
	initBias := make([]float32, f.numBias())
	for i := 0; i < f.numBias(); i++ {
		initBias[i] = rand.Float32()*2 - 1
	}
	f.GPUDriver.MemCopyH2D(f.GPUCtx, f.bias.ptr, initBias)
}

func (f *FullyConnectedLayer) initWeights() {
	initWeights := make([]float32, f.numWeights())
	for i := 0; i < f.numWeights(); i++ {
		initWeights[i] = (rand.Float32() - 0.5) / float32(f.OutputSize) * 2
	}
	f.GPUDriver.MemCopyH2D(f.GPUCtx, f.weight.ptr, initWeights)
}

func (f *FullyConnectedLayer) allocateMemory() {
	f.allocateParams()
	f.allocateGradients()
}

func (f *FullyConnectedLayer) allocateGradients() {
	sizeOfFloat := 4

	gradientsPtr := f.GPUDriver.AllocateMemory(
		f.GPUCtx, uint64(f.numParameters()*sizeOfFloat))
	f.gradients = &Vector{
		size:      f.numParameters(),
		ptr:       gradientsPtr,
		GPUDriver: f.GPUDriver,
		GPUCtx:    f.GPUCtx,
	}

	f.weightGradients = &Vector{
		size:      f.numWeights(),
		ptr:       gradientsPtr,
		GPUDriver: f.GPUDriver,
		GPUCtx:    f.GPUCtx,
	}
	f.biasGradients = &Vector{
		size:      f.numBias(),
		ptr:       gradientsPtr + driver.GPUPtr(f.numWeights()*4),
		GPUDriver: f.GPUDriver,
		GPUCtx:    f.GPUCtx,
	}
}

func (f *FullyConnectedLayer) allocateParams() {
	sizeOfFloat := 4

	parametersPtr := f.GPUDriver.AllocateMemory(
		f.GPUCtx, uint64(f.numParameters()*sizeOfFloat))
	f.parameters = &Vector{
		size:      f.numParameters(),
		ptr:       parametersPtr,
		GPUDriver: f.GPUDriver,
		GPUCtx:    f.GPUCtx,
	}

	f.weight = &Vector{
		size:      f.numWeights(),
		ptr:       parametersPtr,
		GPUDriver: f.GPUDriver,
		GPUCtx:    f.GPUCtx,
	}
	f.bias = &Vector{
		size:      f.numBias(),
		ptr:       parametersPtr + driver.GPUPtr(f.numWeights()*4),
		GPUDriver: f.GPUDriver,
		GPUCtx:    f.GPUCtx,
	}
}

func (f FullyConnectedLayer) numParameters() int {
	numParameters := f.numWeights() + f.numBias()
	return numParameters
}

func (f FullyConnectedLayer) numBias() int {
	numBias := f.OutputSize
	return numBias
}

func (f FullyConnectedLayer) numWeights() int {
	numWeights := f.InputSize * f.OutputSize
	return numWeights
}

// Forward performs the forward propagation algorithm.
func (f *FullyConnectedLayer) Forward(inputT tensor.Tensor) tensor.Tensor {
	input := inputT.(*Tensor)
	output := &Tensor{
		driver: f.GPUDriver,
		ctx:    f.GPUCtx,
		size:   []int{input.Size()[0], f.OutputSize},
		ptr:    f.GPUDriver.AllocateMemory(f.GPUCtx, uint64(input.Size()[0]*f.OutputSize*4)),
	}

	f.saveInput(input)

	inputM := input.Matrix()
	outputM := output.Matrix()
	weightM := f.weight.AsMatrix(f.InputSize, f.OutputSize)
	biasM := f.MatrixOperator.CreateMatrix(inputT.Size()[0], f.OutputSize)
	biasData := make([]float32, f.OutputSize)
	f.GPUDriver.MemCopyD2H(f.GPUCtx, biasData, f.bias.ptr)

	for i := 0; i < inputT.Size()[0]; i++ {
		ptr := driver.GPUPtr(uint64(biasM.data) + uint64(i*f.OutputSize*4))
		f.GPUDriver.MemCopyH2D(f.GPUCtx, ptr, biasData)
	}

	f.MatrixOperator.Gemm(false, false,
		inputT.Size()[0], f.OutputSize, f.InputSize,
		1.0, 1.0,
		inputM, weightM, biasM, outputM)

	f.MatrixOperator.Free(biasM)

	f.verifyForwardPass(input, output)

	return output
}

func (f *FullyConnectedLayer) verifyForwardPass(input, output *Tensor) {
	if !f.verifyForward {
		return
	}

	params := f.Parameters().Raw()
	copy(f.cpuLayer.Parameters().Raw(), params)

	inputV := input.Vector()
	cpuInput := &tensor.SimpleTensor{}
	cpuInput.Init(inputV, input.Size())
	cpuOut := f.cpuLayer.Forward(cpuInput).Vector()
	gpuOut := output.Vector()

	for i := 0; i < len(cpuOut); i++ {
		diff := math.Abs(gpuOut[i] - cpuOut[i])
		if diff > 1e-5 {
			log.Panicf("Mismatch at %d, expected %f, but get %f.",
				i, cpuOut[i], gpuOut[i])
		}
	}

	log.Printf("Fully connected forward verification passed!")
}

func (f *FullyConnectedLayer) saveInput(input *Tensor) {
	if f.forwardInput != 0 {
		f.GPUDriver.FreeMemory(f.GPUCtx, f.forwardInput)
	}

	numElement := input.Size()[0] * input.Size()[1]

	f.forwardInput = f.GPUDriver.AllocateMemory(f.GPUCtx,
		uint64(numElement*4))

	temp := make([]float32, numElement)
	f.GPUDriver.MemCopyD2H(f.GPUCtx, temp, input.ptr)
	f.GPUDriver.MemCopyH2D(f.GPUCtx, f.forwardInput, temp)
}

// Backward performs the backward propagation operation.
func (f *FullyConnectedLayer) Backward(input tensor.Tensor) tensor.Tensor {
	f.resetGradients()
	f.calculateWeightGradients(input.(*Tensor))
	f.calculateBiasGradients(input.(*Tensor))
	output := f.calculateInputGradients(input.(*Tensor))

	f.verifyBackPass(input.(*Tensor), output)

	return output
}

func (f *FullyConnectedLayer) resetGradients() {
	data := make([]float32, f.numParameters())
	f.GPUDriver.MemCopyH2D(f.GPUCtx, f.gradients.ptr, data)
}

func (f *FullyConnectedLayer) verifyBackPass(input, output *Tensor) {
	if !f.verifyBackward {
		return
	}

	params := f.Parameters().Raw()
	copy(f.cpuLayer.Parameters().Raw(), params)

	inputV := input.Vector()
	cpuInput := &tensor.SimpleTensor{}
	cpuInput.Init(inputV, input.Size())
	cpuOut := f.cpuLayer.Backward(cpuInput).Vector()
	gpuOut := output.Vector()

	for i := 0; i < len(cpuOut); i++ {
		diff := math.Abs(gpuOut[i] - cpuOut[i])
		if diff > 1e-5 {
			log.Panicf("Mismatch at %d, expected %f, but get %f.",
				i, cpuOut[i], gpuOut[i])
		}
	}

	cpuGradient := f.cpuLayer.Gradients().Raw()
	gpuGradient := f.Gradients().Raw()

	for i := 0; i < len(cpuGradient); i++ {
		diff := math.Abs(gpuGradient[i] - cpuGradient[i])
		if diff > 1e-3 {
			log.Panicf("Mismatch at %d, expected %f, but get %f.",
				i, cpuGradient[i], gpuGradient[i])
		}
	}

	log.Printf("Fully connected backward verification passed!")
}

func (f *FullyConnectedLayer) calculateBiasGradients(input tensor.Tensor) {
	inputV := input.Vector()
	biasV := f.biasGradients.Raw()

	for i := 0; i < input.Size()[0]; i++ {
		for j := 0; j < input.Size()[1]; j++ {
			index := i*input.Size()[1] + j
			biasV[j] += inputV[index]
		}
	}

	tempData := make([]float32, f.OutputSize)
	for i, value := range biasV {
		tempData[i] = float32(value)
	}

	f.GPUDriver.MemCopyH2D(f.GPUCtx, f.biasGradients.ptr, tempData)
}

func (f *FullyConnectedLayer) calculateWeightGradients(input *Tensor) {
	size := input.Size()
	forwardMatrix := &Matrix{
		row:  size[0],
		col:  f.InputSize,
		data: f.forwardInput,
	}
	forwardMatrixTrans := f.MatrixOperator.CreateMatrix(
		f.InputSize, size[0])
	f.MatrixOperator.Transpose(forwardMatrix, forwardMatrixTrans)

	zeroMatrix := NewTensor(f.GPUDriver, f.GPUCtx)
	zeroMatrix.Init(
		make([]float64, f.numWeights()),
		[]int{f.InputSize, f.OutputSize},
	)

	f.MatrixOperator.Gemm(false, false,
		f.InputSize, f.OutputSize, size[0],
		1.0, 1.0,
		forwardMatrixTrans, input.Matrix(), zeroMatrix.Matrix(),
		f.weightGradients.AsMatrix(f.InputSize, f.OutputSize),
	)
}

func (f FullyConnectedLayer) calculateInputGradients(input *Tensor) *Tensor {
	size := input.Size()
	output := NewTensor(f.GPUDriver, f.GPUCtx)
	output.Init(
		make([]float64, size[0]*f.InputSize),
		[]int{size[0], f.InputSize})

	weightMatrix := f.weight.AsMatrix(f.InputSize, f.OutputSize)
	weightMatrixTrans := f.MatrixOperator.CreateMatrix(
		f.OutputSize, f.InputSize)
	f.MatrixOperator.Transpose(weightMatrix, weightMatrixTrans)

	zeroMatrix := NewTensor(f.GPUDriver, f.GPUCtx)
	zeroMatrix.Init(
		make([]float64, size[0]*f.InputSize),
		[]int{size[0], f.InputSize},
	)

	f.MatrixOperator.Gemm(false, false,
		size[0], f.InputSize, f.OutputSize,
		1.0, 1.0,
		input.Matrix(), weightMatrixTrans, zeroMatrix.Matrix(),
		output.Matrix())

	f.MatrixOperator.Free(weightMatrixTrans)
	return output
}

// Parameters returns the parameters of the layer.
func (f FullyConnectedLayer) Parameters() tensor.Vector {
	return f.parameters
}

// Gradients returns the gradients calculated by the last backward propagation.
func (f FullyConnectedLayer) Gradients() tensor.Vector {
	return f.gradients
}
