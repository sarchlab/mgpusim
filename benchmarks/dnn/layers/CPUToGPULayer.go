package layers

import (
	"gitlab.com/akita/dnn/tensor"
	"gitlab.com/akita/mgpusim/driver"
)

// CPUToGPULayer is a special layer that receives CPU tensor as input and
// outputs GPU tensor.
type CPUToGPULayer struct {
	GPUDriver *driver.Driver
	GPUCtx    *driver.Context
}

// Randomize creates the parameters randomly. Since there is no parameter in a
// CPUToGPULayer, this function does nothing.
func (l CPUToGPULayer) Randomize() {
	// nothing
}

// Forward performs the forward propagation algorithm. This function allocates
// the GPU memory for the output.
func (l CPUToGPULayer) Forward(inputT tensor.Tensor) tensor.Tensor {
	input := inputT.(*tensor.SimpleTensor)

	gpuMem := l.GPUDriver.AllocateMemory(
		l.GPUCtx,
		uint64(len(input.Vector())*4),
	)
	output := &Tensor{
		size:   input.Size(),
		ptr:    gpuMem,
		driver: l.GPUDriver,
		ctx:    l.GPUCtx,
	}

	tempData := make([]float32, len(input.Vector()))
	for i, val := range input.Vector() {
		tempData[i] = float32(val)
	}
	l.GPUDriver.MemCopyH2D(l.GPUCtx, gpuMem, tempData)

	return output
}

// Backward allocates the backward propagation algorithm.
func (l CPUToGPULayer) Backward(inputT tensor.Tensor) tensor.Tensor {
	input := inputT.(*Tensor)
	output := &tensor.SimpleTensor{}

	numElement := 1
	for _, s := range input.Size() {
		numElement *= s
	}

	output.Init(make([]float64, numElement), input.Size())

	tempData := make([]float32, numElement)
	l.GPUDriver.MemCopyD2H(l.GPUCtx, tempData, input.ptr)

	for i, value := range tempData {
		output.Vector()[i] = float64(value)
	}

	return output
}

// Parameters returns the layer parameters. CPUToGPULayers do not have any
// parameter.
func (l CPUToGPULayer) Parameters() tensor.Vector {
	return nil
}

// Gradients returns the gradients from the previous backward propagation.
func (l CPUToGPULayer) Gradients() tensor.Vector {
	return nil
}
