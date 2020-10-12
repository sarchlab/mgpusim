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
	MatrixOperator *MatrixOperator

	forwardInput driver.GPUPtr

	verifyForward  bool
	verifyBackward bool
	// cpuLayer       *layers.Conv2D

	inputSize, outputSize []int
	kernelSize            []int
	stride                []int
	padding               []int
	// bias                  []int

	im2colKernel *insts.HsaCo
	col2imKernel *insts.HsaCo
	flatKernel   *insts.HsaCo

	parameters      *Vector
	kernel          *Vector
	bias            *Vector
	gradients       *Vector
	inputGradients  *Vector
	weightGradients *Vector
	biasGradients   *Vector
}

type KernelArgsCol2im struct {
	Input_h, Input_w, Channels, Output_h, Output_w, Kernel_h, Kernel_w, Pad_h, Pad_w, Stride_h, Stride_w, Dilation_h, Dilation_w int32
	Col_buffer                                                                                                                   driver.GPUPtr
	Col_offset                                                                                                                   int32
	Im_buffer                                                                                                                    driver.GPUPtr
	Im_offset                                                                                                                    int32
	OffsetX, OffsetY, OffsetZ                                                                                                    uint64
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
	Channel                   uint32
	Batch                     uint32
	OffsetX, OffsetY, OffsetZ uint64
}

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
	MatrixOperator *MatrixOperator,
) *Conv2D {
	// argumentsMustBeValid(inputSize, kernelSize, stride, padding)

	l := &Conv2D{
		inputSize:      inputSize,
		kernelSize:     kernelSize,
		stride:         stride,
		padding:        padding,
		GPUDriver:      GPUDriver,
		GPUCtx:         GPUCtx,
		MatrixOperator: MatrixOperator,
	}
	l.calculateOutputSize()
	l.loadKernels()
	l.allocateMemory()

	return l
}

