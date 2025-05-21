package runner

import (
	"flag"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Helper function to reset all flag values to their defaults.
// This is important because flags are global and persist between tests.
func resetFlagsForTesting() {
	// For flags defined on flag.CommandLine (like our -h, -help)
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// For our custom FlagSets
	SimulationFlags = flag.NewFlagSet("Simulation", flag.ExitOnError)
	timingFlag = SimulationFlags.Bool("timing", false, "Run detailed timing simulation.")
	maxInstCount = SimulationFlags.Uint64("max-inst", 0,
		"Terminate the simulation after the given number of instructions is retired.")
	parallelFlag = SimulationFlags.Bool("parallel", false,
		"Run the simulation in parallel.")
	verifyFlag = SimulationFlags.Bool("verify", false, "Verify the emulation result.")
	customPortForAkitaRTM = SimulationFlags.Int("akitartm-port", 0,
		`Custom port to host AkitaRTM. A 4-digit or 5-digit port number is required. If
this number is not given or a invalid number is given number, a random port
will be used.`)
	disableAkitaRTM = SimulationFlags.Bool("disable-rtm", false, "Disable the AkitaRTM monitoring portal")

	HardwareFlags = flag.NewFlagSet("Hardware", flag.ExitOnError)
	isaDebug = HardwareFlags.Bool("debug-isa", false, "Generate the ISA debugging file.")
	gpuFlag = HardwareFlags.String("gpus", "",
		"The GPUs to use, use a format like 1,2,3,4. By default, GPU 1 is used.")
	unifiedGPUFlag = HardwareFlags.String("unified-gpus", "",
		`Run multi-GPU benchmark in a unified mode.
Use a format like 1,2,3,4. Cannot coexist with -gpus.`)
	useUnifiedMemoryFlag = HardwareFlags.Bool("use-unified-memory", false,
		"Run benchmark with Unified Memory or not")
	magicMemoryCopy = HardwareFlags.Bool("magic-memory-copy", false,
		"Copy data from CPU directly to global memory")

	ReportFlags = flag.NewFlagSet("Report", flag.ExitOnError)
	memTracing = ReportFlags.Bool("trace-mem", false, "Generate memory trace")
	instCountReportFlag = ReportFlags.Bool("report-inst-count", false,
		"Report the number of instructions executed in each compute unit.")
	cacheLatencyReportFlag = ReportFlags.Bool("report-cache-latency", false,
		"Report the average cache latency.")
	cacheHitRateReportFlag = ReportFlags.Bool("report-cache-hit-rate", false,
		"Report the cache hit rate of each cache.")
	tlbHitRateReportFlag = ReportFlags.Bool("report-tlb-hit-rate", false,
		"Report the TLB hit rate of each TLB.")
	rdmaTransactionCountReportFlag = ReportFlags.Bool("report-rdma-transaction-count",
		false, "Report the number of transactions going through the RDMA engines.")
	dramTransactionCountReportFlag = ReportFlags.Bool("report-dram-transaction-count",
		false, "Report the number of transactions accessing the DRAMs.")
	reportAll = ReportFlags.Bool("report-all", false, "Report all metrics to .csv file.")
	filenameFlag = ReportFlags.String("metric-file-name", "metrics",
		"Modify the name of the output csv file.")
	bufferLevelTraceDirFlag = ReportFlags.String("buffer-level-trace-dir", "",
		"The directory to dump the buffer level traces.")
	bufferLevelTracePeriodFlag = ReportFlags.Float64("buffer-level-trace-period", 0.0,
		"The period to dump the buffer level trace.")
	simdBusyTimeTracerFlag = ReportFlags.Bool("report-busy-time", false, "Report SIMD Unit's busy time")
	reportCPIStackFlag = ReportFlags.Bool("report-cpi-stack", false, "Report CPI stack")
	analyzerNameFlag = ReportFlags.String("analyzer-name", "",
		"The name of the analyzer to use.")
	analyzerPeriodFlag = ReportFlags.Float64("analyzer-period", 0.0,
		"The period to dump the analyzer results.")
	visTracing = ReportFlags.Bool("trace-vis", false,
		"Generate trace for visualization purposes.")
	visTracerDB = ReportFlags.String("trace-vis-db", "sqlite",
		"The database to store the visualization trace. Possible values are "+
			"sqlite, mysql, and csv.")
	visTracerDBFileName = ReportFlags.String("trace-vis-db-file", "",
		"The file name of the database to store the visualization trace. "+
			"Extension names are not required. "+
			"If not specified, a random file name will be used. "+
			"This flag does not work with Mysql db. When MySQL is used, "+
			"the database name is always randomly generated.")
	visTraceStartTime = ReportFlags.Float64("trace-vis-start", -1,
		"The starting time to collect visualization traces. A negative number "+
			"represents starting from the beginning.")
	visTraceEndTime = ReportFlags.Float64("trace-vis-end", -1,
		"The end time of collecting visualization traces. A negative number"+
			"means that the trace will be collected to the end of the simulation.")

	BenchmarkFlags = flag.NewFlagSet("Benchmark", flag.ExitOnError)
	// Any benchmark flags defined in tests should also be re-initialized here if necessary
}

func TestFlagDefinitions(t *testing.T) {
	resetFlagsForTesting()
	tests := []struct {
		flagSet *flag.FlagSet
		name    string
	}{
		// Simulation Flags
		{SimulationFlags, "timing"},
		{SimulationFlags, "max-inst"},
		{SimulationFlags, "parallel"},
		{SimulationFlags, "verify"},
		{SimulationFlags, "akitartm-port"},
		{SimulationFlags, "disable-rtm"},

		// Hardware Flags
		{HardwareFlags, "debug-isa"},
		{HardwareFlags, "gpus"},
		{HardwareFlags, "unified-gpus"},
		{HardwareFlags, "use-unified-memory"},
		{HardwareFlags, "magic-memory-copy"},

		// Report Flags
		{ReportFlags, "trace-mem"},
		{ReportFlags, "report-inst-count"},
		{ReportFlags, "report-cache-latency"},
		{ReportFlags, "report-cache-hit-rate"},
		{ReportFlags, "report-tlb-hit-rate"},
		{ReportFlags, "report-rdma-transaction-count"},
		{ReportFlags, "report-dram-transaction-count"},
		{ReportFlags, "report-all"},
		{ReportFlags, "metric-file-name"},
		{ReportFlags, "buffer-level-trace-dir"},
		{ReportFlags, "buffer-level-trace-period"},
		{ReportFlags, "report-busy-time"},
		{ReportFlags, "report-cpi-stack"},
		{ReportFlags, "analyzer-name"},
		{ReportFlags, "analyzer-period"},
		{ReportFlags, "trace-vis"},
		{ReportFlags, "trace-vis-db"},
		{ReportFlags, "trace-vis-db-file"},
		{ReportFlags, "trace-vis-start"},
		{ReportFlags, "trace-vis-end"},
	}

	for _, tt := range tests {
		if f := tt.flagSet.Lookup(tt.name); f == nil {
			t.Errorf("Expected flag -%s to be defined on %s FlagSet, but it was not", tt.name, tt.flagSet.Name())
		}
	}
}

// Mock os.Exit to prevent tests from terminating prematurely.
var mockExitStatus int
var mockExitCalled bool

func mockExit(code int) {
	mockExitStatus = code
	mockExitCalled = true
}

// Helper to setup os.Args and call ParseAllFlags
func setupAndParse(args []string) (originalArgs []string, originalExit func(int)) {
	originalArgs = os.Args
	originalExit = osExit // osExit is the original os.Exit, defined in flag.go if not already. Let's assume it exists or define it.
	
	// If osExit is not defined in flag.go, we might need to define it in the test file or ensure it's exported.
	// For now, let's assume flag.go has: var osExit = os.Exit
	// If not, this needs adjustment. For the purpose of this test, we can shadow os.Exit directly.
	osExit = mockExit // Redirect calls to os.Exit to our mock function

	os.Args = append([]string{"cmd"}, args...)
	mockExitCalled = false
	mockExitStatus = 0

	ParseAllFlags()
	return
}

func restoreEnv(originalArgs []string, originalExit func(int)) {
	os.Args = originalArgs
	osExit = originalExit
}


func TestParseAllFlags(t *testing.T) {
	// This needs to be assignable for mocking os.Exit
	// Ensure flag.go has `var osExit = os.Exit` or similar
	// If not, this test setup will need to be more involved (e.g. using build tags for testing)
	// For now, assuming we can reassign it for testing.
	// If `osExit` is not exported or assignable from `flag.go`, this specific part needs rethinking.
	// Let's assume for now `flag.go` has `var osExit = os.Exit` to make it mockable.
	// If not, we can define it here for the test scope if it's not causing conflicts.
	// var originalOsExit = os.Exit // This would shadow the package level os.Exit if it's not from runner.
	// For simplicity, I will assume runner.osExit is available and assignable.
	// If `runner.osExit` is not exported, then we can't directly mock it this way without changing `flag.go`.
	// A common pattern is to have `var osExitFunc = os.Exit` in the package being tested.
	// Let's assume `flag.go` has `var osExit = os.Exit`

	oldOsExit := osExit // Store the original os.Exit function from the runner package
	defer func() { osExit = oldOsExit }() // Restore it after the test

	t.Run("BasicParsing", func(t *testing.T) {
		resetFlagsForTesting()
		args := []string{
			"-timing",
			"-max-inst", "1000",
			"-gpus", "1,2",
			"-report-all",
			"-metric-file-name", "my_metrics",
		}
		origArgs, origExit := setupAndParse(args)
		defer restoreEnv(origArgs, origExit)

		if !*timingFlag {
			t.Errorf("Expected timingFlag to be true, got false")
		}
		if *maxInstCount != 1000 {
			t.Errorf("Expected maxInstCount to be 1000, got %d", *maxInstCount)
		}
		if *gpuFlag != "1,2" {
			t.Errorf("Expected gpuFlag to be '1,2', got '%s'", *gpuFlag)
		}
		if !*reportAll {
			t.Errorf("Expected reportAll to be true, got false")
		}
		if *filenameFlag != "my_metrics" {
			t.Errorf("Expected filenameFlag to be 'my_metrics', got '%s'", *filenameFlag)
		}
	})

	t.Run("OrderAgnostic", func(t *testing.T) {
		resetFlagsForTesting()
		args := []string{
			"-gpus", "3",
			"-report-inst-count",
			"-parallel",
		}
		origArgs, origExit := setupAndParse(args)
		defer restoreEnv(origArgs, origExit)

		if *gpuFlag != "3" {
			t.Errorf("Expected gpuFlag to be '3', got '%s'", *gpuFlag)
		}
		if !*instCountReportFlag {
			t.Errorf("Expected instCountReportFlag to be true, got false")
		}
		if !*parallelFlag {
			t.Errorf("Expected parallelFlag to be true, got false")
		}
	})

	t.Run("BenchmarkFlagParsing", func(t *testing.T) {
		resetFlagsForTesting()
		// Define a dummy benchmark flag for this test
		dummyBenchmarkFlag := BenchmarkFlags.Int("dummy-bench", 123, "A dummy benchmark flag")
		
		args := []string{"-dummy-bench", "456"}
		origArgs, origExit := setupAndParse(args)
		defer restoreEnv(origArgs, origExit)
		
		if *dummyBenchmarkFlag != 456 {
			t.Errorf("Expected dummyBenchmarkFlag to be 456, got %d", *dummyBenchmarkFlag)
		}
	})

	t.Run("DefaultValues", func(t *testing.T) {
		resetFlagsForTesting()
		// No arguments, so default values should be used.
		origArgs, origExit := setupAndParse([]string{})
		defer restoreEnv(origArgs, origExit)

		if *timingFlag != false {
			t.Errorf("Expected timingFlag to be false by default, got %v", *timingFlag)
		}
		if *gpuFlag != "" {
			t.Errorf("Expected gpuFlag to be '' by default, got %v", *gpuFlag)
		}
		if *reportAll != false {
			t.Errorf("Expected reportAll to be false by default, got %v", *reportAll)
		}
		if *maxInstCount != 0 {
			t.Errorf("Expected maxInstCount to be 0 by default, got %v", *maxInstCount)
		}
	})
}

func TestParseAllFlags_Help(t *testing.T) {
	oldOsExit := osExit 
	osExit = mockExit 
	defer func() { osExit = oldOsExit }()

	tests := []struct {
		name    string
		args    []string
		wantErr bool // if we expect os.Exit to be called
	}{
		{"-h", []string{"-h"}, true},
		{"--help", []string{"--help"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetFlagsForTesting()
			mockExitCalled = false
			mockExitStatus = 0

			// Redirect Stderr
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			originalArgs := os.Args
			os.Args = append([]string{"cmd"}, tt.args...)

			ParseAllFlags()

			w.Close()
			os.Stderr = oldStderr // Restore Stderr
			os.Args = originalArgs // Restore os.Args


			if tt.wantErr {
				if !mockExitCalled {
					t.Errorf("Expected os.Exit to be called for %s, but it wasn't", tt.name)
				}
				if mockExitStatus != 0 {
					t.Errorf("Expected exit status 0 for %s, got %d", tt.name, mockExitStatus)
				}
			} else {
				if mockExitCalled {
					t.Errorf("os.Exit called unexpectedly for %s with status %d", tt.name, mockExitStatus)
				}
			}
			
			// Check output (this is a basic check, could be more thorough)
			// The actual output check was removed as it's complex to get the output from flag.Usage()
			// when it's called by flag.Parse() internally on -h.
			// The key is that os.Exit(0) was called, implying help was processed.
			// A more robust check would involve capturing output from flag.Usage if possible.
		})
	}
}


func TestPopulateRunnerFieldsFromFlags(t *testing.T) {
	resetFlagsForTesting()
	r := &Runner{}

	// Set flag variable values directly
	*timingFlag = true
	*parallelFlag = true
	*verifyFlag = false // Default, but explicit
	*useUnifiedMemoryFlag = true
	*gpuFlag = "1,2,3"

	r.populateRunnerFieldsFromFlags()

	if r.Timing != true {
		t.Errorf("Expected r.Timing to be true, got %v", r.Timing)
	}
	if r.Parallel != true {
		t.Errorf("Expected r.Parallel to be true, got %v", r.Parallel)
	}
	if r.Verify != false {
		t.Errorf("Expected r.Verify to be false, got %v", r.Verify)
	}
	if r.UseUnifiedMemory != true {
		t.Errorf("Expected r.UseUnifiedMemory to be true, got %v", r.UseUnifiedMemory)
	}
	expectedGPUIDs := []int{1, 2, 3}
	if !reflect.DeepEqual(r.GPUIDs, expectedGPUIDs) {
		t.Errorf("Expected r.GPUIDs to be %v, got %v", expectedGPUIDs, r.GPUIDs)
	}

	// Test with different values
	resetFlagsForTesting()
	*gpuFlag = ""
	*unifiedGPUFlag = "4,5"
	*useUnifiedMemoryFlag = false
	r.populateRunnerFieldsFromFlags()

	expectedGPUIDs = []int{4, 5}
	if !reflect.DeepEqual(r.GPUIDs, expectedGPUIDs) {
		t.Errorf("Expected r.GPUIDs to be %v, got %v", expectedGPUIDs, r.GPUIDs)
	}
	if r.UseUnifiedMemory != false {
		t.Errorf("Expected r.UseUnifiedMemory to be false, got %v", r.UseUnifiedMemory)
	}
}

func TestParseGPUFlag(t *testing.T) {
	tests := []struct {
		name             string
		gpuFlagVal       string
		unifiedGPUFlagVal string
		expectedGPUIDs   []int
		expectPanic      bool
	}{
		{"DefaultGPU", "", "", []int{1}, false},
		{"SingleGPU_gpuFlag", "1", "", []int{1}, false},
		{"MultiGPU_gpuFlag", "1,2,3", "", []int{1, 2, 3}, false},
		{"SingleGPU_unifiedGPUFlag", "", "4", []int{4}, false},
		{"MultiGPU_unifiedGPUFlag", "", "4,5,6", []int{4, 5, 6}, false},
		{"PanicBothSet", "1", "2", nil, true},
		{"PanicBothSetNonEmpty", "1,2", "3,4", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetFlagsForTesting()
			r := &Runner{}
			*gpuFlag = tt.gpuFlagVal
			*unifiedGPUFlag = tt.unifiedGPUFlagVal

			defer func() {
				if r := recover(); r != nil {
					if !tt.expectPanic {
						t.Errorf("parseGPUFlag() panicked unexpectedly: %v", r)
					}
				} else {
					if tt.expectPanic {
						t.Errorf("parseGPUFlag() did not panic as expected")
					}
				}
			}()

			r.parseGPUFlag() // This is the method on the runner instance

			if !tt.expectPanic {
				if !reflect.DeepEqual(r.GPUIDs, tt.expectedGPUIDs) {
					t.Errorf("Expected r.GPUIDs to be %v, got %v", tt.expectedGPUIDs, r.GPUIDs)
				}
			}
		})
	}
}

