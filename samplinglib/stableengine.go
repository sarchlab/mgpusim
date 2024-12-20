package samplinglib

import (
	"github.com/sarchlab/akita/v3/sim"
)

type WFFeature struct {
	Issuetime  sim.VTimeInSec
	Finishtime sim.VTimeInSec
}

type StableEngine struct {
	issuetime_sum        sim.VTimeInSec
	finishtime_sum       sim.VTimeInSec
	intervaltime_sum     sim.VTimeInSec
	mix_sum              sim.VTimeInSec
	issuetime_square_sum sim.VTimeInSec
	rate                 float64
	granulary            int
	Wffeatures           []WFFeature
	boundary             float64
	enableSampled        bool
	predTime             sim.VTimeInSec
}

func (stable_engine *StableEngine) Analysis() {

	rate_bottom := sim.VTimeInSec(stable_engine.granulary)*stable_engine.issuetime_square_sum - stable_engine.issuetime_sum*stable_engine.issuetime_sum
	rate_top := sim.VTimeInSec(stable_engine.granulary)*stable_engine.mix_sum - stable_engine.issuetime_sum*stable_engine.finishtime_sum
	rate := float64(rate_top / rate_bottom)
	stable_engine.rate = rate
	boundary := stable_engine.boundary
	stable_engine.predTime = stable_engine.intervaltime_sum / sim.VTimeInSec(stable_engine.granulary)
	if rate >= (1-boundary) && rate <= (1+boundary) {
		stable_engine.enableSampled = true
	} else {
		stable_engine.enableSampled = false
	}
}
func (stable_engine *StableEngine) Reset() {
	stable_engine.Wffeatures = nil
	stable_engine.issuetime_sum = 0
	stable_engine.finishtime_sum = 0
	stable_engine.intervaltime_sum = 0
	stable_engine.mix_sum = 0
	stable_engine.issuetime_square_sum = 0
	stable_engine.predTime = 0
	stable_engine.enableSampled = false
}
func (stable_engine *StableEngine) Collect(issuetime, finishtime sim.VTimeInSec) {
	wffeature := WFFeature{
		Issuetime:  issuetime,
		Finishtime: finishtime,
	}

	stable_engine.Wffeatures = append(stable_engine.Wffeatures, wffeature)
	stable_engine.issuetime_sum += issuetime
	stable_engine.finishtime_sum += finishtime
	stable_engine.mix_sum += finishtime * issuetime
	stable_engine.issuetime_square_sum += issuetime * issuetime
	stable_engine.intervaltime_sum += (finishtime - issuetime)
	if len(stable_engine.Wffeatures) == stable_engine.granulary {
		stable_engine.Analysis()
		///delete old data
		wffeature2 := stable_engine.Wffeatures[0]
		stable_engine.Wffeatures = stable_engine.Wffeatures[1:]
		issuetime = wffeature2.Issuetime
		finishtime = wffeature2.Finishtime
		stable_engine.issuetime_sum -= issuetime
		stable_engine.finishtime_sum -= finishtime
		stable_engine.mix_sum -= finishtime * issuetime
		stable_engine.issuetime_square_sum -= issuetime * issuetime
		stable_engine.intervaltime_sum -= (finishtime - issuetime)
	}
}
