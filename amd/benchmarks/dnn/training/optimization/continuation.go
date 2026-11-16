package optimization

import (
	"fmt"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
)

// State is the mutable optimizer state used directly during updates.
type State struct {
	Kind                                                     string
	LearningRate, SmoothFactor, SmoothFactor1, SmoothFactor2 float64
	History, HistoryV                                        map[tensor.ID]tensor.ID
}

func StateOf(a Alg) (*State, error) {
	switch a := a.(type) {
	case *Adam:
		return a.State, nil
	case *RMSProp:
		return a.State, nil
	case *Momentum:
		return a.State, nil
	case *SGD:
		return a.State, nil
	default:
		return nil, fmt.Errorf("unsupported optimizer %T", a)
	}
}
func Bind(to tensor.Operator, s *State) (Alg, error) {
	if s == nil {
		return nil, fmt.Errorf("missing optimizer state")
	}
	refs := tensor.NewReferences(to)
	switch s.Kind {
	case "adam":
		return &Adam{State: s, to: to, refs: refs}, nil
	case "rmsprop":
		return &RMSProp{State: s, to: to, refs: refs}, nil
	case "momentum":
		return &Momentum{State: s, to: to, refs: refs}, nil
	case "sgd":
		return &SGD{State: s, to: to, refs: refs}, nil
	default:
		return nil, fmt.Errorf("unknown optimizer %q", s.Kind)
	}
}
