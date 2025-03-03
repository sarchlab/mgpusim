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
	SimTime              float64 `json:"simtime"`
	WallTime             float64 `json:"walltime"`
	FullSimWalltime      float64 `json:"fullsimwalltime"`
	FullSimWallTimeStart time.Time
	dataIdx              uint64
	stableEngine         *StableEngine
	shortStableEngine    *StableEngine
	predTimeSum          sim.VTimeInSec
	predTimeNum          uint64
	granularity          int
}

// Reset all status
func (se *SampledEngine) Reset() {
	se.FullSimWallTimeStart = time.Now()
	se.stableEngine.Reset()
	se.shortStableEngine.Reset()
	se.predTime = 0
	se.predTimeNum = 0
	se.predTimeSum = 0
	se.dataIdx = 0
	se.enableSampled = false
}

// NewSampledEngine is used to new a sampled engine for wavefront sampling
func NewSampledEngine(
	granularity int,
	boundary float64,
	control bool,
) *SampledEngine {
	stableEngine := &StableEngine{
		granularity: granularity,
		boundary:    boundary,
	}
	shortStableEngine := &StableEngine{
		granularity: granularity / 2,
		boundary:    boundary,
	}
	ret := &SampledEngine{
		stableEngine:      stableEngine,
		shortStableEngine: shortStableEngine,
		granularity:       granularity / 2,
	}
	ret.Reset()
	if control {
		ret.disableEngine = false
	}
	return ret
}

// SampledEngineInstance is used to monitor wavefront sampling
var SampledEngineInstance *SampledEngine

// InitSampledEngine is used to initial all status and data structure
func InitSampledEngine() {
	SampledEngineInstance = NewSampledEngine(
		*SampledRunnerGranularyFlag, *SampledRunnerThresholdFlag, false)

	if *SampledRunnerFlag {
		SampledEngineInstance.Enable()
	} else {
		SampledEngineInstance.Disabled()
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
func (se *SampledEngine) Collect(
	issueTime sim.VTimeInSec,
	finishTime sim.VTimeInSec,
) {
	if se.enableSampled || se.disableEngine { //we do not need to collect data if sampling is enabled
		return
	}

	se.dataIdx++
	if se.dataIdx < 1024 { // discard the first 1024 data
		return
	}

	se.stableEngine.Collect(issueTime, finishTime)
	se.shortStableEngine.Collect(issueTime, finishTime)
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
			se.predTimeSum = shortTime * sim.VTimeInSec(se.granularity)
			se.predTimeNum = uint64(se.granularity)
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
