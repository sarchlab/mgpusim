package optimization

import (
	"fmt"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/gputensor"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/layers"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
)

type State struct {
	Kind                           string
	LearningRate, Smooth1, Smooth2 float64
	History, HistoryV              []gputensor.TensorState
}

func Snapshot(a Alg, ls []layers.Layer) (State, error) {
	s := State{History: make([]gputensor.TensorState, len(ls)), HistoryV: make([]gputensor.TensorState, len(ls))}
	var h, v map[Layer]tensor.Tensor
	switch a := a.(type) {
	case *Adam:
		s.Kind = "adam"
		s.LearningRate = a.LearningRate
		s.Smooth1 = a.SmoothFactor1
		s.Smooth2 = a.SmoothFactor2
		h = a.historyS
		v = a.historyV
	case *RMSProp:
		s.Kind = "rmsprop"
		s.LearningRate = a.LearningRate
		s.Smooth1 = a.SmoothFactor
		h = a.history
	case *Momentum:
		s.Kind = "momentum"
		s.LearningRate = a.LearningRate
		s.Smooth1 = a.SmoothFactor
		h = a.history
	case *SGD:
		s.Kind = "sgd"
		s.LearningRate = a.LearningRate
	default:
		return State{}, fmt.Errorf("unsupported optimizer %T", a)
	}
	for i, l := range ls {
		s.History[i] = gputensor.Snapshot(h[l])
		s.HistoryV[i] = gputensor.Snapshot(v[l])
	}
	return s, nil
}
func Restore(o *gputensor.GPUOperator, ls []layers.Layer, s State) (Alg, error) {
	h, v := make(map[Layer]tensor.Tensor), make(map[Layer]tensor.Tensor)
	for i, l := range ls {
		if s.History[i].Present {
			h[l] = o.RestoreTensor(s.History[i])
		}
		if s.HistoryV[i].Present {
			v[l] = o.RestoreTensor(s.HistoryV[i])
		}
	}
	switch s.Kind {
	case "adam":
		return &Adam{
			to:            o,
			LearningRate:  s.LearningRate,
			SmoothFactor1: s.Smooth1,
			SmoothFactor2: s.Smooth2,
			historyS:      h,
			historyV:      v,
		}, nil
	case "rmsprop":
		return &RMSProp{to: o, LearningRate: s.LearningRate, SmoothFactor: s.Smooth1, history: h}, nil
	case "momentum":
		return &Momentum{to: o, LearningRate: s.LearningRate, SmoothFactor: s.Smooth1, history: h}, nil
	case "sgd":
		return &SGD{to: o, LearningRate: s.LearningRate}, nil
	default:
		return nil, fmt.Errorf("unknown optimizer %q", s.Kind)
	}
}
