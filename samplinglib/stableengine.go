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
func (se *StableEngine) Analysis() {
	rateBottom := sim.VTimeInSec(se.granulary)*se.issuetimeSquareSum - se.issuetimeSum*se.issuetimeSum
	rateTop := sim.VTimeInSec(se.granulary)*se.mixSum - se.issuetimeSum*se.finishtimeSum
	rate := float64(rateTop / rateBottom)
	se.rate = rate
	boundary := se.boundary
	se.predTime = se.intervaltimeSum / sim.VTimeInSec(se.granulary)
	if rate >= (1-boundary) && rate <= (1+boundary) {
		se.enableSampled = true
	} else {
		se.enableSampled = false
	}
}

// Reset all information
func (se *StableEngine) Reset() {
	se.Wffeatures = nil
	se.issuetimeSum = 0
	se.finishtimeSum = 0
	se.intervaltimeSum = 0
	se.mixSum = 0
	se.issuetimeSquareSum = 0
	se.predTime = 0
	se.enableSampled = false
}

// Collect data
func (se *StableEngine) Collect(issuetime, finishtime sim.VTimeInSec) {
	wffeature := WFFeature{
		Issuetime:  issuetime,
		Finishtime: finishtime,
	}

	se.Wffeatures = append(se.Wffeatures, wffeature)
	se.issuetimeSum += issuetime
	se.finishtimeSum += finishtime
	se.mixSum += finishtime * issuetime
	se.issuetimeSquareSum += issuetime * issuetime
	se.intervaltimeSum += (finishtime - issuetime)
	if len(se.Wffeatures) == se.granulary {
		se.Analysis()
		///delete old data
		wffeature2 := se.Wffeatures[0]
		se.Wffeatures = se.Wffeatures[1:]
		issuetime = wffeature2.Issuetime
		finishtime = wffeature2.Finishtime
		se.issuetimeSum -= issuetime
		se.finishtimeSum -= finishtime
		se.mixSum -= finishtime * issuetime
		se.issuetimeSquareSum -= issuetime * issuetime
		se.intervaltimeSum -= (finishtime - issuetime)
	}
}
