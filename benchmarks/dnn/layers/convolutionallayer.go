package layers

import (

	// "math"
	"fmt"
	"log"

	// "gitlab.com/akita/dnn/layers"
	"gitlab.com/akita/dnn/tensor"
	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/kernels"
	// "fmt"
)

// NewConvolutionalLayer creates a new convolutional layer for the DNN network.
// The inputSize should be a 3-number array representing [channel, height,
// width]. The kernel size should be a 4-number array, representing [output
// channel, input channel, height, width]. Stride is a 2-number array
// representing [vertical stride, horizontal stride]. Padding is a 4-number
// array, representing [top padding, right padding, bottom padding, left
// padding].

// A Conv2D can perform convolution operation on input data.
type Conv2D struct {
	GPUDriver      *driver.Driver
	GPUCtx         *driver.Context
	TensorOperator *TensorOperator

	forwardInput *Tensor

	verifyForward  bool
	verifyBackward bool
	// cpuLayer       *layers.Conv2D

	inputSize, outputSize []int
	kernelSize            []int
	stride                []int
	padding               []int
	// bias                  []int

	im2colNCHWKernel *insts.HsaCo
	im2colCNHWKernel *insts.HsaCo
	col2imKernel     *insts.HsaCo
	flatKernel       *insts.HsaCo

	parameters      *Vector
	kernel          *Vector
	bias            *Vector
	gradients       *Vector
	inputGradients  *Vector
	weightGradients *Vector
	biasGradients   *Vector
}

// KernelArgsCol2Im represents the kernel arguments for the Col2Im kernel.
type KernelArgsCol2Im struct {
	InputHeight               int32
	InputWidth                int32
	Channels                  int32
	OutputHeight              int32
	OutputWidth               int32
	KernelHeight              int32
	KernelWidth               int32
	PadHeight                 int32
	PadWidth                  int32
	StrideHeight              int32
	StrideWidth               int32
	DilationHeight            int32
	DilationWidth             int32
	ColBuffer                 driver.GPUPtr
	ColOffset                 int32
	ImBuffer                  driver.GPUPtr
	ImOffset                  int32
	OffsetX, OffsetY, OffsetZ uint64
}

// KernelArgsIm2Col represents the kernel arguments for the Im2Col kernel.
type KernelArgsIm2Col struct {
	Input                     driver.GPUPtr
	Output                    driver.GPUPtr
	InputDimensions           [2]uint32
	MaskDimensions            [2]uint32
	StrDimensions             [2]uint32
	PadVertDimensions         [2]uint32
	PadHoriDimensions         [2]uint32
	Dilation                  [2]uint32
	Channel                   uint32
	Batch                     uint32
	OffsetX, OffsetY, OffsetZ uint64
}

// KernelArgsFlatten represents the kernel arguments for the Flatten kernel.
type KernelArgsFlatten struct {
	Input                     driver.GPUPtr
	Output                    driver.GPUPtr
	InputChannel              uint32
	OutputChannel             uint32
	Height                    uint32
	Width                     uint32
	OffsetX, OffsetY, OffsetZ uint64
}

// NewConvolutionalLayer creates a new convolutional layer with given settings.
func NewConvolutionalLayer(
	inputSize, kernelSize, stride, padding []int,
	GPUDriver *driver.Driver,
	GPUCtx *driver.Context,
	TensorOperator *TensorOperator,
) *Conv2D {
	// argumentsMustBeValid(inputSize, kernelSize, stride, padding)

	l := &Conv2D{
		inputSize:      inputSize,
		kernelSize:     kernelSize,
		stride:         stride,
		padding:        padding,
		GPUDriver:      GPUDriver,
		GPUCtx:         GPUCtx,
		TensorOperator: TensorOperator,
	}
	l.calculateOutputSize()
	l.loadKernels()
	l.allocateMemory()

	return l
}

