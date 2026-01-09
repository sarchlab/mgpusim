package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/sarchlab/mgpusim/v4/amd/cli/config"
	"github.com/sarchlab/mgpusim/v4/amd/cli/registry"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a GPU simulation",
	Long: `Run a GPU benchmark simulation with the specified configuration.

Examples:
  # Basic emulation
  mgpusim_amd run --benchmark=fir --fir.length=1024

  # Timing simulation with reports
  mgpusim_amd run --benchmark=fir --fir.length=1024 --sim.timing --report.all

  # Multi-GPU
  mgpusim_amd run --benchmark=fir --hw.gpus=1,2,3,4 --sim.timing

  # From config file
  mgpusim_amd run --config=simulation.yaml`,
	PreRunE: preRun,
	RunE:    runSimulation,
}

func init() {
	flags := runCmd.Flags()

	// Benchmark selection
	flags.String("benchmark", "", "Benchmark to run (required)")

	// Simulation flags (sim.*)
	flags.Bool("sim.timing", false, "Enable timing simulation")
	flags.Bool("sim.parallel", false, "Enable parallel simulation")
	flags.Bool("sim.verify", false, "Verify simulation results")
	flags.Uint64("sim.max-inst", 0, "Max instructions (0=unlimited)")

	// Hardware flags (hw.*)
	flags.String("hw.arch", "gcn3", "GPU architecture (gcn3|cdna3)")
	flags.String("hw.gpu", "r9nano", "GPU model (r9nano|mi300a)")
	flags.IntSlice("hw.gpus", []int{1}, "GPU IDs to use")
	flags.IntSlice("hw.unified-gpus", nil, "Unified GPU IDs (mutually exclusive with hw.gpus)")
	flags.Bool("hw.unified-memory", false, "Enable unified memory")
	flags.Bool("hw.magic-memory-copy", false, "Direct CPU-to-GPU memory copy")

	// Report flags (report.*)
	flags.Bool("report.all", false, "Enable all reports")
	flags.String("report.filename", "metrics", "Output filename")
	flags.Bool("report.inst-count", false, "Report instruction count")
	flags.Bool("report.cache-latency", false, "Report cache latency")
	flags.Bool("report.cache-hit-rate", false, "Report cache hit rate")
	flags.Bool("report.tlb-hit-rate", false, "Report TLB hit rate")
	flags.Bool("report.rdma-transaction-count", false, "Report RDMA transactions")
	flags.Bool("report.dram-transaction-count", false, "Report DRAM transactions")
	flags.Bool("report.simd-busy-time", false, "Report SIMD busy time")
	flags.Bool("report.cpi-stack", false, "Report CPI stack")

	// Tracing flags (trace.*)
	flags.Bool("trace.vis", false, "Enable visualization tracing")
	flags.String("trace.vis-db", "sqlite", "Trace database type (sqlite|mysql|csv)")
	flags.String("trace.vis-db-file", "", "Trace database filename")
	flags.Float64("trace.vis-start", -1, "Trace start time")
	flags.Float64("trace.vis-end", -1, "Trace end time")
	flags.Bool("trace.mem", false, "Enable memory tracing")
	flags.Bool("trace.isa-debug", false, "Enable ISA debugging")

	// Register all benchmark-specific flags upfront
	registerBenchmarkFlags(flags)

	// Bind all flags to viper
	_ = viper.BindPFlags(flags)
}

func registerBenchmarkFlags(flags *pflag.FlagSet) {
	for name, meta := range registry.Registry {
		for _, param := range meta.Parameters {
			flagName := name + "." + param.Name
			switch param.Type {
			case "int", "uint":
				flags.Int(flagName, toIntDefault(param.Default), param.Description)
			case "float":
				flags.Float64(flagName, toFloatDefault(param.Default), param.Description)
			case "bool":
				flags.Bool(flagName, toBoolDefault(param.Default), param.Description)
			case "string":
				flags.String(flagName, toStringDefault(param.Default), param.Description)
			}
		}
	}
}

func preRun(cmd *cobra.Command, args []string) error {
	// Validate benchmark is specified
	benchmarkName := viper.GetString("benchmark")
	if benchmarkName == "" {
		return fmt.Errorf("--benchmark is required")
	}

	if _, ok := registry.Registry[benchmarkName]; !ok {
		return fmt.Errorf("unknown benchmark: %s", benchmarkName)
	}

	return nil
}

