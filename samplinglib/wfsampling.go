package samplinglib

import (
	"flag"
	"log"
	"time"

	"github.com/sarchlab/akita/v3/sim"
)

// SampledRunnerFlag is used to enable wf sampling
var SampledRunnerFlag = flag.Bool("wf-sampling", false, "enable wavefront-level sampled simulation.")

// SampledRunnerThresholdFlag is used to set the threshold of the sampling
var SampledRunnerThresholdFlag = flag.Float64("sampled-threshold", 0.03, "the threshold of the sampled execution to enable sampling simulation.")

// SampledRunnerGranularyFlag is used to set the granulary of the sampling
var SampledRunnerGranularyFlag = flag.Int("sampled-granulary", 1024, "the granulary of the sampled execution to collect and analyze data.")

// SampledEngine is used to detect if the wavefront sampling is stable or not.
type SampledEngine struct {
	predTime             sim.VTimeInSec
	enableSampled        bool
	disableEngine        bool
	Simtime              float64 `json:"simtime"`
	Walltime             float64 `json:"walltime"`
	FullSimWalltime      float64 `json:"fullsimwalltime"`
	FullSimWalltimeStart time.Time
	dataidx              uint64
	stableEngine         *StableEngine
	shortStableEngine    *StableEngine
	predTimeSum          sim.VTimeInSec
	predTimeNum          uint64
	granulary            int
}

// Reset all status
func (sampled_engine *SampledEngine) Reset() {
	sampled_engine.FullSimWalltimeStart = time.Now()
	sampled_engine.stableEngine.Reset()
	sampled_engine.shortStableEngine.Reset()
	sampled_engine.predTime = 0
	sampled_engine.predTimeNum = 0
	sampled_engine.predTimeSum = 0
	sampled_engine.dataidx = 0
	sampled_engine.enableSampled = false
}

// NewSampledEngine is used to new a sampled engine for wavefront sampling
func NewSampledEngine(granulary int, boundary float64, control bool) *SampledEngine {
	stableEngine := &StableEngine{
		granulary: granulary,
		boundary:  boundary,
	}
	shortStableEngine := &StableEngine{
		granulary: granulary / 2,
		boundary:  boundary,
	}
	ret := &SampledEngine{
		stableEngine:      stableEngine,
		shortStableEngine: shortStableEngine,
		granulary:         granulary / 2,
	}
	ret.Reset()
	if control {
		ret.disableEngine = false
	}
	return ret
}

// Sampledengine is used to monitor wavefront sampling
var Sampledengine *SampledEngine

// InitSampledEngine is used to initial all status and data structure
func InitSampledEngine() {
	Sampledengine = NewSampledEngine(*SampledRunnerGranularyFlag, *SampledRunnerThresholdFlag, false)
	if *SampledRunnerFlag {
		Sampledengine.Enable()
	} else {
		Sampledengine.Disabled()
	}
}

// Disabled the sampling engine
func (sampled_engine *SampledEngine) Disabled() {
	sampled_engine.disableEngine = true
}

// Enable the sampling engine
func (sampled_engine *SampledEngine) Enable() {
	sampled_engine.disableEngine = false
}

// IfDisable the sampling engine
func (sampled_engine *SampledEngine) IfDisable() bool {
	return sampled_engine.disableEngine
}

// Collect the runtime information
func (sampled_engine *SampledEngine) Collect(issuetime sim.VTimeInSec, finishtime sim.VTimeInSec) {
	if sampled_engine.enableSampled || sampled_engine.disableEngine { //we do not need to collect data if sampling is enabled
		return
	}
	sampled_engine.dataidx++
	if sampled_engine.dataidx < 1024 { // discard the first 1024 data
		return
	}
	sampled_engine.stableEngine.Collect(issuetime, finishtime)
	sampled_engine.shortStableEngine.Collect(issuetime, finishtime)
	stableEngine := sampled_engine.stableEngine
	shortStableEngine := sampled_engine.shortStableEngine
	if stableEngine.enableSampled {
		longTime := stableEngine.predTime
		shortTime := shortStableEngine.predTime
		sampled_engine.predTime = shortStableEngine.predTime
		diff := float64((longTime - shortTime) / (longTime + shortTime))
		diffBoundary := *SampledRunnerThresholdFlag
		if diff <= diffBoundary && diff >= -diffBoundary {
			sampled_engine.enableSampled = true
			sampled_engine.predTime = shortTime
			sampled_engine.predTimeSum = shortTime * sim.VTimeInSec(sampled_engine.granulary)
			sampled_engine.predTimeNum = uint64(sampled_engine.granulary)
		}
	} else if shortStableEngine.enableSampled {
		sampled_engine.predTime = stableEngine.predTime
	}
	if sampled_engine.enableSampled {
		log.Printf("Warp Sampling is enabled")
	}
}

// Predict the execution time of the next wavefronts
func (sampled_engine *SampledEngine) Predict() (sim.VTimeInSec, bool) {
	return sampled_engine.predTime, sampled_engine.enableSampled
}