func (l *Conv2D) loadKernels() {
	im2colHsaCoBytes := _escFSMustByte(true, "/im2col.hsaco")
	l.im2colNCHWKernel = kernels.LoadProgramFromMemory(
		im2colHsaCoBytes, "im2colKernelNCHW")
	if l.im2colNCHWKernel == nil {
		log.Panic("Failed to load im2col NCHW kernel binary")
	}
	l.im2colCNHWKernel = kernels.LoadProgramFromMemory(
		im2colHsaCoBytes, "im2colKernelCNHW")
	if l.im2colCNHWKernel == nil {
		log.Panic("Failed to load im2col CNHW kernel binary")
	}

	col2imHsaCoBytes := _escFSMustByte(true, "/col2im.hsaco")
	l.col2imKernel = kernels.LoadProgramFromMemory(
		col2imHsaCoBytes, "col2imKernel")
	if l.col2imKernel == nil {
		log.Panic("Failed to load col2im kernel binary")
	}

	flattenHsaCoBytes := _escFSMustByte(true, "/flatten.hsaco")
	l.flatKernel = kernels.LoadProgramFromMemory(
		flattenHsaCoBytes, "flattenKernel")
	if l.flatKernel == nil {
		log.Panic("Failed to load flatten kernel binary")
	}
}

func (l *Conv2D) allocateMemory() {
	l.allocateParams()
	l.allocateGradients()
}

func (l *Conv2D) allocateGradients() {
	sizeOfFloat := 4
	numGradients := l.numWeights() + l.numInput() + l.numBias()
	gradientsPtr := l.GPUDriver.AllocateMemory(
		l.GPUCtx, uint64(numGradients*sizeOfFloat))

	l.inputGradients = &Vector{
		size:      l.numInput(),
		ptr:       gradientsPtr,
		GPUDriver: l.GPUDriver,
		GPUCtx:    l.GPUCtx,
	}
	l.weightGradients = &Vector{
		size:      l.numWeights(),
		ptr:       gradientsPtr + driver.GPUPtr(l.numInput()*sizeOfFloat),
		GPUDriver: l.GPUDriver,
		GPUCtx:    l.GPUCtx,
	}
	l.biasGradients = &Vector{
		size:      l.numBias(),
		ptr:       gradientsPtr + driver.GPUPtr((l.numInput()+l.numWeights())*sizeOfFloat),
		GPUDriver: l.GPUDriver,
		GPUCtx:    l.GPUCtx,
	}
}

func (l *Conv2D) allocateParams() {
	sizeOfFloat := 4
	parametersPtr := l.GPUDriver.AllocateMemory(
		l.GPUCtx, uint64(l.numParameters()*sizeOfFloat))

	l.parameters = &Vector{
		size:      l.numParameters(),
		ptr:       parametersPtr,
		GPUDriver: l.GPUDriver,
		GPUCtx:    l.GPUCtx,
	}
	l.kernel = &Vector{
		size:      l.numWeights(),
		ptr:       parametersPtr,
		GPUDriver: l.GPUDriver,
		GPUCtx:    l.GPUCtx,
	}
	l.bias = &Vector{
		size:      l.numBias(),
		ptr:       parametersPtr + driver.GPUPtr(l.numWeights()*sizeOfFloat),
		GPUDriver: l.GPUDriver,
		GPUCtx:    l.GPUCtx,
	}
}

// EnableVerification asks the layer to validate against CPU calculation after
// every forward and backward propagation calculation.
func (l *Conv2D) EnableVerification() {
	l.verifyForward = true
	l.verifyBackward = true
	// l.cpuLayer = layers.NewConvolutionalLayer(l.inputSize, l.kernelSize, l.stride, l.padding)
}

func (l *Conv2D) saveInput(input *Tensor) {
	l.forwardInput = input
}

func (l Conv2D) numParameters() int {
	numParameters := l.numWeights() + l.numBias()
	return numParameters
}

func (l Conv2D) numBias() int {
	numBias := l.outputSize[0]
	return numBias
}

func (l Conv2D) numWeights() int {
	numWeights := l.kernelSize[0] * l.kernelSize[1] * l.kernelSize[2] * l.kernelSize[3] // number of elements in kernel.
	return numWeights
}

func (l Conv2D) numInput() int {
	numInput := l.inputSize[0] * l.inputSize[1] * l.inputSize[2] // number of elements in input.
	return numInput
}

