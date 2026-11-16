package layers

import (
	"fmt"
	"math/rand"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
)

func randomFloat64(to tensor.Operator) float64 {
	if r, ok := to.(tensor.RandomSource); ok {
		return r.RandomFloat64()
	}
	return rand.Float64()
}

// State holds the concrete state used by a layer, never a snapshot mirror.
type State struct {
	Kind                string
	FullyConnectedLayer *FullyConnectedLayerState
	Conv2D              *Conv2DState
	AvgPoolingLayer     *AvgPoolingLayerState
	MaxPoolingLayer     *MaxPoolingLayerState
	ReluLayer           *ReluLayerState
}

// StateOf binds a newly constructed layer into its owner's saved graph.
func StateOf(l Layer) (State, error) {
	switch l := l.(type) {
	case *FullyConnectedLayer:
		return State{Kind: "FullyConnectedLayer", FullyConnectedLayer: l.FullyConnectedLayerState}, nil
	case *Conv2D:
		return State{Kind: "Conv2D", Conv2D: l.Conv2DState}, nil
	case *AvgPoolingLayer:
		return State{Kind: "AvgPoolingLayer", AvgPoolingLayer: l.AvgPoolingLayerState}, nil
	case *MaxPoolingLayer:
		return State{Kind: "MaxPoolingLayer", MaxPoolingLayer: l.MaxPoolingLayerState}, nil
	case *ReluLayer:
		return State{Kind: "ReluLayer", ReluLayer: l.ReluLayerState}, nil
	default:
		return State{}, fmt.Errorf("unsupported layer %T", l)
	}
}

// Bind reconnects interfaces to the actual saved layer state without allocation.
func Bind(to tensor.Operator, s State) (Layer, error) {
	refs := tensor.NewReferences(to)
	switch s.Kind {
	case "FullyConnectedLayer":
		if s.FullyConnectedLayer == nil {
			return nil, fmt.Errorf("missing FullyConnectedLayer state")
		}
		return &FullyConnectedLayer{FullyConnectedLayerState: s.FullyConnectedLayer, to: to, refs: refs}, nil
	case "Conv2D":
		if s.Conv2D == nil {
			return nil, fmt.Errorf("missing Conv2D state")
		}
		return &Conv2D{Conv2DState: s.Conv2D, to: to, refs: refs}, nil
	case "AvgPoolingLayer":
		if s.AvgPoolingLayer == nil {
			return nil, fmt.Errorf("missing AvgPoolingLayer state")
		}
		return &AvgPoolingLayer{AvgPoolingLayerState: s.AvgPoolingLayer, to: to, refs: refs}, nil
	case "MaxPoolingLayer":
		if s.MaxPoolingLayer == nil {
			return nil, fmt.Errorf("missing MaxPoolingLayer state")
		}
		return &MaxPoolingLayer{MaxPoolingLayerState: s.MaxPoolingLayer, to: to, refs: refs}, nil
	case "ReluLayer":
		if s.ReluLayer == nil {
			return nil, fmt.Errorf("missing ReluLayer state")
		}
		return &ReluLayer{ReluLayerState: s.ReluLayer, to: to, refs: refs}, nil
	default:
		return nil, fmt.Errorf("unknown layer %q", s.Kind)
	}
}
func ReleaseForward(l Layer, o *gputensor.GPUOperator) {
	release := func(id *tensor.ID) {
		if t := o.Resolve(*id); t != nil {
			o.Free(t)
		}
		*id = 0
	}
	switch l := l.(type) {
	case *FullyConnectedLayer:
		release(&l.ForwardInputID)
	case *Conv2D:
		release(&l.ForwardInputID)
	case *ReluLayer:
		release(&l.ForwardInputID)
	case *AvgPoolingLayer:
		release(&l.ForwardInID)
	case *MaxPoolingLayer:
		release(&l.ForwardInID)
		release(&l.ForwardMaskID)
	}
}