func runSimulation(cmd *cobra.Command, args []string) error {
	// Build configuration from viper
	cfg := buildConfig()

	// Validate
	if cfg.Benchmark.Name == "" {
		return fmt.Errorf("benchmark name is required")
	}

	meta, ok := registry.Registry[cfg.Benchmark.Name]
	if !ok {
		return fmt.Errorf("unknown benchmark: %s", cfg.Benchmark.Name)
	}

	// Extract benchmark params from viper
	for _, param := range meta.Parameters {
		flagName := cfg.Benchmark.Name + "." + param.Name
		if viper.IsSet(flagName) {
			cfg.Benchmark.Params[param.Name] = viper.Get(flagName)
		} else {
			cfg.Benchmark.Params[param.Name] = param.Default
		}
	}

	// Print configuration summary
	printConfigSummary(cfg)

	// Run the simulation
	return executeSimulation(cfg, meta)
}

func buildConfig() *config.Config {
	cfg := config.NewDefault()

	// Benchmark
	cfg.Benchmark.Name = viper.GetString("benchmark")
	cfg.Benchmark.Params = make(map[string]any)

	// Simulation
	cfg.Simulation.Timing = viper.GetBool("sim.timing")
	cfg.Simulation.Parallel = viper.GetBool("sim.parallel")
	cfg.Simulation.Verify = viper.GetBool("sim.verify")
	cfg.Simulation.MaxInst = viper.GetUint64("sim.max-inst")

	// Hardware
	cfg.Hardware.Arch = viper.GetString("hw.arch")
	cfg.Hardware.GPU = viper.GetString("hw.gpu")
	cfg.Hardware.GPUs = viper.GetIntSlice("hw.gpus")
	cfg.Hardware.UnifiedGPUs = viper.GetIntSlice("hw.unified-gpus")
	cfg.Hardware.UnifiedMemory = viper.GetBool("hw.unified-memory")
	cfg.Hardware.MagicMemoryCopy = viper.GetBool("hw.magic-memory-copy")

	// Report
	cfg.Report.All = viper.GetBool("report.all")
	cfg.Report.Filename = viper.GetString("report.filename")
	cfg.Report.Metrics.InstCount = viper.GetBool("report.inst-count")
	cfg.Report.Metrics.CacheLatency = viper.GetBool("report.cache-latency")
	cfg.Report.Metrics.CacheHitRate = viper.GetBool("report.cache-hit-rate")
	cfg.Report.Metrics.TLBHitRate = viper.GetBool("report.tlb-hit-rate")
	cfg.Report.Metrics.RDMATransactionCount = viper.GetBool("report.rdma-transaction-count")
	cfg.Report.Metrics.DRAMTransactionCount = viper.GetBool("report.dram-transaction-count")
	cfg.Report.Metrics.SIMDBusyTime = viper.GetBool("report.simd-busy-time")
	cfg.Report.Metrics.CPIStack = viper.GetBool("report.cpi-stack")

	// Tracing
	cfg.Tracing.Visualization = viper.GetBool("trace.vis")
	cfg.Tracing.VisDB = viper.GetString("trace.vis-db")
	cfg.Tracing.VisDBFile = viper.GetString("trace.vis-db-file")
	cfg.Tracing.VisStartTime = viper.GetFloat64("trace.vis-start")
	cfg.Tracing.VisEndTime = viper.GetFloat64("trace.vis-end")
	cfg.Tracing.Memory = viper.GetBool("trace.mem")
	cfg.Tracing.ISADebug = viper.GetBool("trace.isa-debug")

	return cfg
}

func printConfigSummary(cfg *config.Config) {
	fmt.Println("=== MGPUSim Configuration ===")
	fmt.Printf("Benchmark: %s\n", cfg.Benchmark.Name)

	mode := "Emulation"
	if cfg.Simulation.Timing {
		mode = "Timing"
	}
	fmt.Printf("Mode: %s", mode)
	if cfg.Simulation.Parallel {
		fmt.Print(" (parallel)")
	}
	fmt.Println()

	fmt.Printf("Architecture: %s (%s)\n", cfg.Hardware.Arch, cfg.Hardware.GPU)
	fmt.Printf("GPUs: %v\n", cfg.Hardware.GPUs)

	if len(cfg.Benchmark.Params) > 0 {
		fmt.Println("Parameters:")
		for k, v := range cfg.Benchmark.Params {
			fmt.Printf("  %s: %v\n", k, v)
		}
	}
	fmt.Println("=============================")
}

func executeSimulation(cfg *config.Config, meta registry.BenchmarkMeta) error {
	// Set os.Args for the runner's flag.Parse() call
	setRunnerFlags(cfg)

	// Create and initialize runner
	r := new(runner.Runner).Init()

	// Override runner settings from our config
	// (The runner reads from flag pointers during Init, but we need our values)
	r.Verify = cfg.Simulation.Verify
	r.Timing = cfg.Simulation.Timing
	r.Parallel = cfg.Simulation.Parallel
	r.UseUnifiedMemory = cfg.Hardware.UnifiedMemory

	// Create and configure benchmark
	benchmark := meta.Factory(r.Driver())
	meta.Configure(benchmark, cfg.Benchmark.Params)

	// Add benchmark to runner
	r.AddBenchmark(benchmark)

	// Run simulation
	r.Run()

	return nil
}

