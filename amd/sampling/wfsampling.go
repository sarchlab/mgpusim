package sampling

import (
	"flag"
	"log"
	"sync"
	"time"

	"github.com/sarchlab/akita/v5/sim/modeling"

	"github.com/sarchlab/akita/v5/sim/timing"
)

// SampledRunnerFlag is used to enable wf sampling
var SampledRunnerFlag = flag.Bool("wf-sampling", false, "enable wavefront-level sampled simulation.")

// SampledRunnerThresholdFlag is used to set the threshold of the sampling
var SampledRunnerThresholdFlag = flag.Float64("sampled-threshold", 0.03,
	"the threshold of the sampled execution to enable sampling simulation.")

// SampledRunnerGranularyFlag is used to set the granulary of the sampling
var SampledRunnerGranularyFlag = flag.Int("sampled-granulary", 1024,
	"the granulary of the sampled execution to collect and analyze data.")

const picosPerSecond = 1e12

// SampledEngine is used to detect if the wavefront sampling is stable or not.
// Internal time statistics are float64 seconds; the public API exchanges
// timing.VTimeInPicoSec.
type SampledEngine struct {
	mu                   sync.Mutex
	name                 string
	boundary             float64
	predTime             float64
	enableSampled        bool
	disableEngine        bool
	SimTime              float64 `json:"simtime"`
	WallTime             float64 `json:"walltime"`
	FullSimWalltime      float64 `json:"fullsimwalltime"`
	FullSimWallTimeStart time.Time
	dataIdx              uint64
	stableEngine         *StableEngine
	shortStableEngine    *StableEngine
	predTimeSum          float64
	predTimeNum          uint64
	granularity          int
}

// Reset all status
func (se *SampledEngine) Reset() {
	se.mu.Lock()
	defer se.mu.Unlock()
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
		boundary:          boundary,
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

// NewSimulationEngine constructs and registers the simulation's sampling state.
// The same engine is shared by its command processors and compute units.
func NewSimulationEngine(s modeling.Simulation, granularity int, boundary float64, enabled bool) *SampledEngine {
	engine := NewSampledEngine(granularity, boundary, false)
	engine.name = "WavefrontSampling"
	if !enabled {
		engine.Disabled()
	}
	s.RegisterResource(engine)
	return engine
}

// Name identifies the sampling checkpoint resource.
func (se *SampledEngine) Name() string { return se.name }

// Disabled the sampling engine
func (se *SampledEngine) Disabled() {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.disableEngine = true
}

// Enable the sampling engine
func (se *SampledEngine) Enable() {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.disableEngine = false
}

// IfDisable the sampling engine
func (se *SampledEngine) IfDisable() bool {
	se.mu.Lock()
	defer se.mu.Unlock()
	return se.disableEngine
}

// Collect the runtime information
func (se *SampledEngine) Collect(
	issueTime timing.VTimeInPicoSec,
	finishTime timing.VTimeInPicoSec,
) {
	se.mu.Lock()
	defer se.mu.Unlock()
	if se.enableSampled || se.disableEngine { //we do not need to collect data if sampling is enabled
		return
	}

	se.dataIdx++
	if se.dataIdx < 1024 { // discard the first 1024 data
		return
	}

	issue := float64(issueTime) / picosPerSecond
	finish := float64(finishTime) / picosPerSecond

	se.stableEngine.Collect(issue, finish)
	se.shortStableEngine.Collect(issue, finish)
	stableEngine := se.stableEngine
	shortStableEngine := se.shortStableEngine

	if stableEngine.enableSampled {
		longTime := stableEngine.predTime
		shortTime := shortStableEngine.predTime
		se.predTime = shortStableEngine.predTime
		diff := (longTime - shortTime) / (longTime + shortTime)
		diffBoundary := se.boundary
		if diff <= diffBoundary && diff >= -diffBoundary {
			se.enableSampled = true
			se.predTime = shortTime
			se.predTimeSum = shortTime * float64(se.granularity)
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
func (se *SampledEngine) Predict() (timing.VTimeInPicoSec, bool) {
	se.mu.Lock()
	defer se.mu.Unlock()
	return timing.VTimeInPicoSec(se.predTime * picosPerSecond), se.enableSampled
}
