package component

import "flag"

var instCountReportFlag = flag.Bool("report-inst-count", false,
	"Report the number of instructions executed in each compute unit.")

type Runner struct {
	platform   *Platform
	benchmarks []*Benchmark

	ReportInstCount bool
}

func NewRunner() *Runner {
	return &Runner{}
}

func (r *Runner) SetPlatform(platform *Platform) {
	r.platform = platform
}

func (r *Runner) AddBenchmark(benchmark *Benchmark) {
	r.benchmarks = append(r.benchmarks, benchmark)
}

func (r *Runner) ParseFlags() {
	if *instCountReportFlag {
		r.ReportInstCount = true
	}
}

func (r *Runner) Run() {
	r.platform.gpu.TickLater(0)
	r.platform.Engine().Run()

	for _, benchmark := range r.benchmarks {
		kernelCount := benchmark.KernelsCount()
		for i := int64(0); i < kernelCount; i++ {
			kernel := benchmark.Kernel(i)
			r.platform.Driver().RunKernel(*kernel)
		}
		r.reportStatus()
	}

	r.platform.Engine().Finished()
}

type ReportProperties string

const (
	ReportPropertiesInstCount = "inst-count"
)

func (r *Runner) reportStatus() {
	r.reportInstCount()
}

func (r *Runner) reportInstCount() {
	if r.ReportInstCount {
		r.platform.Driver().ReportStatus(ReportPropertiesInstCount)
	}
}
