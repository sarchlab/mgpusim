package sampling

import (
	"flag"
	"log"
	"time"

	"github.com/sarchlab/akita/v4/sim"
)

// SampledRunnerFlag is used to enable wf sampling
var SampledRunnerFlag = flag.Bool("wf-sampling", false, "enable wavefront-level sampled simulation.")

// SampledRunnerThresholdFlag is used to set the threshold of the sampling
var SampledRunnerThresholdFlag = flag.Float64("sampled-threshold", 0.03,
	"the threshold of the sampled execution to enable sampling simulation.")

// SampledRunnerGranularyFlag is used to set the granulary of the sampling
var SampledRunnerGranularyFlag = flag.Int("sampled-granulary", 1024,
	"the granulary of the sampled execution to collect and analyze data.")

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
func (se *SampledEngine) Reset() {
	se.FullSimWalltimeStart = time.Now()
	se.stableEngine.Reset()
	se.shortStableEngine.Reset()
	se.predTime = 0
	se.predTimeNum = 0
	se.predTimeSum = 0
	se.dataidx = 0
	se.enableSampled = false
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
func (se *SampledEngine) Disabled() {
	se.disableEngine = true
}

// Enable the sampling engine
func (se *SampledEngine) Enable() {
	se.disableEngine = false
}

// IfDisable the sampling engine
func (se *SampledEngine) IfDisable() bool {
	return se.disableEngine
}

// Collect the runtime information
func (se *SampledEngine) Collect(issuetime sim.VTimeInSec, finishtime sim.VTimeInSec) {
	if se.enableSampled || se.disableEngine { //we do not need to collect data if sampling is enabled
		return
	}
	se.dataidx++
	if se.dataidx < 1024 { // discard the first 1024 data
		return
	}
	se.stableEngine.Collect(issuetime, finishtime)
	se.shortStableEngine.Collect(issuetime, finishtime)
	stableEngine := se.stableEngine
	shortStableEngine := se.shortStableEngine
	if stableEngine.enableSampled {
		longTime := stableEngine.predTime
		shortTime := shortStableEngine.predTime
		se.predTime = shortStableEngine.predTime
		diff := float64((longTime - shortTime) / (longTime + shortTime))
		diffBoundary := *SampledRunnerThresholdFlag
		if diff <= diffBoundary && diff >= -diffBoundary {
			se.enableSampled = true
			se.predTime = shortTime
			se.predTimeSum = shortTime * sim.VTimeInSec(se.granulary)
			se.predTimeNum = uint64(se.granulary)
		}
	} else if shortStableEngine.enableSampled {
		se.predTime = stableEngine.predTime
	}
	if se.enableSampled {
		log.Printf("Warp Sampling is enabled")
	}
}

// Predict the execution time of the next wavefronts
func (se *SampledEngine) Predict() (sim.VTimeInSec, bool) {
	return se.predTime, se.enableSampled
}
