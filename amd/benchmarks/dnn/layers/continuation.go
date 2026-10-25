package layers

import (
	"fmt"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
	"math/rand"
)

func randomFloat64(to tensor.Operator) float64 {
	if r, ok := to.(interface{ RandomFloat64() float64 }); ok {
		return r.RandomFloat64()
	}
	return rand.Float64()
}

// State records each layer's parameters, views, and saved forward activation.
type State struct {
	Kind                string
	FullyConnectedLayer FullyConnectedLayerState
	Conv2D              Conv2DState
	AvgPoolingLayer     AvgPoolingLayerState
	MaxPoolingLayer     MaxPoolingLayerState
	ReluLayer           ReluLayerState
}
type FullyConnectedLayerState struct {
	LayerIndex      int
	InputSize       int
	OutputSize      int
	Parameters      gputensor.TensorState
	Weights         gputensor.TensorState
	Bias            gputensor.TensorState
	Gradients       gputensor.TensorState
	WeightGradients gputensor.TensorState
	BiasGradients   gputensor.TensorState
	ForwardInput    gputensor.TensorState
}
type Conv2DState struct {
	LayerIndex      int
	InputSize       []int
	OutputSize      []int
	KernelSize      []int
	Stride          []int
	Padding         []int
	Parameters      gputensor.TensorState
	Weights         gputensor.TensorState
	Bias            gputensor.TensorState
	Gradients       gputensor.TensorState
	WeightGradients gputensor.TensorState
	BiasGradients   gputensor.TensorState
	ForwardInput    gputensor.TensorState
}
type AvgPoolingLayerState struct {
	KernelSize        []int
	Padding           []int
	Stride            []int
	ForwardIn         gputensor.TensorState
	ForwardOutputSize []int
}
type MaxPoolingLayerState struct {
	KernelSize        []int
	Padding           []int
	Stride            []int
	ForwardIn         gputensor.TensorState
	ForwardMask       gputensor.TensorState
	ForwardOutputSize []int
}
type ReluLayerState struct {
	ForwardInput gputensor.TensorState
}