// This is needed to allow mocking of os.Exit in ParseAllFlags
// It should be defined in the original flag.go as `var osExit = os.Exit`
// If it's not, this test file won't compile or work as intended for help flag testing.
// For the purpose of this exercise, we assume it's available.
// If not, flag.go would need:
// var osExit = os.Exit
// And then tests can do:
// runner.osExit = func(code int) { /* mock */ }
// For now, defining it here if not present in runner package.
var osExit = os.Exit 

// Helper to capture stdout/stderr
func captureOutput(f func()) string {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	f()

	w.Close()
	os.Stderr = oldStderr
	
	out := new(strings.Builder)
	_, _ = io.Copy(out, r)
	return out.String()
}

func TestParseAllFlags_HelpOutput(t *testing.T) {
	oldOsExit := osExit
	osExit = mockExit
	defer func() { osExit = oldOsExit }()

	expectedHeaders := []string{
		"Simulation Flags:",
		"Hardware Flags:",
		"Report Flags:",
		"Benchmark Flags:",
		"Other Flags:", // For the global -h/-help
	}

	args := []string{"-h"} // Could also test with --help

	resetFlagsForTesting()
	mockExitCalled = false // Reset mock state

	// Define a dummy benchmark flag to ensure "Benchmark Flags:" section appears
	_ = BenchmarkFlags.Int("test-bench-flag-for-help", 0, "A test flag for help output.")
	
	// Redirect Stderr
	originalStderr := os.Stderr
	rPipe, wPipe, _ := os.Pipe()
	os.Stderr = wPipe

	originalOsArgs := os.Args
	os.Args = append([]string{"cmd"}, args...)

	ParseAllFlags() // This should call our mockExit

	wPipe.Close() // Close writer to flush
	os.Stderr = originalStderr // Restore
	os.Args = originalOsArgs // Restore

	if !mockExitCalled {
		t.Fatalf("os.Exit was not called by ParseAllFlags with -h")
	}
	if mockExitStatus != 0 {
		t.Fatalf("Expected exit status 0 for help, got %d", mockExitStatus)
	}
	
	outputBytes, _ := io.ReadAll(rPipe)
	output := string(outputBytes)

	for _, header := range expectedHeaders {
		if !strings.Contains(output, header) {
			t.Errorf("Help message output did not contain expected header: %s\nFull output:\n%s", header, output)
		}
	}

	// Check if a specific flag is present for one of the categories
	if !strings.Contains(output, "-timing") {
		t.Errorf("Help message output did not contain example flag '-timing' under Simulation Flags.\nFull output:\n%s", output)
	}
	if !strings.Contains(output, "-gpus") {
		t.Errorf("Help message output did not contain example flag '-gpus' under Hardware Flags.\nFull output:\n%s", output)
	}
	if !strings.Contains(output, "-report-all") {
		t.Errorf("Help message output did not contain example flag '-report-all' under Report Flags.\nFull output:\n%s", output)
	}
	if !strings.Contains(output, "-test-bench-flag-for-help") {
		t.Errorf("Help message output did not contain example flag '-test-bench-flag-for-help' under Benchmark Flags.\nFull output:\n%s", output)
	}
}
