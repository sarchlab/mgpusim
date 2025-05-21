package runner

import (
	"flag"
	"fmt"
	"os"
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
	
	// Distribute arguments to their respective slices
	// This logic assumes that flags for a specific category are somewhat predictable by prefix.
	// Benchmark flags are treated as "everything else".
	for i := 0; i < len(os.Args[1:]); i++ {
		arg := os.Args[1+i] // Correctly get argument from os.Args

		isGlobalHelp := false
		if arg == "-h" || arg == "-help" {
			isGlobalHelp = true
		}

		// Check if the argument is a value for a preceding flag
		// This simple check assumes values don't start with '-'
		// More sophisticated parsing might be needed for flags that take negative numbers as values
		isValue := !strings.HasPrefix(arg, "-")

		// Determine which slice the current argument (and potentially its value) belongs to
		// The logic for `currentArgs` in the original code was problematic.
		// This revised logic attempts to categorize based on prefixes.
		// It's still not perfect, as a benchmark flag could coincidentally share a prefix.
		// However, it's an improvement. The most robust way is for benchmarks to register
		// their flags with a specific prefix or for this function to know all flags.
		
		// Default to benchmarkArgs for unknown flags
		var currentTargetSlice *[]string = &benchmarkArgs 

		switch {
		case isGlobalHelp:
			currentTargetSlice = &globalArgs
		case strings.HasPrefix(arg, "-timing"),
			strings.HasPrefix(arg, "-max-inst"),
			strings.HasPrefix(arg, "-parallel"),
			strings.HasPrefix(arg, "-verify"),
			strings.HasPrefix(arg, "-akitartm-port"),
			strings.HasPrefix(arg, "-disable-rtm"):
			currentTargetSlice = &simArgs
		case strings.HasPrefix(arg, "-debug-isa"),
			strings.HasPrefix(arg, "-gpus"),
			strings.HasPrefix(arg, "-unified-gpus"),
			strings.HasPrefix(arg, "-use-unified-memory"),
			strings.HasPrefix(arg, "-magic-memory-copy"):
			currentTargetSlice = &hwArgs
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
			currentTargetSlice = &reportArgs
		}
		
		*currentTargetSlice = append(*currentTargetSlice, arg)
		
		// If this arg is a flag (not a value itself) and the next arg exists and is a value,
		// append the value to the same slice.
		if strings.HasPrefix(arg, "-") && (i+1 < len(os.Args[1:])) {
			nextArg := os.Args[1+i+1]
			if !strings.HasPrefix(nextArg, "-") {
				// Check if the flag type expects an argument.
				// This is a simplification; BoolFlags don't consume the next argument.
				// The standard library flag parsing handles this correctly.
				// For this distribution logic, we assume if a -flag is followed by a non -flag, it's its value.
				// This might misclassify a positional argument if it follows a boolean flag.
				// However, the flag.*.Parse methods will ultimately validate this.
				definedFlag := SimulationFlags.Lookup(strings.TrimPrefix(arg, "-")) != nil ||
				               HardwareFlags.Lookup(strings.TrimPrefix(arg, "-")) != nil ||
				               ReportFlags.Lookup(strings.TrimPrefix(arg, "-")) != nil ||
				               BenchmarkFlags.Lookup(strings.TrimPrefix(arg, "-")) != nil || // Check benchmark flags too
				               flag.CommandLine.Lookup(strings.TrimPrefix(arg, "-")) != nil // Check global -h, -help
				
				isBool := false
				if definedFlagInstance := SimulationFlags.Lookup(strings.TrimPrefix(arg, "-")); definedFlagInstance != nil {
					if _, ok := definedFlagInstance.Value.(flag.Getter).Get().(bool); ok { isBool = true }
				} else if definedFlagInstance := HardwareFlags.Lookup(strings.TrimPrefix(arg, "-")); definedFlagInstance != nil {
					if _, ok := definedFlagInstance.Value.(flag.Getter).Get().(bool); ok { isBool = true }
				} else if definedFlagInstance := ReportFlags.Lookup(strings.TrimPrefix(arg, "-")); definedFlagInstance != nil {
					if _, ok := definedFlagInstance.Value.(flag.Getter).Get().(bool); ok { isBool = true }
				} // Add similar checks for BenchmarkFlags and flag.CommandLine if needed for precision here.

				if definedFlag && !isBool { // Only append nextArg if current flag is defined and not boolean
					*currentTargetSlice = append(*currentTargetSlice, nextArg)
					i++ // Increment i because we've consumed the next argument as a value
				}
			}
		}
	}

	// Parse global flags first (this should only be -h or -help).
	// Use flag.CommandLine to parse these.
	// The `help` variable is defined using `flag.Bool` which registers on flag.CommandLine.
	if err := flag.CommandLine.Parse(globalArgs); err != nil && err != flag.ErrHelp {
		// This error should ideally not happen if globalArgs only contains -h/-help
		// or if other global flags were explicitly defined on flag.CommandLine
		fmt.Fprintf(os.Stderr, "Error parsing global flags: %v\n", err)
		flag.Usage() // Show combined usage
		os.Exit(1)
	}

	if *help { // Check if the global -h or -help flag was parsed
		flag.Usage()
		os.Exit(0)
	}

	// Parse each categorized flag set with its collected arguments.
	// Note: flag.ErrHelp is returned by FlagSet.Parse if -h or -help is in args
	// and the FlagSet is configured with ContinueOnError or ExitOnError.
	// We want our global help to take precedence.
	if err := SimulationFlags.Parse(simArgs); err != nil && err.Error() != "flag: help requested" {
		fmt.Fprintf(os.Stderr, "Error parsing simulation flags: %v\n", err)
		flag.Usage() // Show combined usage
		os.Exit(1)
	}
	if err := HardwareFlags.Parse(hwArgs); err != nil && err.Error() != "flag: help requested" {
		fmt.Fprintf(os.Stderr, "Error parsing hardware flags: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}
	if err := ReportFlags.Parse(reportArgs); err != nil && err.Error() != "flag: help requested" {
		fmt.Fprintf(os.Stderr, "Error parsing report flags: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}
	if err := BenchmarkFlags.Parse(benchmarkArgs); err != nil && err.Error() != "flag: help requested" {
		// This check is to prevent exiting if benchmarkArgs is empty and Parse is called.
		// An error like "flag provided but not defined: -h" might occur if -h was miscategorized.
		// The main global help should catch -h before this.
		isNonHelpError := true
		if err != nil {
			// Check if the error is simply because no arguments were passed to BenchmarkFlags.Parse
			// or if it's a genuine parsing error other than help.
			// An empty benchmarkArgs list will not cause Parse to error unless args contains something undefined.
			if len(benchmarkArgs) == 0 && err.Error() == "flag: help requested" { // Should not happen if global help works
				isNonHelpError = false 
			}
			// If benchmarkArgs has items, or the error is not about "help requested", then it's a real error.
		}

		if isNonHelpError && err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing benchmark flags: %v\n", err)
			flag.Usage()
			os.Exit(1)
		}
	}
}