func Snapshot(l Layer) (State, error) {
	switch l := l.(type) {
	case *FullyConnectedLayer:
		return State{Kind: "FullyConnectedLayer", FullyConnectedLayer: FullyConnectedLayerState{
			LayerIndex:      l.layerIndex,
			InputSize:       l.InputSize,
			OutputSize:      l.OutputSize,
			Parameters:      gputensor.Snapshot(l.parameters),
			Weights:         gputensor.Snapshot(l.weights),
			Bias:            gputensor.Snapshot(l.bias),
			Gradients:       gputensor.Snapshot(l.gradients),
			WeightGradients: gputensor.Snapshot(l.weightGradients),
			BiasGradients:   gputensor.Snapshot(l.biasGradients),
			ForwardInput:    gputensor.Snapshot(l.forwardInput),
		}}, nil
	case *Conv2D:
		return State{Kind: "Conv2D", Conv2D: Conv2DState{
			LayerIndex:      l.layerIndex,
			InputSize:       l.inputSize,
			OutputSize:      l.outputSize,
			KernelSize:      l.kernelSize,
			Stride:          l.stride,
			Padding:         l.padding,
			Parameters:      gputensor.Snapshot(l.parameters),
			Weights:         gputensor.Snapshot(l.weights),
			Bias:            gputensor.Snapshot(l.bias),
			Gradients:       gputensor.Snapshot(l.gradients),
			WeightGradients: gputensor.Snapshot(l.weightGradients),
			BiasGradients:   gputensor.Snapshot(l.biasGradients),
			ForwardInput:    gputensor.Snapshot(l.forwardInput),
		}}, nil
	case *AvgPoolingLayer:
		return State{Kind: "AvgPoolingLayer", AvgPoolingLayer: AvgPoolingLayerState{
			KernelSize:        l.kernelSize,
			Padding:           l.padding,
			Stride:            l.stride,
			ForwardIn:         gputensor.Snapshot(l.forwardIn),
			ForwardOutputSize: l.forwardOutputSize,
		}}, nil
	case *MaxPoolingLayer:
		return State{Kind: "MaxPoolingLayer", MaxPoolingLayer: MaxPoolingLayerState{
			KernelSize:        l.kernelSize,
			Padding:           l.padding,
			Stride:            l.stride,
			ForwardIn:         gputensor.Snapshot(l.forwardIn),
			ForwardMask:       gputensor.Snapshot(l.forwardMask),
			ForwardOutputSize: l.forwardOutputSize,
		}}, nil
	case *ReluLayer:
		return State{Kind: "ReluLayer", ReluLayer: ReluLayerState{
			ForwardInput: gputensor.Snapshot(l.forwardInput),
		}}, nil
	default:
		return State{}, fmt.Errorf("unsupported GPU layer %T", l)
	}
}
func Restore(o *gputensor.GPUOperator, s State) (Layer, error) {
	switch s.Kind {
	case "FullyConnectedLayer":
		v := s.FullyConnectedLayer
		return &FullyConnectedLayer{to: o,
			layerIndex:      v.LayerIndex,
			InputSize:       v.InputSize,
			OutputSize:      v.OutputSize,
			parameters:      o.RestoreTensor(v.Parameters),
			weights:         o.RestoreTensor(v.Weights),
			bias:            o.RestoreTensor(v.Bias),
			gradients:       o.RestoreTensor(v.Gradients),
			weightGradients: o.RestoreTensor(v.WeightGradients),
			biasGradients:   o.RestoreTensor(v.BiasGradients),
			forwardInput:    o.RestoreTensor(v.ForwardInput),
		}, nil
	case "Conv2D":
		v := s.Conv2D
		return &Conv2D{to: o,
			layerIndex:      v.LayerIndex,
			inputSize:       v.InputSize,
			outputSize:      v.OutputSize,
			kernelSize:      v.KernelSize,
			stride:          v.Stride,
			padding:         v.Padding,
			parameters:      o.RestoreTensor(v.Parameters),
			weights:         o.RestoreTensor(v.Weights),
			bias:            o.RestoreTensor(v.Bias),
			gradients:       o.RestoreTensor(v.Gradients),
			weightGradients: o.RestoreTensor(v.WeightGradients),
			biasGradients:   o.RestoreTensor(v.BiasGradients),
			forwardInput:    o.RestoreTensor(v.ForwardInput),
		}, nil
	case "AvgPoolingLayer":
		v := s.AvgPoolingLayer
		return &AvgPoolingLayer{to: o,
			kernelSize:        v.KernelSize,
			padding:           v.Padding,
			stride:            v.Stride,
			forwardIn:         o.RestoreTensor(v.ForwardIn),
			forwardOutputSize: v.ForwardOutputSize,
		}, nil
	case "MaxPoolingLayer":
		v := s.MaxPoolingLayer
		return &MaxPoolingLayer{to: o,
			kernelSize:        v.KernelSize,
			padding:           v.Padding,
			stride:            v.Stride,
			forwardIn:         o.RestoreTensor(v.ForwardIn),
			forwardMask:       o.RestoreTensor(v.ForwardMask),
			forwardOutputSize: v.ForwardOutputSize,
		}, nil
	case "ReluLayer":
		v := s.ReluLayer
		return &ReluLayer{to: o,
			forwardInput: o.RestoreTensor(v.ForwardInput),
		}, nil
	default:
		return nil, fmt.Errorf("unknown saved layer %q", s.Kind)
	}
}

// ReleaseForward discards inference-only saved activations after their forward
// commands have been enqueued. GPUOperator defers their physical release.
func ReleaseForward(l Layer, o *gputensor.GPUOperator) {
	release := func(t *tensor.Tensor) {
		if *t != nil {
			o.Free(*t)
			*t = nil
		}
	}
	switch l := l.(type) {
	case *FullyConnectedLayer:
		release(&l.forwardInput)
	case *Conv2D:
		release(&l.forwardInput)
	case *ReluLayer:
		release(&l.forwardInput)
	case *AvgPoolingLayer:
		release(&l.forwardIn)
	case *MaxPoolingLayer:
		release(&l.forwardIn)
		release(&l.forwardMask)
	}
}