func (l Conv2D) numKernels() int {
	return l.kernelSize[0]
}

func (l Conv2D) numChannels() int {
	return l.kernelSize[1]
}

func (l Conv2D) kernelWidth() int {
	return l.kernelSize[3]
}

func (l Conv2D) kernelHeight() int {
	return l.kernelSize[2]
}

func (l *Conv2D) verifyForwardPass(input, output *Tensor) {

	// if !l.verifyForward {
	// 	return
	// }
	// params := l.parameters().Raw()
	// copy(l.cpuLayer.parameters().Raw(), params)

	// inputV := input.Vector()
	// cpuInput := &tensor.SimpleTensor{}
	// cpuInput.Init(inputV, input.Size())
	// cpuOut := l.cpuLayer.Forward(cpuInput).Vector()
	// gpuOut := output.Vector()

	// for i := 0; i < len(cpuOut); i++ {
	// 	diff := math.Abs(gpuOut[i] - cpuOut[i])
	// 	if diff > 1e-5 {
	// 		log.Panicf("Mismatch at %d, expected %f, but get %l.",
	// 			i, cpuOut[i], gpuOut[i])
	// 	}
	// }

	// log.Printf("Conv2D forward verification passed!")
}

func (l *Conv2D) verifyBackPass(input, output *Tensor) {

	// if !l.verifyBackward {
	// 	return
	// }
	// params := l.parameters().Raw()
	// copy(l.cpuLayer.parameters().Raw(), params)

	// inputV := input.Vector()
	// cpuInput := &tensor.SimpleTensor{}
	// cpuInput.Init(inputV, input.Size())
	// cpuOut := l.cpuLayer.Backward(cpuInput).Vector()
	// gpuOut := output.Vector()

	// for i := 0; i < len(cpuOut); i++ {
	// 	diff := math.Abs(gpuOut[i] - cpuOut[i])
	// 	if diff > 1e-5 {
	// 		log.Panicf("Mismatch at %d, expected %f, but get %l.",
	// 			i, cpuOut[i], gpuOut[i])
	// 	}
	// }

	// cpuGradient := l.cpuLayer.Gradients().Raw()
	// gpuGradient := l.Gradients().Raw()

	// for i := 0; i < len(cpuGradient); i++ {
	// 	diff := math.Abs(gpuGradient[i] - cpuGradient[i])
	// 	if diff > 1e-3 {
	// 		log.Panicf("Mismatch at %d, expected %f, but get %l.",
	// 			i, cpuGradient[i], gpuGradient[i])
	// 	}
	// }

	// log.Printf("Conv2D backward verification passed!")
}

func (l *Conv2D) flipped(input driver.GPUPtr, output driver.GPUPtr) {
	gridSize := l.kernelSize[0] * l.kernelSize[2] * l.kernelSize[3] * l.kernelSize[1]

	queue := l.GPUDriver.CreateCommandQueue(l.GPUCtx)
	kernArg := KernelArgsFlatten{
		input,
		output,
		uint32(l.kernelSize[1]),
		uint32(l.kernelSize[0]),
		uint32(l.kernelSize[2]),
		uint32(l.kernelSize[3]),
		uint64(gridSize), 0, 0,
	}

	l.GPUDriver.EnqueueLaunchKernel(
		queue,
		l.im2colNCHWKernel,
		[3]uint32{uint32(gridSize), 1, 1},
		[3]uint16{uint16(64), 1, 1},
		&kernArg,
	)

	l.GPUDriver.DrainCommandQueue(queue)

	return
}

func numElements(size []int) int {
	product := 1
	for _, s := range size {
		product *= s
	}
	return product
}

func (l *Conv2D) inputBatchSize(input tensor.Tensor) int {
	switch input.Descriptor() {
	case "", "NCHW":
		return input.Size()[0]
	case "CNHW":
		return input.Size()[1]
	default:
		panic("tensor type " + input.Descriptor() + "is not supported")
	}
}

