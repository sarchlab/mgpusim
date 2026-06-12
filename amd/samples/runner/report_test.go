package runner

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/simulation"
)

type testHookableComponent struct {
	*sim.ComponentBase
}

func newTestHookableComponent(name string) *testHookableComponent {
	return &testHookableComponent{
		ComponentBase: sim.NewComponentBase(name),
	}
}

func (c *testHookableComponent) NotifyRecv(port sim.Port) {}

func (c *testHookableComponent) NotifyPortFree(port sim.Port) {}

func TestCacheReportingFlagsControlTheirOwnTracers(t *testing.T) {
	tests := []struct {
		name             string
		reportAll        bool
		reportLatency    bool
		reportHitRate    bool
		wantLatencyCount int
		wantHitRateCount int
	}{
		{
			name:             "cache hit rate only",
			reportHitRate:    true,
			wantHitRateCount: 1,
		},
		{
			name:             "cache latency only",
			reportLatency:    true,
			wantLatencyCount: 1,
		},
		{
			name:             "report all",
			reportAll:        true,
			wantLatencyCount: 1,
			wantHitRateCount: 1,
		},
		{
			name: "neither cache report flag",
		},
	}

	oldReportAll := *reportAll
	oldReportLatency := *cacheLatencyReportFlag
	oldReportHitRate := *cacheHitRateReportFlag
	defer func() {
		*reportAll = oldReportAll
		*cacheLatencyReportFlag = oldReportLatency
		*cacheHitRateReportFlag = oldReportHitRate
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*reportAll = tt.reportAll
			*cacheLatencyReportFlag = tt.reportLatency
			*cacheHitRateReportFlag = tt.reportHitRate

			s := simulation.MakeBuilder().WithoutMonitoring().Build()
			s.RegisterComponent(newTestHookableComponent("TestCache"))

			reporter := &reporter{}
			reporter.injectCacheLatencyTracer(s)
			reporter.injectCacheHitRateTracer(s)

			if got := len(reporter.cacheLatencyTracers); got != tt.wantLatencyCount {
				t.Fatalf("cache latency tracer count = %d, want %d", got, tt.wantLatencyCount)
			}

			if got := len(reporter.cacheHitRateTracers); got != tt.wantHitRateCount {
				t.Fatalf("cache hit-rate tracer count = %d, want %d", got, tt.wantHitRateCount)
			}
		})
	}
}
