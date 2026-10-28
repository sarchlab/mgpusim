package sampling

import (
	"io"
	"time"

	"github.com/sarchlab/akita/v5/sim/modeling"
)

type stableState struct {
	IssueSum, FinishSum, IntervalSum, MixSum, IssueSquareSum float64
	Features                                                 []WFFeature
	Enabled                                                  bool
	Prediction                                               float64
}

func (se *StableEngine) snapshot() stableState {
	return stableState{
		se.issueTimeSum,
		se.finishTimeSum,
		se.intervalTimeSum,
		se.mixSum,
		se.issueTimeSquareSum,
		se.WfFeatures,
		se.enableSampled,
		se.predTime,
	}
}
func (se *StableEngine) restore(s stableState) {
	se.issueTimeSum, se.finishTimeSum, se.intervalTimeSum = s.IssueSum, s.FinishSum, s.IntervalSum
	se.mixSum, se.issueTimeSquareSum = s.MixSum, s.IssueSquareSum
	se.WfFeatures, se.enableSampled, se.predTime = s.Features, s.Enabled, s.Prediction
}

type sampleState struct {
	Prediction, PredictionSum float64
	Enabled, Disabled         bool
	Index, PredictionCount    uint64
	Long, Short               stableState
}

// SaveCheckpoint preserves the prediction window and decision state. Host wall
// time measurements are deliberately restarted when a new process restores.
func (se *SampledEngine) SaveCheckpoint(w io.Writer) error {
	se.mu.Lock()
	defer se.mu.Unlock()
	state := sampleState{
		se.predTime,
		se.predTimeSum,
		se.enableSampled,
		se.disableEngine,
		se.dataIdx,
		se.predTimeNum,
		se.stableEngine.snapshot(),
		se.shortStableEngine.snapshot(),
	}
	return modeling.WriteCheckpoint(w, state, nil)
}
func (se *SampledEngine) LoadCheckpoint(r io.Reader) error {
	se.mu.Lock()
	defer se.mu.Unlock()
	var state sampleState
	if err := modeling.ReadCheckpoint(r, &state, nil); err != nil {
		return err
	}
	se.predTime, se.predTimeSum = state.Prediction, state.PredictionSum
	se.enableSampled, se.disableEngine = state.Enabled, state.Disabled
	se.dataIdx, se.predTimeNum = state.Index, state.PredictionCount
	se.stableEngine.restore(state.Long)
	se.shortStableEngine.restore(state.Short)
	se.FullSimWallTimeStart = time.Now()
	return nil
}