func (l *Conv2D) inputChannelSize(input tensor.Tensor) int {
	switch input.Descriptor() {
	case "", "NCHW":
		return input.Size()[1]
	case "CNHW":
		return input.Size()[0]
	default:
		panic("tensor type " + input.Descriptor() + "is not supported")
	}
}

// Forward processes the forward pass over the convolutional layer.
//nolint:funlen
func (l *Conv2D) Forward(inputTensor tensor.Tensor) tensor.Tensor {
	l.inputSizeMustMatch(inputTensor)

	save := inputTensor.(*Tensor)
	l.saveInput(save)

	batchSize := l.inputBatchSize(inputTensor)

	input := save
	outputSize := []int{
		l.outputSize[0],
		batchSize,
		l.outputSize[1],
		l.outputSize[2],
	}

	outputHeight := outputSize[2]
	outputWidth := outputSize[3]

	im2ColMatrixHeight := l.numChannels() * l.kernelWidth() * l.kernelHeight()
	im2ColMatrixWidth := outputWidth * outputHeight * batchSize
	im2ColMatrix := l.TensorOperator.CreateTensor(
		[]int{im2ColMatrixHeight, im2ColMatrixWidth})
	defer l.TensorOperator.Free(im2ColMatrix)

	l.im2Col(input, im2ColMatrix,
		[2]int{l.kernelWidth(), l.kernelHeight()},
		[4]int{l.padding[0], l.padding[1], l.padding[2], l.padding[3]},
		[2]int{l.stride[0], l.stride[1]},
		[2]int{0, 0},
	)

	kernelMatrixWidth := l.kernelWidth() * l.kernelHeight() * l.numChannels()
	kernelMatrixHeight := l.numKernels()
	kernelMatrix := l.kernel.AsMatrix(kernelMatrixHeight, kernelMatrixWidth)

	hKernelData := make([]float32, kernelMatrixWidth*kernelMatrixHeight)
	l.GPUDriver.MemCopyD2H(l.GPUCtx, hKernelData, kernelMatrix.ptr)

	outputMatrix := l.TensorOperator.CreateTensor(
		[]int{kernelMatrixHeight, im2ColMatrixWidth})
	biasMatrix := l.TensorOperator.CreateTensor(
		[]int{kernelMatrixHeight, im2ColMatrixWidth})

	l.TensorOperator.Gemm(
		false, false,
		kernelMatrixHeight,
		im2ColMatrixWidth,
		kernelMatrixWidth,
		1.0, 1.0,
		kernelMatrix, im2ColMatrix, biasMatrix, outputMatrix)

	output := &Tensor{
		driver:     l.GPUDriver,
		ctx:        l.GPUCtx,
		size:       outputSize,
		ptr:        outputMatrix.ptr,
		descriptor: "CNHW",
	}
	fmt.Printf(l.TensorOperator.Dump("im2col", im2ColMatrix))
	fmt.Printf(l.TensorOperator.Dump("kernel matrix", kernelMatrix))
	fmt.Printf(l.TensorOperator.Dump("bias matrix", biasMatrix))
	fmt.Printf(l.TensorOperator.Dump("gemm out matrix", outputMatrix))
	fmt.Printf(l.TensorOperator.Dump("Forward output, pre trans", output))

	transposedOutput := l.TensorOperator.CreateTensor([]int{
		batchSize,
		l.outputSize[0],
		l.outputSize[1],
		l.outputSize[2]})
	l.TensorOperator.TransposeTensor(
		output, transposedOutput, []int{1, 0, 2, 3})

	l.TensorOperator.Free(biasMatrix)
	l.TensorOperator.Free(output)

	fmt.Printf(l.TensorOperator.Dump("Forward output", transposedOutput))

	return transposedOutput
}

func (l *Conv2D) inputSizeMustMatch(inputTensor tensor.Tensor) {
	if l.inputChannelSize(inputTensor) != l.inputSize[0] ||
		inputTensor.Size()[2] != l.inputSize[1] ||
		inputTensor.Size()[3] != l.inputSize[2] {
		panic("input dimension not correct")
	}
}

