package layers

import (

	// "math"
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
	gradients       *Vector //TODO: useful?
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

type KernelArgsim2col struct {
	Input                                                                                driver.GPUPtr
	Output                                                                               driver.GPUPtr
	InputDimensions, MaskDimensions, StrDimensions, PadVertDimensions, PadHoriDimensions [2]uint32
	Channel                                                                              uint32
	Batch                                                                                uint32
	OffsetX, OffsetY, OffsetZ                                                            uint64
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

/*$$$$$$$$$$$$$$$$$$$$$$$$$$$$GPU_VERSION$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$*/

func NewConvolutionalLayer(
	inputSize, kernelSize, stride, padding []int, GPUDriver *driver.Driver, GPUCtx *driver.Context, MatrixOperator *MatrixOperator,
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
	hsacoBytes := _escFSMustByte(true, "/im2col.hsaco")
	//clang-ocl -mcpu=gfx803 im2col.cl -o im2col.hsaco
	//clang-ocl -mcpu=gfx803 im2col.cl -S -o im2col.asm
	// source bash
	//esc -private -o esc.go -pkg simpleimcol ./im2col.hsaco

	l.im2colKernel = kernels.LoadProgramFromMemory(hsacoBytes, "im2colKernel")
	if l.im2colKernel == nil {
		log.Panic("Failed to load im2col kernel binary")
	}

	hsacoBytes1 := _escFSMustByte(true, "/col2im.hsaco")

	l.col2imKernel = kernels.LoadProgramFromMemory(hsacoBytes1, "col2imKernel")
	if l.col2imKernel == nil {
		log.Panic("Failed to load col2im kernel binary")
	}

	hsacoBytes2 := _escFSMustByte(true, "/flatten.hsaco")

	l.flatKernel = kernels.LoadProgramFromMemory(hsacoBytes2, "flattenKernel")
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
	numBias := l.outputSize[0] //TODO: the number of bias equals to the output channel size?
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

func (l *Conv2D) verifyForwardPass(input, output *Tensor) {
	//TODO: not finished
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
	//TODO: not finished
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

func (l *Conv2D) Forward(inputTensor tensor.Tensor) tensor.Tensor {
	save := inputTensor.(*Tensor)
	l.saveInput(save)
	sizeOfFloat := 4
	if inputTensor.Size()[0] != l.inputSize[0] ||
		inputTensor.Size()[1] != l.inputSize[1] ||
		inputTensor.Size()[2] != l.inputSize[2] {
		panic("input dimension not correct")
	}

	batchSize := 1 // preserved variable for batchSize
	input := save
	// inputTensor.(*tensor.SimpleTensor)
	output := &tensor.SimpleTensor{}

	outputHeight := l.outputSize[1]
	outputWidth := l.outputSize[2]

	inputHeight := l.inputSize[1]
	inputWidth := l.inputSize[2]

	fieldWidth := (inputWidth-l.kernelSize[3]+l.padding[1]+l.padding[3])/l.stride[1] + 1
	fieldHeight := (inputHeight-l.kernelSize[2]+l.padding[0]+l.padding[2])/l.stride[0] + 1

	inputTotalSize := l.inputSize[0] * l.inputSize[1] * l.inputSize[2]
	//imcolOutputSize := fieldWidth * fieldHeight * l.kernelSize[2] * l.kernelSize[3] * l.kernelSize[1]
	outputTotalSize := l.outputSize[0] * outputHeight * outputWidth
	//kernelTotalSize := l.kernelSize[0] * l.kernelSize[2] * l.kernelSize[3]

	cpuOutput := make([]float64, outputTotalSize)
	// assume kernel in kernelTEMP

	dInputData := l.GPUDriver.AllocateMemory(l.GPUCtx,
		uint64(inputTotalSize*sizeOfFloat))

	// assuming kernel is in
	kernel_b := l.kernel.AsMatrix(l.outputSize[0], l.kernelSize[2]*l.kernelSize[3]*l.kernelSize[1])
	kernelM := l.MatrixOperator.CreateMatrix(l.outputSize[0], l.kernelSize[2]*l.kernelSize[3]*l.kernelSize[1])
	im2colM := l.MatrixOperator.CreateMatrix(l.kernelSize[2]*l.kernelSize[3]*l.kernelSize[1], fieldHeight*fieldWidth)
	outputM := l.MatrixOperator.CreateMatrix(l.outputSize[0], fieldHeight*fieldWidth)
	biasM := l.MatrixOperator.CreateMatrix(l.outputSize[0], fieldHeight*fieldWidth)

	dim2colData := im2colM.data
	dOutputData := outputM.data
	// dKernel := kernelM.data

	l.GPUDriver.MemCopyH2D(l.GPUCtx, dInputData, input.ptr)
	//l.GPUDriver.MemCopyH2D(l.GPUCtx, dKernel, l.kernelTEMP)

	gridSize := fieldWidth * fieldHeight * l.kernelSize[1]
	// need to be changed, since it is not standard number for a kernel call
	/*
		gridSize := ((b.Width + b.padWidth) * (b.Height + b.padHeight)) /
			uint32(len(b.gpus))
	*/
	l.flipped(kernel_b.data, kernelM.data)
	l.im2col(dInputData, dim2colData, l.kernelSize[1], batchSize, gridSize)
	l.MatrixOperator.Gemm(false, false,
		l.outputSize[0], l.kernelSize[2]*l.kernelSize[3]*l.kernelSize[1], fieldHeight*fieldWidth,
		1.0, 1.0,
		kernelM, im2colM, biasM, outputM)

	l.MatrixOperator.Free(biasM)

	l.GPUDriver.MemCopyD2H(l.GPUCtx, cpuOutput, dOutputData)
	output.Init(cpuOutput, l.outputSize)
	return output
}

func (l *Conv2D) Backward(inputTensor tensor.Tensor) {

	// for i := range l.gradients {
	// 	l.gradients[i] = 0
	// }
	// input := inputTensor.(*tensor.SimpleTensor)
	output := &tensor.SimpleTensor{}

	l.calculateWeightGradients(inputTensor)
	// l.calculateBiasGradients(input) //TODO: add bias
	l.calculateInputGradients(inputTensor)
	output.Init(l.inputGradients.Raw(), l.inputSize)
	return
}

func (l *Conv2D) calculateInputGradients(input tensor.Tensor) {
	sizeOfFloat := 4
	tempInput := input.(*Tensor)
	outputGradient := tempInput.Vector()

	// inputHeight := l.inputSize[1] + l.padding[0] + l.padding[1]
	// inputWidth := l.inputSize[2] + l.padding[1] + l.padding[3]
	inputChannelNum := l.inputSize[0]
	// inputChannelSize := inputHeight * inputWidth
	//inputTotalSize := inputChannelNum * inputChannelSize

	outputHeight := l.outputSize[1]
	outputWidth := l.outputSize[2]
	outputChannelNum := l.outputSize[0]
	outputChannelSize := outputHeight * outputWidth
	outputTotalSize := outputChannelNum * outputChannelSize

	kernelHeight := l.kernelSize[2]
	kernelWidth := l.kernelSize[3]
	kernelChannelSize := kernelHeight * kernelWidth
	// kernelTotalSize := outputChannelNum * inputChannelNum * kernelChannelSize

	dOutputGradient := l.GPUDriver.AllocateMemory(l.GPUCtx,
		uint64(outputTotalSize*sizeOfFloat))

	ColData := &Tensor{
		driver: l.GPUDriver,
		ctx:    l.GPUCtx,
		size:   []int{outputChannelSize, kernelChannelSize * inputChannelNum * sizeOfFloat}, // TODO: correct?
		ptr:    l.GPUDriver.AllocateMemory(l.GPUCtx, uint64(outputChannelSize*kernelChannelSize*inputChannelNum)),
	}

	zeroMatrix := NewTensor(l.GPUDriver, l.GPUCtx)
	zeroMatrix.Init(
		make([]float64, outputChannelSize*kernelChannelSize*inputChannelNum),
		[]int{outputChannelSize, kernelChannelSize * inputChannelNum}, //TODO: correct?
	)

	l.GPUDriver.MemCopyH2D(l.GPUCtx, dOutputGradient, outputGradient)
	OutputGradientT := &Tensor{
		driver: l.GPUDriver,
		ctx:    l.GPUCtx,
		size:   []int{outputChannelSize, outputChannelNum}, // TODO: correct?
		ptr:    dOutputGradient,
	}

	// GPU call one: gemm(dOutputGradient, dKernel) -> dColData
	// GPU call two: Col2im(dColData) -> dimputGradientData
	weightMatrix := l.kernel.AsMatrix(kernelChannelSize*inputChannelNum, outputChannelNum)
	// weightMatrixTrans := l.MatrixOperator.CreateMatrix(l.outputSize, l.InputSize)
	// l.MatrixOperator.Transpose(weightMatrix, weightMatrixTrans)

	// ColDataMatrix := dColData.AsMatrix(outputChannelSize, kernelTotalSize)

	l.MatrixOperator.Gemm(false, true,
		outputChannelSize, outputChannelNum, kernelChannelSize*inputChannelNum, //TODO: correct?
		1.0, 1.0,
		OutputGradientT.Matrix(), weightMatrix, zeroMatrix.Matrix(),
		ColData.Matrix())

	l.col2im(ColData)

	// l.MatrixOperator.Free(weightMatrixTrans)
	return
}

func (l *Conv2D) calculateWeightGradients(input tensor.Tensor) {
	sizeOfFloat := 4
	// tempInput := input.(*tensor.SimpleTensor)
	tempInput := input.(*Tensor)
	outputGradient := tempInput.Vector()

	// inputHeight := l.inputSize[1] + l.padding[0] + l.padding[1]
	// inputWidth := l.inputSize[2] + l.padding[1] + l.padding[3]
	inputChannelNum := l.inputSize[0]
	// inputChannelSize := inputHeight * inputWidth
	//inputTotalSize := inputChannelNum * inputChannelSize

	outputHeight := l.outputSize[1]
	outputWidth := l.outputSize[2]
	outputChannelNum := l.outputSize[0]
	outputChannelSize := outputHeight * outputWidth
	outputTotalSize := outputChannelNum * outputChannelSize

	kernelHeight := l.kernelSize[2]
	kernelWidth := l.kernelSize[3]
	kernelChannelSize := kernelHeight * kernelWidth
	kernelTotalSize := outputChannelNum * inputChannelNum * kernelChannelSize

	colSize := outputChannelSize * kernelChannelSize * outputChannelNum

	dOutputGradient := l.GPUDriver.AllocateMemory(l.GPUCtx,
		uint64(outputTotalSize*sizeOfFloat))
	dIm2colData := l.GPUDriver.AllocateMemory(l.GPUCtx,
		uint64(colSize*sizeOfFloat))

	l.GPUDriver.MemCopyH2D(l.GPUCtx, dOutputGradient, outputGradient)
	OutputGradientT := &Tensor{
		driver: l.GPUDriver,
		ctx:    l.GPUCtx,
		size:   []int{outputChannelSize, outputChannelNum}, // TODO: correct?
		ptr:    dOutputGradient,
	}
	batchSize := 1
	gridSize := outputChannelSize
	l.im2col(l.forwardInput, dIm2colData, l.kernelSize[1], batchSize, gridSize)

	// GPU call one: im2col(dInputData) -> dIm2colData
	// GPU call two: Gemm(dIm2colData, dOutputGradient) -> dWeightGradientData

	weightGradientM := l.weightGradients.AsMatrix(kernelChannelSize*inputChannelNum, outputChannelNum) //TODO: correct?

	dIm2colTensor := &Tensor{
		driver: l.GPUDriver,
		ctx:    l.GPUCtx,
		size:   []int{outputChannelSize, kernelChannelSize * inputChannelNum}, // TODO: correct?
		ptr:    dIm2colData,
	}

	zeroMatrix := NewTensor(l.GPUDriver, l.GPUCtx)
	zeroMatrix.Init(
		make([]float64, kernelTotalSize),
		[]int{kernelChannelSize * inputChannelNum, outputChannelNum}, //TODO: correct?
	)

	l.MatrixOperator.Gemm(false, false,
		kernelChannelSize*inputChannelNum, outputChannelSize, outputChannelNum, //TODO: correct?
		1.0, 1.0,
		dIm2colTensor.Matrix(), OutputGradientT.Matrix(), zeroMatrix.Matrix(),
		weightGradientM)

	// l.MatrixOperator.Free(weightMatrixTrans)
	return
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

		uint64(gridSize), 0, 0,
	}
	// fmt.Println("    ~~~~~~~~~    ")
	l.GPUDriver.EnqueueLaunchKernel(
		queue,
		l.col2imKernel, //TODO: replace kernel
		[3]uint32{gridSize, 1, 1},
		[3]uint16{uint16(64), 1, 1},
		&kernArg,
	)
	// fmt.Println("    ??????????    ")
	l.GPUDriver.DrainCommandQueue(queue)

	// fmt.Println("    !!!!!!!!!!    ")

	return
}

func (l *Conv2D) im2col(dInputData driver.GPUPtr, dim2colData driver.GPUPtr, channel int, batchSize int, gridSize int) {

	queue := l.GPUDriver.CreateCommandQueue(l.GPUCtx)
	kernArg := KernelArgsim2col{
		dInputData,
		dim2colData,
		[2]uint32{uint32(l.inputSize[2]), uint32(l.inputSize[1])},
		[2]uint32{uint32(l.kernelSize[3]), uint32(l.kernelSize[2])},
		[2]uint32{uint32(l.stride[0]), uint32(l.stride[1])},
		[2]uint32{uint32(l.padding[0]), uint32(l.padding[2])},
		[2]uint32{uint32(l.padding[3]), uint32(l.padding[1])},
		uint32(channel),
		uint32(batchSize),
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

func (l *Conv2D) calculateOutputSize() {
	width := (l.inputSize[2]-l.kernelSize[3]+l.padding[1]+l.padding[3])/
		l.stride[1] + 1
	height := (l.inputSize[1]-l.kernelSize[2]+l.padding[0]+l.padding[2])/
		l.stride[0] + 1
	channel := l.kernelSize[0]
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
