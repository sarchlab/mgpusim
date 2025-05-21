package runner

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// SimulationFlags houses flags for simulation behavior.
var SimulationFlags = flag.NewFlagSet("Simulation", flag.ExitOnError)

// HardwareFlags houses flags for hardware configuration.
var HardwareFlags = flag.NewFlagSet("Hardware", flag.ExitOnError)

// ReportFlags houses flags for report generation.
var ReportFlags = flag.NewFlagSet("Report", flag.ExitOnError)

// BenchmarkFlags houses flags for benchmark-specific settings.
var BenchmarkFlags = flag.NewFlagSet("Benchmark", flag.ExitOnError)

// Simulation flags
var timingFlag = SimulationFlags.Bool("timing", false, "Run detailed timing simulation.")
var maxInstCount = SimulationFlags.Uint64("max-inst", 0,
	"Terminate the simulation after the given number of instructions is retired.")
var parallelFlag = SimulationFlags.Bool("parallel", false,
	"Run the simulation in parallel.")
var verifyFlag = SimulationFlags.Bool("verify", false, "Verify the emulation result.")
var customPortForAkitaRTM = SimulationFlags.Int("akitartm-port", 0,
	`Custom port to host AkitaRTM. A 4-digit or 5-digit port number is required. If
this number is not given or a invalid number is given number, a random port
will be used.`)
var disableAkitaRTM = SimulationFlags.Bool("disable-rtm", false, "Disable the AkitaRTM monitoring portal")

// Hardware flags
var isaDebug = HardwareFlags.Bool("debug-isa", false, "Generate the ISA debugging file.")
var gpuFlag = HardwareFlags.String("gpus", "",
	"The GPUs to use, use a format like 1,2,3,4. By default, GPU 1 is used.")
var unifiedGPUFlag = HardwareFlags.String("unified-gpus", "",
	`Run multi-GPU benchmark in a unified mode.
Use a format like 1,2,3,4. Cannot coexist with -gpus.`)
var useUnifiedMemoryFlag = HardwareFlags.Bool("use-unified-memory", false,
	"Run benchmark with Unified Memory or not")
var magicMemoryCopy = HardwareFlags.Bool("magic-memory-copy", false,
	"Copy data from CPU directly to global memory")

// Report flags
var memTracing = ReportFlags.Bool("trace-mem", false, "Generate memory trace")
var instCountReportFlag = ReportFlags.Bool("report-inst-count", false,
	"Report the number of instructions executed in each compute unit.")
var cacheLatencyReportFlag = ReportFlags.Bool("report-cache-latency", false,
	"Report the average cache latency.")
var cacheHitRateReportFlag = ReportFlags.Bool("report-cache-hit-rate", false,
	"Report the cache hit rate of each cache.")
var tlbHitRateReportFlag = ReportFlags.Bool("report-tlb-hit-rate", false,
	"Report the TLB hit rate of each TLB.")
var rdmaTransactionCountReportFlag = ReportFlags.Bool("report-rdma-transaction-count",
	false, "Report the number of transactions going through the RDMA engines.")
var dramTransactionCountReportFlag = ReportFlags.Bool("report-dram-transaction-count",
	false, "Report the number of transactions accessing the DRAMs.")
var reportAll = ReportFlags.Bool("report-all", false, "Report all metrics to .csv file.")
var filenameFlag = ReportFlags.String("metric-file-name", "metrics",
	"Modify the name of the output csv file.")
var bufferLevelTraceDirFlag = ReportFlags.String("buffer-level-trace-dir", "",
	"The directory to dump the buffer level traces.")
var bufferLevelTracePeriodFlag = ReportFlags.Float64("buffer-level-trace-period", 0.0,
	"The period to dump the buffer level trace.")
var simdBusyTimeTracerFlag = ReportFlags.Bool("report-busy-time", false, "Report SIMD Unit's busy time")
var reportCPIStackFlag = ReportFlags.Bool("report-cpi-stack", false, "Report CPI stack")
var analyzerNameFlag = ReportFlags.String("analyzer-name", "",
	"The name of the analyzer to use.")
var analyzerPeriodFlag = ReportFlags.Float64("analyzer-period", 0.0,
	"The period to dump the analyzer results.")
var visTracing = ReportFlags.Bool("trace-vis", false,
	"Generate trace for visualization purposes.")
var visTracerDB = ReportFlags.String("trace-vis-db", "sqlite",
	"The database to store the visualization trace. Possible values are "+
		"sqlite, mysql, and csv.")
var visTracerDBFileName = ReportFlags.String("trace-vis-db-file", "",
	"The file name of the database to store the visualization trace. "+
		"Extension names are not required. "+
		"If not specified, a random file name will be used. "+
		"This flag does not work with Mysql db. When MySQL is used, "+
		"the database name is always randomly generated.")
var visTraceStartTime = ReportFlags.Float64("trace-vis-start", -1,
	"The starting time to collect visualization traces. A negative number "+
		"represents starting from the beginning.")
var visTraceEndTime = ReportFlags.Float64("trace-vis-end", -1,
	"The end time of collecting visualization traces. A negative number"+
		"means that the trace will be collected to the end of the simulation.")

// Benchmark flags - Will be populated by specific benchmark packages
// Example: BenchmarkFlags.String("length", "1024", "Length of the FIR filter.")

