package samplinglib

import (
	"flag"
	"log"
	"time"

	"github.com/sarchlab/akita/v3/sim"
)

var SampledRunnerFlag = flag.Bool("wf-sampling", false, "enable wavefront-level sampled simulation.")
var SampledRunnerThresholdFlag = flag.Float64("sampled-threshold", 0.03, "the threshold of the sampled execution to enable sampling simulation.")
var SampledRunnerGranularyFlag = flag.Int("sampled-granulary", 1024, "the granulary of the sampled execution to collect and analyze data.")

type SampledEngine struct {
	predTime             sim.VTimeInSec
	enableSampled        bool
	disableEngine        bool
	Simtime              float64 `json:"simtime"`
	Walltime             float64 `json:"walltime"`
	FullSimWalltime      float64 `json:"fullsimwalltime"`
	FullSimWalltimeStart time.Time
	dataidx              uint64
	stable_engine        *StableEngine
	short_stable_engine  *StableEngine
	predTimeSum          sim.VTimeInSec
	predTimeNum          uint64
	granulary            int
}

func (sampled_engine *SampledEngine) Reset() {
	sampled_engine.FullSimWalltimeStart = time.Now()
	sampled_engine.stable_engine.Reset()
	sampled_engine.short_stable_engine.Reset()
	sampled_engine.predTime = 0
	sampled_engine.predTimeNum = 0
	sampled_engine.predTimeSum = 0
	sampled_engine.dataidx = 0
	sampled_engine.enableSampled = false
}

// const granulary = 512
func NewSampledEngine(granulary int, boundary float64, control bool) *SampledEngine {

	stable_engine := &StableEngine{
		granulary: granulary,
		boundary:  boundary,
	}
	short_stable_engine := &StableEngine{
		granulary: granulary / 2,
		boundary:  boundary,
	}
	ret := &SampledEngine{
		stable_engine:       stable_engine,
		short_stable_engine: short_stable_engine,
		granulary:           granulary / 2,
	}
	ret.Reset()
	if control {
		ret.disableEngine = false
	}
	return ret
}

var Sampledengine *SampledEngine

func InitSampledEngine() {
	Sampledengine = NewSampledEngine(*SampledRunnerGranularyFlag, *SampledRunnerThresholdFlag, false)
	if *SampledRunnerFlag {
		Sampledengine.Enable()
	} else {
		Sampledengine.Disabled()
	}
}

func (sampled_engine *SampledEngine) Disabled() {
	sampled_engine.disableEngine = true
}
func (sampled_engine *SampledEngine) Enable() {
	sampled_engine.disableEngine = false
}
func (sampled_engine *SampledEngine) IfDisable() bool {
	return sampled_engine.disableEngine
}
func (sampled_engine *SampledEngine) Collect(issuetime sim.VTimeInSec, finishtime sim.VTimeInSec) {
	if sampled_engine.enableSampled || sampled_engine.disableEngine { //we do not need to collect data if sampling is enabled
		return
	}

	sampled_engine.dataidx++
	if sampled_engine.dataidx < 1024 { // discard the first 1024 data
		return
	}

	sampled_engine.stable_engine.Collect(issuetime, finishtime)
	sampled_engine.short_stable_engine.Collect(issuetime, finishtime)
	stable_engine := sampled_engine.stable_engine
	short_stable_engine := sampled_engine.short_stable_engine

	if stable_engine.enableSampled {

		long_time := stable_engine.predTime
		short_time := short_stable_engine.predTime
		sampled_engine.predTime = short_stable_engine.predTime
		diff := float64((long_time - short_time) / (long_time + short_time))

		diff_boundary := *SampledRunnerThresholdFlag
		if diff <= diff_boundary && diff >= -diff_boundary {
			sampled_engine.enableSampled = true
			sampled_engine.predTime = short_time
			sampled_engine.predTimeSum = short_time * sim.VTimeInSec(sampled_engine.granulary)
			sampled_engine.predTimeNum = uint64(sampled_engine.granulary)
		}

	} else if short_stable_engine.enableSampled {
		sampled_engine.predTime = stable_engine.predTime
	}
	if sampled_engine.enableSampled {
		log.Printf("Warp Sampling is enabled")
	}
}

func (sampled_engine *SampledEngine) Predict() (sim.VTimeInSec, bool) {
	return sampled_engine.predTime, sampled_engine.enableSampled
}
