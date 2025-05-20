package runner

import (
	"flag"
	"strconv"
	"strings"
)

var timingFlag = flag.Bool("timing", false, "Run detailed timing simulation.")
var maxInstCount = flag.Uint64("max-inst", 0,
	"Terminate the simulation after the given number of instructions is retired.")
var parallelFlag = flag.Bool("parallel", false,
	"Run the simulation in parallel.")
var isaDebug = flag.Bool("debug-isa", false, "Generate the ISA debugging file.")

var verifyFlag = flag.Bool("verify", false, "Verify the emulation result.")
var memTracing = flag.Bool("trace-mem", false, "Generate memory trace")
var instCountReportFlag = flag.Bool("report-inst-count", false,
	"Report the number of instructions executed in each compute unit.")
var cacheLatencyReportFlag = flag.Bool("report-cache-latency", false,
	"Report the average cache latency.")
var cacheHitRateReportFlag = flag.Bool("report-cache-hit-rate", false,
	"Report the cache hit rate of each cache.")
var tlbHitRateReportFlag = flag.Bool("report-tlb-hit-rate", false,
	"Report the TLB hit rate of each TLB.")
var rdmaTransactionCountReportFlag = flag.Bool("report-rdma-transaction-count",
	false, "Report the number of transactions going through the RDMA engines.")
var dramTransactionCountReportFlag = flag.Bool("report-dram-transaction-count",
	false, "Report the number of transactions accessing the DRAMs.")
var gpuFlag = flag.String("gpus", "",
	"The GPUs to use, use a format like 1,2,3,4. By default, GPU 1 is used.")
var unifiedGPUFlag = flag.String("unified-gpus", "",
	`Run multi-GPU benchmark in a unified mode.
Use a format like 1,2,3,4. Cannot coexist with -gpus.`)
var useUnifiedMemoryFlag = flag.Bool("use-unified-memory", false,
	"Run benchmark with Unified Memory or not")
var reportAll = flag.Bool("report-all", false, "Report all metrics to .csv file.")
var filenameFlag = flag.String("metric-file-name", "metrics",
	"Modify the name of the output csv file.")
var magicMemoryCopy = flag.Bool("magic-memory-copy", false,
	"Copy data from CPU directly to global memory")
var bufferLevelTraceDirFlag = flag.String("buffer-level-trace-dir", "",
	"The directory to dump the buffer level traces.")
var bufferLevelTracePeriodFlag = flag.Float64("buffer-level-trace-period", 0.0,
	"The period to dump the buffer level trace.")
var simdBusyTimeTracerFlag = flag.Bool("report-busy-time", false, "Report SIMD Unit's busy time")
var reportCPIStackFlag = flag.Bool("report-cpi-stack", false, "Report CPI stack")
var customPortForAkitaRTM = flag.Int("akitartm-port", 0,
	`Custom port to host AkitaRTM. A 4-digit or 5-digit port number is required. If 
this number is not given or a invalid number is given number, a random port 
will be used.`)
var disableAkitaRTM = flag.Bool("disable-rtm", false, "Disable the AkitaRTM monitoring portal")

var analyzerNameFlag = flag.String("analyzer-name", "",
	"The name of the analyzer to use.")

var analyzerPeriodFlag = flag.Float64("analyzer-period", 0.0,
	"The period to dump the analyzer results.")

var visTracing = flag.Bool("trace-vis", false,
	"Generate trace for visualization purposes.")
var visTracerDB = flag.String("trace-vis-db", "sqlite",
	"The database to store the visualization trace. Possible values are "+
		"sqlite, mysql, and csv.")
var visTracerDBFileName = flag.String("trace-vis-db-file", "",
	"The file name of the database to store the visualization trace. "+
		"Extension names are not required. "+
		"If not specified, a random file name will be used. "+
		"This flag does not work with Mysql db. When MySQL is used, "+
		"the database name is always randomly generated.")
var visTraceStartTime = flag.Float64("trace-vis-start", -1,
	"The starting time to collect visualization traces. A negative number "+
		"represents starting from the beginning.")
var visTraceEndTime = flag.Float64("trace-vis-end", -1,
	"The end time of collecting visualization traces. A negative number"+
		"means that the trace will be collected to the end of the simulation.")

// parseFlag applies the runner flag to runner object
func (r *Runner) parseFlag() *Runner {
	r.parseSimulationFlags()
	r.parseGPUFlag()

	return r
}

func (r *Runner) parseSimulationFlags() {
	if *parallelFlag {
		r.Parallel = true
	}

	if *verifyFlag {
		r.Verify = true
	}

	if *timingFlag {
		r.Timing = true
	}

	if *useUnifiedMemoryFlag {
		r.UseUnifiedMemory = true
	}
}

func (r *Runner) parseGPUFlag() {
	if *gpuFlag == "" && *unifiedGPUFlag == "" {
		r.GPUIDs = []int{1}
		return
	}

	if *gpuFlag != "" && *unifiedGPUFlag != "" {
		panic("cannot use -gpus and -unified-gpus together")
	}

	var gpuIDs []int
	if *gpuFlag != "" {
		gpuIDs = r.gpuIDStringToList(*gpuFlag)
	} else if *unifiedGPUFlag != "" {
		gpuIDs = r.gpuIDStringToList(*unifiedGPUFlag)
	}

	r.GPUIDs = gpuIDs
}

func (r *Runner) gpuIDStringToList(gpuIDsString string) []int {
	gpuIDs := make([]int, 0)
	gpuIDTokens := strings.Split(gpuIDsString, ",")

	for _, t := range gpuIDTokens {
		gpuID, err := strconv.Atoi(t)
		if err != nil {
			panic(err)
		}
		gpuIDs = append(gpuIDs, gpuID)
	}

	return gpuIDs
}