// ParseAllFlags parses all the flag sets.
func ParseAllFlags() {
	// Create a custom help flag
	help := flag.Bool("h", false, "Show help message")
	flag.BoolVar(help, "help", false, "Show help message")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nSimulation Flags:\n")
		SimulationFlags.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nHardware Flags:\n")
		HardwareFlags.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nReport Flags:\n")
		ReportFlags.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nBenchmark Flags:\n")
		BenchmarkFlags.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nOther Flags:\n")
		flag.PrintDefaults() // Print any other global flags
	}

	// Collect all arguments for each flag set
	var simArgs, hwArgs, reportArgs, benchmarkArgs, globalArgs []string
	currentArgs := &globalArgs

	for _, arg := range os.Args[1:] {
		switch {
		case strings.HasPrefix(arg, "-timing"),
			strings.HasPrefix(arg, "-max-inst"),
			strings.HasPrefix(arg, "-parallel"),
			strings.HasPrefix(arg, "-verify"),
			strings.HasPrefix(arg, "-akitartm-port"),
			strings.HasPrefix(arg, "-disable-rtm"):
			currentArgs = &simArgs
		case strings.HasPrefix(arg, "-debug-isa"),
			strings.HasPrefix(arg, "-gpus"),
			strings.HasPrefix(arg, "-unified-gpus"),
			strings.HasPrefix(arg, "-use-unified-memory"),
			strings.HasPrefix(arg, "-magic-memory-copy"):
			currentArgs = &hwArgs
		case strings.HasPrefix(arg, "-trace-mem"),
			strings.HasPrefix(arg, "-report-inst-count"),
			strings.HasPrefix(arg, "-report-cache-latency"),
			strings.HasPrefix(arg, "-report-cache-hit-rate"),
			strings.HasPrefix(arg, "-report-tlb-hit-rate"),
			strings.HasPrefix(arg, "-report-rdma-transaction-count"),
			strings.HasPrefix(arg, "-report-dram-transaction-count"),
			strings.HasPrefix(arg, "-report-all"),
			strings.HasPrefix(arg, "-metric-file-name"),
			strings.HasPrefix(arg, "-buffer-level-trace-dir"),
			strings.HasPrefix(arg, "-buffer-level-trace-period"),
			strings.HasPrefix(arg, "-report-busy-time"),
			strings.HasPrefix(arg, "-report-cpi-stack"),
			strings.HasPrefix(arg, "-analyzer-name"),
			strings.HasPrefix(arg, "-analyzer-period"),
			strings.HasPrefix(arg, "-trace-vis"),
			strings.HasPrefix(arg, "-trace-vis-db"),
			strings.HasPrefix(arg, "-trace-vis-db-file"),
			strings.HasPrefix(arg, "-trace-vis-start"),
			strings.HasPrefix(arg, "-trace-vis-end"):
			currentArgs = &reportArgs
		// This case is tricky as benchmark flags are not predefined here.
		// A more robust solution might involve a prefix for benchmark flags,
		// or benchmarks registering their flags with BenchmarkFlags.
		// For now, we'll assume benchmark flags don't overlap with others
		// or they are handled by the benchmark itself.
		// We can add a placeholder here if needed.
		// else if isBenchmarkFlag(arg) {
		// 	currentArgs = &benchmarkArgs
		// }
		default:
			// If it's a value for a preceding flag, it should stay with currentArgs.
			// Otherwise, it could be a global flag or a benchmark flag not caught above.
			if !strings.HasPrefix(arg, "-") && len(*currentArgs) > 0 {
				// This is likely a value for the previous flag
			} else {
				// This could be a global flag or a new benchmark flag
				// For simplicity, let's assign unknown flags to global or benchmark
				// This part needs refinement based on how benchmark flags are handled.
				// For now, let's put them to globalArgs to be safe.
				currentArgs = &globalArgs
			}
		}
		*currentArgs = append(*currentArgs, arg)
	}

	// It's important to parse global flags first, especially the help flag.
	flag.Parse(globalArgs)
	if *help {
		flag.Usage()
		os.Exit(0)
	}

	// Parse each flag set with its collected arguments
	// ExitOnError will cause the program to exit if parsing fails.
	if err := SimulationFlags.Parse(simArgs); err != nil && err != flag.ErrHelp {
		// Allow ErrHelp to be handled by the global help flag if not parsed by specific set
		fmt.Fprintf(os.Stderr, "Error parsing simulation flags: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}
	if err := HardwareFlags.Parse(hwArgs); err != nil && err != flag.ErrHelp {
		fmt.Fprintf(os.Stderr, "Error parsing hardware flags: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}
	if err := ReportFlags.Parse(reportArgs); err != nil && err != flag.ErrHelp {
		fmt.Fprintf(os.Stderr, "Error parsing report flags: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}
	if err := BenchmarkFlags.Parse(benchmarkArgs); err != nil && err != flag.ErrHelp {
		// BenchmarkFlags might be empty if no benchmark-specific flags are passed
		// or defined yet. This is not necessarily an error.
		// However, if parsing fails for other reasons, it's an error.
		if len(benchmarkArgs) > 0 || (err != nil && err.Error() != "flag provided but not defined: -h") { // A bit hacky check for -h
			fmt.Fprintf(os.Stderr, "Error parsing benchmark flags: %v\n", err)
			flag.Usage()
			os.Exit(1)
		}
	}
}
