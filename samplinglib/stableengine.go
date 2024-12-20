// Package samplinglib provides tools for performing sampling simulation
package samplinglib

import (
	"github.com/sarchlab/akita/v3/sim"
)

// WFFeature is used for recording the runtime info
type WFFeature struct {
	Issuetime  sim.VTimeInSec
	Finishtime sim.VTimeInSec
}

// StableEngine is used to detect if the feature detecting is stable or not
type StableEngine struct {
	issuetimeSum       sim.VTimeInSec
	finishtimeSum      sim.VTimeInSec
	intervaltimeSum    sim.VTimeInSec
	mixSum             sim.VTimeInSec
	issuetimeSquareSum sim.VTimeInSec
	rate               float64
	granulary          int
	Wffeatures         []WFFeature
	boundary           float64
	enableSampled      bool
	predTime           sim.VTimeInSec
}

// Analysis the data
func (stable_engine *StableEngine) Analysis() {
	rateBottom := sim.VTimeInSec(stable_engine.granulary)*stable_engine.issuetimeSquareSum - stable_engine.issuetimeSum*stable_engine.issuetimeSum
	rateTop := sim.VTimeInSec(stable_engine.granulary)*stable_engine.mixSum - stable_engine.issuetimeSum*stable_engine.finishtimeSum
	rate := float64(rateTop / rateBottom)
	stable_engine.rate = rate
	boundary := stable_engine.boundary
	stable_engine.predTime = stable_engine.intervaltimeSum / sim.VTimeInSec(stable_engine.granulary)
	if rate >= (1-boundary) && rate <= (1+boundary) {
		stable_engine.enableSampled = true
	} else {
		stable_engine.enableSampled = false
	}
}

// Reset all information
func (stable_engine *StableEngine) Reset() {
	stable_engine.Wffeatures = nil
	stable_engine.issuetimeSum = 0
	stable_engine.finishtimeSum = 0
	stable_engine.intervaltimeSum = 0
	stable_engine.mixSum = 0
	stable_engine.issuetimeSquareSum = 0
	stable_engine.predTime = 0
	stable_engine.enableSampled = false
}

// Collect data
func (stable_engine *StableEngine) Collect(issuetime, finishtime sim.VTimeInSec) {
	wffeature := WFFeature{
		Issuetime:  issuetime,
		Finishtime: finishtime,
	}

	stable_engine.Wffeatures = append(stable_engine.Wffeatures, wffeature)
	stable_engine.issuetimeSum += issuetime
	stable_engine.finishtimeSum += finishtime
	stable_engine.mixSum += finishtime * issuetime
	stable_engine.issuetimeSquareSum += issuetime * issuetime
	stable_engine.intervaltimeSum += (finishtime - issuetime)
	if len(stable_engine.Wffeatures) == stable_engine.granulary {
		stable_engine.Analysis()
		///delete old data
		wffeature2 := stable_engine.Wffeatures[0]
		stable_engine.Wffeatures = stable_engine.Wffeatures[1:]
		issuetime = wffeature2.Issuetime
		finishtime = wffeature2.Finishtime
		stable_engine.issuetimeSum -= issuetime
		stable_engine.finishtimeSum -= finishtime
		stable_engine.mixSum -= finishtime * issuetime
		stable_engine.issuetimeSquareSum -= issuetime * issuetime
		stable_engine.intervaltimeSum -= (finishtime - issuetime)
	}
}
