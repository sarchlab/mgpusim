package runner

import (
	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/platform"
)

type RunnerBuilder struct {
	platform *platform.Platform
}

func (r *RunnerBuilder) WithPlatform(platform *platform.Platform) *RunnerBuilder {
	r.platform = platform
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
		platform: r.platform,
	}
}

func (r *RunnerBuilder) platformMustBeSet() {
	if r.platform == nil {
		log.Panic("Platform must be set")
	}
}
