package runner

import (
	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/akita/v4/simulation"
	"github.com/sarchlab/mgpusim/v4/nvidia/platform"
)

type RunnerBuilder struct {
	platform   *platform.Platform
	simulation *simulation.Simulation
}

func (r *RunnerBuilder) WithPlatform(platform *platform.Platform) *RunnerBuilder {
	r.platform = platform
	return r
}

func (r *RunnerBuilder) WithSimulation(simulation *simulation.Simulation) *RunnerBuilder {
	r.simulation = simulation
	return r
}

func (r *RunnerBuilder) ParseFlags() {
	// if *instCountReportFlag {
	// 	r.ReportInstCount = true
	// }
}

func (r *RunnerBuilder) Build() *Runner {
	r.platformMustBeSet()

	return &Runner{
		platform:   r.platform,
		simulation: r.simulation,
	}
}

func (r *RunnerBuilder) platformMustBeSet() {
	if r.platform == nil {
		log.Panic("Platform must be set")
	}
}
