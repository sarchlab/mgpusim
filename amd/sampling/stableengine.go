// Package sampling provides tools for performing sampling simulation
package sampling

// WFFeature is used for recording the runtime info. Times are in seconds, as
// float64, because the stability analysis multiplies times together and the
// products would overflow integer picoseconds.
type WFFeature struct {
	IssueTime  float64
	FinishTime float64
}

// StableEngine is used to detect if the feature detecting is stable or not
type StableEngine struct {
	issueTimeSum       float64
	finishTimeSum      float64
	intervalTimeSum    float64
	mixSum             float64
	issueTimeSquareSum float64
	rate               float64
	granularity        int
	WfFeatures         []WFFeature
	boundary           float64
	enableSampled      bool
	predTime           float64
}

// Analysis the data
func (se *StableEngine) Analysis() {
	rateBottom := float64(se.granularity)*se.issueTimeSquareSum -
		se.issueTimeSum*se.issueTimeSum
	rateTop := float64(se.granularity)*se.mixSum -
		se.issueTimeSum*se.finishTimeSum
	rate := rateTop / rateBottom

	se.rate = rate
	boundary := se.boundary
	se.predTime = se.intervalTimeSum / float64(se.granularity)

	if rate >= (1-boundary) && rate <= (1+boundary) {
		se.enableSampled = true
	} else {
		se.enableSampled = false
	}
}

// Reset all information
func (se *StableEngine) Reset() {
	se.WfFeatures = nil
	se.issueTimeSum = 0
	se.finishTimeSum = 0
	se.intervalTimeSum = 0
	se.mixSum = 0
	se.issueTimeSquareSum = 0
	se.predTime = 0
	se.enableSampled = false
}

// Collect data
func (se *StableEngine) Collect(issueTime, finishTime float64) {
	wffeature := WFFeature{
		IssueTime:  issueTime,
		FinishTime: finishTime,
	}

	se.WfFeatures = append(se.WfFeatures, wffeature)
	se.issueTimeSum += issueTime
	se.finishTimeSum += finishTime
	se.mixSum += finishTime * issueTime
	se.issueTimeSquareSum += issueTime * issueTime

	se.intervalTimeSum += (finishTime - issueTime)
	if len(se.WfFeatures) == se.granularity {
		se.Analysis()
		///delete old data
		wfFeature2 := se.WfFeatures[0]
		se.WfFeatures = se.WfFeatures[1:]
		issueTime = wfFeature2.IssueTime
		finishTime = wfFeature2.FinishTime
		se.issueTimeSum -= issueTime
		se.finishTimeSum -= finishTime
		se.mixSum -= finishTime * issueTime
		se.issueTimeSquareSum -= issueTime * issueTime
		se.intervalTimeSum -= (finishTime - issueTime)
	}
}
