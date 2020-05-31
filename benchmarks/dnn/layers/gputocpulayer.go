package layers

import (
	"gitlab.com/akita/dnn/tensor"
	"gitlab.com/akita/mgpusim/driver"
)

// GPUToCPULayer is a special layer that receives CPU tensor as input and
// outputs GPU tensor.
type GPUToCPULayer struct {
	GPUDriver *driver.Driver
	GPUCtx    *driver.Context
}

// Randomize initializes the layer parameter randomly.
func (l GPUToCPULayer) Randomize() {
	// nothing
}

// Forward performs the forward propagation operation.
func (l GPUToCPULayer) Forward(inputT tensor.Tensor) tensor.Tensor {
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

// Backward performance backward propagation operation.
func (l GPUToCPULayer) Backward(inputT tensor.Tensor) tensor.Tensor {
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

// Parameters returns the layer parameters. GPUToCPULayers do not have
// parameter.
func (l GPUToCPULayer) Parameters() tensor.Vector {
	return nil
}

// Gradients returns the gradients calculated by the last backward propagation
// operation.
func (l GPUToCPULayer) Gradients() tensor.Vector {
	return nil
}