func (l *Conv2D) loadKernels() {
	im2colHsaCoBytes := _escFSMustByte(true, "/im2col.hsaco")
	l.im2colKernel = kernels.LoadProgramFromMemory(
		im2colHsaCoBytes, "im2colKernel")
	if l.im2colKernel == nil {
		log.Panic("Failed to load im2col kernel binary")
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
	if l.forwardInput != 0 {
		l.GPUDriver.FreeMemory(l.GPUCtx, l.forwardInput)
	}

	numElement := input.Size()[0] * input.Size()[1] * input.Size()[2]
	l.forwardInput = l.GPUDriver.AllocateMemory(l.GPUCtx,
		uint64(numElement*4))

	temp := make([]float32, numElement)
	l.GPUDriver.MemCopyD2H(l.GPUCtx, temp, input.ptr)
	l.GPUDriver.MemCopyH2D(l.GPUCtx, l.forwardInput, temp)
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
		l.im2colKernel,
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

func (l *Conv2D) Forward(inputTensor tensor.Tensor) tensor.Tensor {
	l.inputSizeMustMatch(inputTensor)

	sizeOfFloat := 4

	save := inputTensor.(*Tensor)
	l.saveInput(save)

	batchSize := 1 // preserved variable for batchSize
	input := save
	// inputTensor.(*tensor.SimpleTensor)

	outputSize := []int{
		input.Size()[0],
		l.outputSize[0],
		l.outputSize[1],
		l.outputSize[2],
	}
	outputElements := numElements(outputSize)
	output := &Tensor{
		driver: l.GPUDriver,
		ctx:    l.GPUCtx,
		size:   outputSize,
		ptr: l.GPUDriver.AllocateMemory(l.GPUCtx,
			uint64(outputElements*sizeOfFloat)),
	}

	outputHeight := l.outputSize[1]
	outputWidth := l.outputSize[2]

	// inputHeight := l.inputSize[1]
	// inputWidth := l.inputSize[2]

	// kernel_b := l.kernel.AsMatrix(l.outputSize[0], l.kernelSize[2]*l.kernelSize[3]*l.kernelSize[1])
	// kernelM := l.MatrixOperator.CreateMatrix(l.outputSize[0], l.kernelSize[2]*l.kernelSize[3]*l.kernelSize[1])
	im2ColMatrix := l.MatrixOperator.CreateMatrix(
		l.numChannels()*l.kernelWidth()*l.kernelHeight(),
		outputWidth*outputHeight*l.numKernels(),
	)
	// outputM := l.MatrixOperator.CreateMatrix(l.outputSize[0], fieldHeight*fieldWidth)
	// biasM := l.MatrixOperator.CreateMatrix(l.outputSize[0], fieldHeight*fieldWidth)

	dIm2ColData := im2ColMatrix.data
	// dOutputData := outputM.data
	// dKernel := kernelM.data

	// l.GPUDriver.MemCopyH2D(l.GPUCtx, dInputData, input.ptr)
	//l.GPUDriver.MemCopyH2D(l.GPUCtx, dKernel, l.kernelTEMP)

	gridSize := outputWidth * outputHeight * l.numChannels()
	// need to be changed, since it is not standard number for a kernel call
	/*
		gridSize := ((b.Width + b.padWidth) * (b.Height + b.padHeight)) /
			uint32(len(b.gpus))
	*/
	// l.flipped(kernel_b.data, kernelM.data)
	hInputData := make([]float32, 3*3)
	l.GPUDriver.MemCopyD2H(l.GPUCtx, hInputData, input.ptr)
	fmt.Println("Forward, input Data ", hInputData)

	l.im2col(input.ptr, dIm2ColData, l.numChannels(), batchSize, gridSize)

	hIm2ColData := make([]float32, im2ColMatrix.col*im2ColMatrix.row)
	l.GPUDriver.MemCopyD2H(l.GPUCtx, hIm2ColData, dIm2ColData)
	fmt.Println(hIm2ColData)

	// l.MatrixOperator.Gemm(false, false,
	// 	l.outputSize[0], l.kernelSize[2]*l.kernelSize[3]*l.kernelSize[1], fieldHeight*fieldWidth,
	// 	1.0, 1.0,
	// 	kernelM, im2colM, biasM, outputM)

	// l.MatrixOperator.Free(biasM)

	// l.GPUDriver.MemCopyD2H(l.GPUCtx, cpuOutput, dOutputData)
	// output.Init(cpuOutput, l.outputSize)
	return output
}

func (l *Conv2D) inputSizeMustMatch(inputTensor tensor.Tensor) {
	if inputTensor.Size()[0] != l.inputSize[0] ||
		inputTensor.Size()[1] != l.inputSize[1] ||
		inputTensor.Size()[2] != l.inputSize[2] {
		panic("input dimension not correct")
	}
}

func (l *Conv2D) Backward(inputTensor tensor.Tensor) {

	// for i := range l.gradients {
	// 	l.gradients[i] = 0
	// }
	// input := inputTensor.(*tensor.SimpleTensor)
	output := &tensor.SimpleTensor{}
	l.calculateWeightGradients(inputTensor)
	l.calculateBiasGradients(inputTensor)
	l.calculateInputGradients(inputTensor)
	output.Init(l.inputGradients.Raw(), l.inputSize)
	return
}

func (l *Conv2D) calculateInputGradients(input tensor.Tensor) {
	// sizeOfFloat := 4
	outputGradient := input.(*Tensor)
	// outputGradient := tempInput.ptr

	// inputHeight := l.inputSize[1] + l.padding[0] + l.padding[1]
	// inputWidth := l.inputSize[2] + l.padding[1] + l.padding[3]
	inputChannelNum := l.inputSize[0]
	// inputChannelSize := inputHeight * inputWidth
	//inputTotalSize := inputChannelNum * inputChannelSize

	outputHeight := l.outputSize[1]
	outputWidth := l.outputSize[2]
	outputChannelNum := l.outputSize[0]
	outputChannelSize := outputHeight * outputWidth
	// outputTotalSize := outputChannelNum * outputChannelSize

	kernelHeight := l.kernelSize[2]
	kernelWidth := l.kernelSize[3]
	kernelChannelSize := kernelHeight * kernelWidth
	// kernelTotalSize := outputChannelNum * inputChannelNum * kernelChannelSize
	outputGradient.size = []int{outputChannelSize, outputChannelNum}

	ColData := NewTensor(l.GPUDriver, l.GPUCtx)
	ColData.Init(
		make([]float64, outputChannelSize*kernelChannelSize*inputChannelNum),
		[]int{outputChannelSize, kernelChannelSize * inputChannelNum})

	zeroMatrix := NewTensor(l.GPUDriver, l.GPUCtx)
	zeroMatrix.Init(
		make([]float64, outputChannelSize*kernelChannelSize*inputChannelNum),
		[]int{outputChannelSize, kernelChannelSize * inputChannelNum},
	)
	// GPU call one: gemm(dOutputGradient, dKernel) -> dColData
	// GPU call two: Col2im(dColData) -> dimputGradientData
	weightMatrix := l.kernel.AsMatrix(kernelChannelSize*inputChannelNum, outputChannelNum)
	weightMatrixTrans := l.MatrixOperator.CreateMatrix(outputChannelNum, kernelChannelSize*inputChannelNum)
	l.MatrixOperator.Transpose(weightMatrix, weightMatrixTrans)

	fmt.Println(outputChannelSize, outputChannelNum, kernelChannelSize*inputChannelNum)
	l.MatrixOperator.Gemm(false, false,
		outputChannelSize, kernelChannelSize*inputChannelNum, outputChannelNum,
		1.0, 1.0,
		outputGradient.Matrix(), weightMatrixTrans, zeroMatrix.Matrix(),
		ColData.Matrix())

	l.col2im(ColData) //TODO: page not found error

	// l.MatrixOperator.Free(weightMatrixTrans)
	return
}

func (l *Conv2D) calculateWeightGradients(input tensor.Tensor) {
	sizeOfFloat := 4
	// tempInput := input.(*tensor.SimpleTensor)
	outputGradient := input.(*Tensor)

	// inputHeight := l.inputSize[1] + l.padding[0] + l.padding[1]
	// inputWidth := l.inputSize[2] + l.padding[1] + l.padding[3]
	inputChannelNum := l.inputSize[0]
	// inputChannelSize := inputHeight * inputWidth
	//inputTotalSize := inputChannelNum * inputChannelSize

	outputHeight := l.outputSize[1]
	outputWidth := l.outputSize[2]
	outputChannelNum := l.outputSize[0]
	outputChannelSize := outputHeight * outputWidth
	// outputTotalSize := outputChannelNum * outputChannelSize

	kernelHeight := l.kernelSize[2]
	kernelWidth := l.kernelSize[3]
	kernelChannelSize := kernelHeight * kernelWidth
	kernelTotalSize := outputChannelNum * inputChannelNum * kernelChannelSize

	outputGradient.size = []int{outputChannelSize, outputChannelNum}

	colSize := outputChannelSize * kernelChannelSize * outputChannelNum
	dIm2colData := l.GPUDriver.AllocateMemory(l.GPUCtx,
		uint64(colSize*sizeOfFloat))

	batchSize := 1
	gridSize := outputChannelSize
	l.im2col(l.forwardInput, dIm2colData, l.kernelSize[1], batchSize, gridSize)

	// GPU call one: im2col(dInputData) -> dIm2colData
	// GPU call two: Gemm(dIm2colData, dOutputGradient) -> dWeightGradientData

	weightGradientM := l.weightGradients.AsMatrix(kernelChannelSize*inputChannelNum, outputChannelNum)

	dIm2colTensor := &Tensor{
		driver: l.GPUDriver,
		ctx:    l.GPUCtx,
		size:   []int{outputChannelSize, kernelChannelSize * inputChannelNum},
		ptr:    dIm2colData,
	}

	zeroMatrix := NewTensor(l.GPUDriver, l.GPUCtx)
	zeroMatrix.Init(
		make([]float64, kernelTotalSize),
		[]int{kernelChannelSize * inputChannelNum, outputChannelNum},
	)

	l.MatrixOperator.Gemm(false, false,
		kernelChannelSize*inputChannelNum, outputChannelNum, outputChannelSize,
		1.0, 1.0,
		dIm2colTensor.Matrix(), outputGradient.Matrix(), zeroMatrix.Matrix(),
		weightGradientM)

	// Output := make([]float32, kernelTotalSize)
	// l.GPUDriver.MemCopyD2H(l.GPUCtx, Output, l.weightGradients.ptr)
	// fmt.Println("WG ", Output)
	// l.MatrixOperator.Free(weightMatrixTrans)
	return
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

func (l *Conv2D) col2im(input *Tensor) {
	ColData := input.Matrix().data
	queue := l.GPUDriver.CreateCommandQueue(l.GPUCtx)

	inputHeight := l.inputSize[1] + l.padding[0] + l.padding[1]
	inputWidth := l.inputSize[2] + l.padding[1] + l.padding[3]
	inputChannelNum := l.inputSize[0]
	inputChannelSize := inputHeight * inputWidth
	inputTotalSize := inputChannelNum * inputChannelSize

	gridSize := uint32(inputTotalSize)
	kernArg := KernelArgsCol2im{
		int32(l.inputSize[1]), int32(l.inputSize[2]), int32(l.inputSize[0]),
		int32(l.outputSize[1]), int32(l.outputSize[2]),
		int32(l.kernelSize[2]), int32(l.kernelSize[3]),
		int32(l.padding[0]), int32(l.padding[1]),
		int32(l.stride[0]), int32(l.stride[1]),
		int32(1), int32(1),
		ColData, int32(0),
		l.inputGradients.ptr, int32(0),

		0, 0, 0,
	}
	l.GPUDriver.EnqueueLaunchKernel(
		queue,
		l.col2imKernel,
		[3]uint32{gridSize, 1, 1},
		[3]uint16{uint16(64), 1, 1},
		&kernArg,
	)

	l.GPUDriver.DrainCommandQueue(queue)

	return
}

func (l *Conv2D) im2col(
	dInputData driver.GPUPtr,
	dIm2ColData driver.GPUPtr,
	channel int,
	batchSize int,
	gridSize int,
) {
	queue := l.GPUDriver.CreateCommandQueue(l.GPUCtx)
	kernArg := KernelArgsIm2Col{
		dInputData,
		dIm2ColData,
		[2]uint32{uint32(l.inputSize[2]), uint32(l.inputSize[1])},
		[2]uint32{uint32(l.kernelSize[3]), uint32(l.kernelSize[2])},
		[2]uint32{uint32(l.stride[0]), uint32(l.stride[1])},
		[2]uint32{uint32(l.padding[0]), uint32(l.padding[2])},
		[2]uint32{uint32(l.padding[3]), uint32(l.padding[1])},
		uint32(channel),
		uint32(batchSize),
		0, 0, 0,
	}

	l.GPUDriver.EnqueueLaunchKernel(
		queue,
		l.im2colKernel,
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