// Backward performs the backward pass over the convoluational layer.
func (l *Conv2D) Backward(inputTensor tensor.Tensor) {
	// for i := range l.gradients {
	// 	l.gradients[i] = 0
	// }
	// input := inputTensor.(*tensor.SimpleTensor)
	output := &tensor.SimpleTensor{}
	l.calculateWeightGradients(inputTensor)
	// l.calculateBiasGradients(inputTensor)
	// l.calculateInputGradients(inputTensor)
	output.Init(l.inputGradients.Raw(), l.inputSize)
	return
}

func (l *Conv2D) calculateInputGradients(input tensor.Tensor) {
	// // sizeOfFloat := 4
	// outputGradient := input.(*Tensor)
	// // outputGradient := tempInput.ptr

	// // inputHeight := l.inputSize[1] + l.padding[0] + l.padding[1]
	// // inputWidth := l.inputSize[2] + l.padding[1] + l.padding[3]
	// inputChannelNum := l.inputSize[0]
	// // inputChannelSize := inputHeight * inputWidth
	// //inputTotalSize := inputChannelNum * inputChannelSize

	// outputHeight := l.outputSize[1]
	// outputWidth := l.outputSize[2]
	// outputChannelNum := l.outputSize[0]
	// outputChannelSize := outputHeight * outputWidth
	// // outputTotalSize := outputChannelNum * outputChannelSize

	// kernelHeight := l.kernelSize[2]
	// kernelWidth := l.kernelSize[3]
	// kernelChannelSize := kernelHeight * kernelWidth
	// // kernelTotalSize := outputChannelNum * inputChannelNum * kernelChannelSize
	// outputGradient.size = []int{outputChannelSize, outputChannelNum}

	// ColData := NewTensor(l.GPUDriver, l.GPUCtx)
	// ColData.Init(
	// 	make([]float64, outputChannelSize*kernelChannelSize*inputChannelNum),
	// 	[]int{outputChannelSize, kernelChannelSize * inputChannelNum})

	// zeroMatrix := NewTensor(l.GPUDriver, l.GPUCtx)
	// zeroMatrix.Init(
	// 	make([]float64, outputChannelSize*kernelChannelSize*inputChannelNum),
	// 	[]int{outputChannelSize, kernelChannelSize * inputChannelNum},
	// )
	// // GPU call one: gemm(dOutputGradient, dKernel) -> dColData
	// // GPU call two: Col2im(dColData) -> dimputGradientData
	// weightMatrix := l.kernel.AsMatrix(kernelChannelSize*inputChannelNum, outputChannelNum)
	// weightMatrixTrans := l.TensorOperator.CreateTensor(
	// 	[]int{outputChannelNum, kernelChannelSize * inputChannelNum})
	// l.TensorOperator.Transpose(weightMatrix, weightMatrixTrans)

	// fmt.Println(outputChannelSize, outputChannelNum, kernelChannelSize*inputChannelNum)
	// l.TensorOperator.Gemm(false, false,
	// 	outputChannelSize, kernelChannelSize*inputChannelNum, outputChannelNum,
	// 	1.0, 1.0,
	// 	outputGradient.Matrix(), weightMatrixTrans, zeroMatrix.Matrix(),
	// 	ColData.Matrix())

	// l.col2im(ColData) //TODO: page not found error

	// // l.TensorOperator.Free(weightMatrixTrans)
	return
}

func (l *Conv2D) calculateWeightGradients(input tensor.Tensor) {
	im2ColHeight := l.kernelWidth() * l.kernelHeight() * l.numChannels()
	im2ColWidth := l.kernelWidth() * l.kernelHeight()

	im2ColMat := l.TensorOperator.CreateTensor(
		[]int{im2ColHeight, im2ColWidth})
	l.im2Col(l.forwardInput, im2ColMat,
		[2]int{l.kernelWidth(), l.kernelHeight()},
		[4]int{l.padding[0], l.padding[1], l.padding[2], l.padding[3]},
		[2]int{l.stride[0], l.stride[1]},
		[2]int{0, 0},
	)

	fmt.Println(l.TensorOperator.Dump("Im2Col", im2ColMat))

	backwardInMat := input.(*Tensor).Reshape([]int{
		l.numKernels(),
		l.kernelWidth() * l.kernelHeight() * l.numChannels(),
	})

	biasMat := l.TensorOperator.CreateTensor([]int{l.numKernels(), im2ColWidth})
	weightGradientMat := &Tensor{
		size: []int{l.numKernels(), im2ColWidth},
		ptr:  l.weightGradients.ptr,
	}

	l.TensorOperator.Gemm(false, false,
		l.numKernels(), im2ColWidth, im2ColHeight,
		1, 1,
		backwardInMat, im2ColMat, biasMat, weightGradientMat)

	fmt.Printf(l.TensorOperator.Dump("Weight Gradient", weightGradientMat))
}

