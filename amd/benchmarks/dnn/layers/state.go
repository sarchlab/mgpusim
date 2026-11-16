package layers

import "github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"

type AvgPoolingLayerState struct {
	KernelSize        []int
	Padding           []int
	Stride            []int
	ForwardInID       tensor.ID
	ForwardOutputSize []int
}
type Conv2DState struct {
	LayerIndex            int
	InputSize, OutputSize []int
	KernelSize            []int
	Stride                []int
	Padding               []int
	ParametersID          tensor.ID
	WeightsID             tensor.ID
	BiasID                tensor.ID
	GradientsID           tensor.ID
	WeightGradientsID     tensor.ID
	BiasGradientsID       tensor.ID
	ForwardInputID        tensor.ID
}
type FullyConnectedLayerState struct {
	LayerIndex        int
	InputSize         int
	OutputSize        int
	ParametersID      tensor.ID
	WeightsID         tensor.ID
	BiasID            tensor.ID
	GradientsID       tensor.ID
	WeightGradientsID tensor.ID
	BiasGradientsID   tensor.ID
	ForwardInputID    tensor.ID
}
type MaxPoolingLayerState struct {
	KernelSize        []int
	Padding           []int
	Stride            []int
	ForwardInID       tensor.ID
	ForwardMaskID     tensor.ID
	ForwardOutputSize []int
}
type ReluLayerState struct {
	ForwardInputID tensor.ID
}
