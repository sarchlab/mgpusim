package layers

import (
	"log"

	"gitlab.com/akita/dnn/layers"
	"gitlab.com/akita/dnn/tensor"
	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/kernels"
)

// ReluLayer implements the ReLU algorithm
type ReluLayer struct {
	GPUDriver *driver.Driver
	GPUCtx    *driver.Context

	verifyForward  bool
	verifyBackward bool
	cpuLayer       *layers.ReluLayer

	forwardKernel  *insts.HsaCo
	backwardKernel *insts.HsaCo

	forwardInput driver.GPUPtr
}

// NewReluLayer creates a new Relu layer
func NewReluLayer(driver *driver.Driver, ctx *driver.Context) *ReluLayer {
	l := &ReluLayer{
		GPUDriver: driver,
		GPUCtx:    ctx,
	}

	kernelBytes := _escFSMustByte(false, "/relu.hsaco")
	l.forwardKernel = kernels.LoadProgramFromMemory(
		kernelBytes, "ReLUForward")
	if l.forwardKernel == nil {
		panic("fail to load relu forward kernel")
	}

	l.backwardKernel = kernels.LoadProgramFromMemory(
		kernelBytes, "ReLUBackward")
	if l.backwardKernel == nil {
		panic("fail to load relu backward kernel")
	}

	return l
}

// EnableVerification runs a CPU pass for every forward and backward propagation
// though the ReLU layer to make sure the simulator is correct.
func (l *ReluLayer) EnableVerification() {
	l.verifyForward = true
	l.verifyBackward = true
	l.cpuLayer = layers.NewReluLayer()
}

// Randomize generates the layer parameters randomly.
func (l ReluLayer) Randomize() {
	// do nothing
}

// ForwardKernelArgs defines the forward propagation kernel arguments.
type ForwardKernelArgs struct {
	Count, Padding int32
	In, Out        driver.GPUPtr
}

// BackwardKernelArgs defines the backward kernel arguments.
type BackwardKernelArgs struct {
	Count, Padding     int32
	ForwardIn, In, Out driver.GPUPtr
}

// Forward performs the forward propagation of the ReLU layer.
func (l *ReluLayer) Forward(inputT tensor.Tensor) tensor.Tensor {
	input := inputT.(*Tensor)
	size := input.Size()
	output := &Tensor{
		driver: l.GPUDriver,
		ctx:    l.GPUCtx,
		size:   size,
		ptr: l.GPUDriver.AllocateMemory(
			l.GPUCtx, uint64(size[0]*size[1]*4)),
	}

	l.saveInput(input)

	kernArg := ForwardKernelArgs{
		Count: int32(size[0] * size[1]),
		In:    input.ptr,
		Out:   output.ptr,
	}

	l.GPUDriver.LaunchKernel(l.GPUCtx, l.forwardKernel,
		[3]uint32{uint32(size[0] * size[1]), 1, 1},
		[3]uint16{64, 1, 1},
		&kernArg)

	l.verifyForwardPass(input, output)

	return output
}

func (l *ReluLayer) verifyForwardPass(input, output *Tensor) {
	if !l.verifyForward {
		return
	}

	inputV := input.Vector()
	cpuInput := &tensor.SimpleTensor{}
	cpuInput.Init(inputV, input.Size())
	cpuOut := l.cpuLayer.Forward(cpuInput).Vector()
	gpuOut := output.Vector()

	for i := 0; i < len(cpuOut); i++ {
		if cpuOut[i] != gpuOut[i] {
			log.Panicf("Mismatch at %d, expected %f, but get %f. Input is %f",
				i, cpuOut[i], gpuOut[i], inputV[i])
		}
	}

	log.Printf("ReLU forward verification passed!")
}

func (l *ReluLayer) saveInput(input *Tensor) {
	if l.forwardInput != 0 {
		l.GPUDriver.FreeMemory(l.GPUCtx, l.forwardInput)
	}

	numElement := input.Size()[0] * input.Size()[1]

	l.forwardInput = l.GPUDriver.AllocateMemory(l.GPUCtx,
		uint64(numElement*4))

	temp := make([]float32, numElement)
	l.GPUDriver.MemCopyD2H(l.GPUCtx, temp, input.ptr)
	l.GPUDriver.MemCopyH2D(l.GPUCtx, l.forwardInput, temp)
}

// Backward performs the backward propagation algorithm.
func (l *ReluLayer) Backward(inputT tensor.Tensor) tensor.Tensor {
	input := inputT.(*Tensor)
	size := input.Size()
	output := &Tensor{
		driver: l.GPUDriver,
		ctx:    l.GPUCtx,
		size:   size,
		ptr: l.GPUDriver.AllocateMemory(
			l.GPUCtx, uint64(size[0]*size[1]*4)),
	}

	kernArg := BackwardKernelArgs{
		Count:     int32(size[0] * size[1]),
		ForwardIn: l.forwardInput,
		In:        input.ptr,
		Out:       output.ptr,
	}

	l.GPUDriver.LaunchKernel(l.GPUCtx, l.backwardKernel,
		[3]uint32{uint32(size[0] * size[1]), 1, 1},
		[3]uint16{64, 1, 1},
		&kernArg)

	l.verifyBackwardPass(input, output)

	return output
}

func (l *ReluLayer) verifyBackwardPass(input, output *Tensor) {
	if !l.verifyBackward {
		return
	}

	inputV := input.Vector()
	cpuInput := &tensor.SimpleTensor{}
	cpuInput.Init(inputV, input.Size())
	cpuOut := l.cpuLayer.Backward(cpuInput).Vector()
	gpuOut := output.Vector()

	for i := 0; i < len(cpuOut); i++ {
		if cpuOut[i] != gpuOut[i] {
			log.Panicf("Mismatch at %d, expected %f, but get %f. Input is %f",
				i, cpuOut[i], gpuOut[i], inputV[i])
		}
	}

	log.Printf("ReLU backward verification passed!")
}

// Parameters returns all the parameters of the layers. ReLU layers do not have
// any parameters.
func (l ReluLayer) Parameters() tensor.Vector {
	return nil
}

// Gradients returns the gradients calculated from the last back propagation
// process. ReLU layers do not have any gradients.
func (l ReluLayer) Gradients() tensor.Vector {
	return nil
}