func (l *Conv2D) calculateBiasGradients(input tensor.Tensor) {
	// outputTotalSize := l.outputSize[0] * l.outputSize[1] * l.outputSize[2]
	outputChannelNum := l.outputSize[0]
	outputImageSize := l.outputSize[1] * l.outputSize[2]

	inputV := input.Vector()
	biasV := l.biasGradients.Raw()

	for i := 0; i < outputImageSize; i++ {
		for j := 0; j < outputChannelNum; j++ {
			index := i*outputChannelNum + j
			biasV[j] += inputV[index]
		}
	}

	tempData := make([]float32, outputChannelNum)
	for i, value := range biasV {
		tempData[i] = float32(value)
	}

	l.GPUDriver.MemCopyH2D(l.GPUCtx, l.biasGradients.ptr, tempData)
}

// func (l *Conv2D) col2im(input *Tensor) {
// 	ColData := input.Matrix().data
// 	queue := l.GPUDriver.CreateCommandQueue(l.GPUCtx)

// 	inputHeight := l.inputSize[1] + l.padding[0] + l.padding[1]
// 	inputWidth := l.inputSize[2] + l.padding[1] + l.padding[3]
// 	inputChannelNum := l.inputSize[0]
// 	inputChannelSize := inputHeight * inputWidth
// 	inputTotalSize := inputChannelNum * inputChannelSize

// 	gridSize := uint32(inputTotalSize)
// 	kernArg := KernelArgsCol2Im{
// 		int32(l.inputSize[1]), int32(l.inputSize[2]), int32(l.inputSize[0]),
// 		int32(l.outputSize[1]), int32(l.outputSize[2]),
// 		int32(l.kernelSize[2]), int32(l.kernelSize[3]),
// 		int32(l.padding[0]), int32(l.padding[1]),
// 		int32(l.stride[0]), int32(l.stride[1]),
// 		int32(1), int32(1),
// 		ColData, int32(0),
// 		l.inputGradients.ptr, int32(0),

// 		0, 0, 0,
// 	}
// 	l.GPUDriver.EnqueueLaunchKernel(
// 		queue,
// 		l.col2imKernel,
// 		[3]uint32{gridSize, 1, 1},
// 		[3]uint16{uint16(64), 1, 1},
// 		&kernArg,
// 	)

// 	l.GPUDriver.DrainCommandQueue(queue)

// 	return
// }

func (l *Conv2D) im2Col(
	input *Tensor,
	im2ColMatrix *Tensor,
	maskDimension [2]int,
	padding [4]int,
	stride [2]int,
	dilation [2]int,
) {
	switch input.Descriptor() {
	case "", "NCHW":
		l.im2ColNCHW(input, im2ColMatrix,
			maskDimension, padding, stride, dilation)
	case "CNHW":
		l.im2ColCNHW(input, im2ColMatrix,
			maskDimension, padding, stride, dilation)
	default:
		panic("unsupported tensor type " + input.Descriptor())
	}
}

func (l *Conv2D) groupSize(maskSize, dilation int) int {
	return maskSize + (maskSize-1)*dilation
}

func (l *Conv2D) fieldSize(
	inputSize, maskSize, dilation, stride, padding int) int {
	totalInput := inputSize + padding
	groupSize := l.groupSize(maskSize, dilation)
	return (totalInput-groupSize)/stride + 1
}