//nolint:funlen // This function maps all config fields to CLI args
func setRunnerFlags(cfg *config.Config) {
	// Build command line args that the existing runner expects
	// This is a bridge until we refactor runner to accept Config directly
	args := []string{"mgpusim_amd"}

	args = appendSimulationFlags(args, cfg)
	args = appendHardwareFlags(args, cfg)
	args = appendReportFlags(args, cfg)
	args = appendTracingFlags(args, cfg)

	os.Args = args
}

func appendSimulationFlags(args []string, cfg *config.Config) []string {
	if cfg.Simulation.Timing {
		args = append(args, "-timing")
	}
	if cfg.Simulation.Parallel {
		args = append(args, "-parallel")
	}
	if cfg.Simulation.Verify {
		args = append(args, "-verify")
	}
	if cfg.Simulation.MaxInst > 0 {
		args = append(args, fmt.Sprintf("-max-inst=%d", cfg.Simulation.MaxInst))
	}
	return args
}

func appendHardwareFlags(args []string, cfg *config.Config) []string {
	args = append(args, fmt.Sprintf("-arch=%s", cfg.Hardware.Arch))
	args = append(args, fmt.Sprintf("-gpu=%s", cfg.Hardware.GPU))

	if len(cfg.Hardware.GPUs) > 0 {
		args = append(args, fmt.Sprintf("-gpus=%s", intsToString(cfg.Hardware.GPUs)))
	}
	if len(cfg.Hardware.UnifiedGPUs) > 0 {
		args = append(args, fmt.Sprintf("-unified-gpus=%s", intsToString(cfg.Hardware.UnifiedGPUs)))
	}
	if cfg.Hardware.UnifiedMemory {
		args = append(args, "-use-unified-memory")
	}
	if cfg.Hardware.MagicMemoryCopy {
		args = append(args, "-magic-memory-copy")
	}
	return args
}

func appendReportFlags(args []string, cfg *config.Config) []string {
	if cfg.Report.All {
		args = append(args, "-report-all")
	}
	if cfg.Report.Filename != "metrics" {
		args = append(args, fmt.Sprintf("-metric-file-name=%s", cfg.Report.Filename))
	}
	if cfg.Report.Metrics.InstCount {
		args = append(args, "-report-inst-count")
	}
	if cfg.Report.Metrics.CacheLatency {
		args = append(args, "-report-cache-latency")
	}
	if cfg.Report.Metrics.CacheHitRate {
		args = append(args, "-report-cache-hit-rate")
	}
	if cfg.Report.Metrics.TLBHitRate {
		args = append(args, "-report-tlb-hit-rate")
	}
	if cfg.Report.Metrics.RDMATransactionCount {
		args = append(args, "-report-rdma-transaction-count")
	}
	if cfg.Report.Metrics.DRAMTransactionCount {
		args = append(args, "-report-dram-transaction-count")
	}
	if cfg.Report.Metrics.SIMDBusyTime {
		args = append(args, "-report-busy-time")
	}
	if cfg.Report.Metrics.CPIStack {
		args = append(args, "-report-cpi-stack")
	}
	return args
}

func appendTracingFlags(args []string, cfg *config.Config) []string {
	if cfg.Tracing.Visualization {
		args = append(args, "-trace-vis")
	}
	if cfg.Tracing.VisDB != "sqlite" {
		args = append(args, fmt.Sprintf("-trace-vis-db=%s", cfg.Tracing.VisDB))
	}
	if cfg.Tracing.VisDBFile != "" {
		args = append(args, fmt.Sprintf("-trace-vis-db-file=%s", cfg.Tracing.VisDBFile))
	}
	if cfg.Tracing.VisStartTime >= 0 {
		args = append(args, fmt.Sprintf("-trace-vis-start=%f", cfg.Tracing.VisStartTime))
	}
	if cfg.Tracing.VisEndTime >= 0 {
		args = append(args, fmt.Sprintf("-trace-vis-end=%f", cfg.Tracing.VisEndTime))
	}
	if cfg.Tracing.Memory {
		args = append(args, "-trace-mem")
	}
	if cfg.Tracing.ISADebug {
		args = append(args, "-debug-isa")
	}
	return args
}

func intsToString(ints []int) string {
	strs := make([]string, len(ints))
	for i, v := range ints {
		strs[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(strs, ",")
}

func toIntDefault(v any) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	default:
		return 0
	}
}

func toFloatDefault(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	default:
		return 0
	}
}

func toBoolDefault(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func toStringDefault(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
