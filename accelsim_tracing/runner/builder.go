package runner

import (
	"log"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/platform"
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