func (l *Conv2D) im2ColNCHW(
	input *Tensor,
	matrix *Tensor,
	maskDimension [2]int,
	padding [4]int,
	stride [2]int,
	dilation [2]int,
) {
	queue := l.GPUDriver.CreateCommandQueue(l.GPUCtx)
	kernArg := KernelArgsIm2Col{
		input.ptr,
		matrix.ptr,
		[2]uint32{uint32(input.size[3]), uint32(input.size[2])},
		[2]uint32{uint32(maskDimension[0]), uint32(maskDimension[1])},
		[2]uint32{uint32(stride[0]), uint32(stride[1])},
		[2]uint32{uint32(padding[0]), uint32(padding[2])},
		[2]uint32{uint32(padding[3]), uint32(padding[1])},
		[2]uint32{uint32(dilation[0]), uint32(dilation[1])},
		uint32(input.size[1]),
		uint32(input.size[0]),
		0, 0, 0,
	}

	fieldSizeX := l.fieldSize(
		input.size[3], maskDimension[0],
		dilation[0], stride[0], padding[1]+padding[3])
	fieldSizeY := l.fieldSize(
		input.size[2], maskDimension[1],
		dilation[1], stride[1], padding[0]+padding[2])
	gridSize := fieldSizeX * fieldSizeY * input.size[0]

	l.GPUDriver.EnqueueLaunchKernel(
		queue,
		l.im2colNCHWKernel,
		[3]uint32{uint32(gridSize), 1, 1},
		[3]uint16{uint16(64), 1, 1},
		&kernArg,
	)

	l.GPUDriver.DrainCommandQueue(queue)
}

func (l *Conv2D) im2ColCNHW(
	input *Tensor,
	matrix *Tensor,
	maskDimension [2]int,
	padding [4]int,
	stride [2]int,
	dilation [2]int,
) {
	queue := l.GPUDriver.CreateCommandQueue(l.GPUCtx)
	kernArg := KernelArgsIm2Col{
		input.ptr,
		matrix.ptr,
		[2]uint32{uint32(input.size[3]), uint32(input.size[2])},
		[2]uint32{uint32(maskDimension[0]), uint32(maskDimension[1])},
		[2]uint32{uint32(stride[0]), uint32(stride[1])},
		[2]uint32{uint32(padding[0]), uint32(padding[2])},
		[2]uint32{uint32(padding[3]), uint32(padding[1])},
		[2]uint32{uint32(dilation[0]), uint32(dilation[1])},
		uint32(input.size[0]),
		uint32(input.size[1]),
		0, 0, 0,
	}

	fieldSizeX := l.fieldSize(
		input.size[3], maskDimension[0],
		dilation[0], stride[0], padding[1]+padding[3])
	fieldSizeY := l.fieldSize(
		input.size[2], maskDimension[1],
		dilation[1], stride[1], padding[0]+padding[2])
	gridSize := fieldSizeX * fieldSizeY * input.size[1]

	l.GPUDriver.EnqueueLaunchKernel(
		queue,
		l.im2colCNHWKernel,
		[3]uint32{uint32(gridSize), 1, 1},
		[3]uint16{uint16(64), 1, 1},
		&kernArg,
	)

	l.GPUDriver.DrainCommandQueue(queue)
}

func (l *Conv2D) calculateOutputSize() {
	width := (l.inputSize[2]-l.kernelWidth()+l.padding[1]+l.padding[3])/
		l.stride[1] + 1
	height := (l.inputSize[1]-l.kernelHeight()+l.padding[0]+l.padding[2])/
		l.stride[0] + 1
	channel := l.numKernels()
	l.outputSize = []int{channel, height, width}
}

/*$$$$$$$$$$$$$$$$$$$$$$$$$$$$CPU_VERSION$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$*/

// NewConvolutionalLayer creates a new convolutional layer for the DNN network.
// The inputSize should be a 3-number array representing [channel, height,
// width]. The kernel size should be a 4-number array, representing [output
// channel, input channel, height, width]. Stride is a 2-number array
// representing [vertical stride, horizontal stride]. Padding is a 4-number
// array, representing [top padding, right padding, bottom padding, left
// padding].
