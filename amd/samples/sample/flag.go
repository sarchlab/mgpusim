package sample

import (
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/waferscale"
)

var timingFlag = flag.Bool("timing", false, "Run detailed timing simulation.")
var maxInstCount = flag.Uint64("max-inst", 0,
	"Terminate the simulation after the given number of instructions is retired.")
var parallelFlag = flag.Bool("parallel", false,
	"Run the simulation in parallel.")
var isaDebug = flag.Bool("debug-isa", false, "Generate the ISA debugging file.")
var archFlag = flag.String("arch", "gcn3", "GPU architecture: gcn3 or cdna3.")
var mi300xClockMHz = flag.Uint64("mi300x-clock-mhz", 0,
	"MI300X timing clock in MHz (0 uses the GPU preset).")
var gpuTypeFlag = flag.String("gpu", "r9nano",
	`GPU model for timing simulation: r9nano, mi300x, or waferscale.
waferscale is a mesh of R9 Nano-based tiles with the host in the centre (see
-wafer-mesh); -gpus picks the tiles a benchmark runs on.`)
var waferMeshFlag = flag.String("wafer-mesh", "5x5",
	`Mesh shape of a waferscale GPU as WIDTHxHEIGHT. The host takes the centre
position, so a WxH mesh has W*H-1 GPU tiles (5x5: 24, 7x7: 48).`)

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
	`The GPUs to use, as IDs and ranges such as 1,3-5,8, or all for every tile
of a waferscale GPU. By default, GPU 1 is used.`)
var unifiedGPUFlag = flag.String("unified-gpus", "",
	`Run multi-GPU benchmark in a unified mode.
Use a format like -gpus, such as 1-4 or all. Cannot coexist with -gpus.`)
var useUnifiedMemoryFlag = flag.Bool("use-unified-memory", false,
	"Run benchmark with Unified Memory or not")
var reportKernelDispatches = flag.Bool("report-kernel-dispatches", false,
	"Record per-dispatch command-processor times and launch metadata in SQLite.")
var reportAll = flag.Bool("report-all", false, "Report all metrics to .csv file.")
var filenameFlag = flag.String("metric-file-name", "metrics",
	"Modify the name of the output csv file.")
var magicMemoryCopy = flag.Bool("magic-memory-copy", false,
	`Copy data between the CPU and the global memory directly, in no time and
around the GPUs. With -timing the caches are not flushed for it, so it does not
see what a kernel wrote; see docs/page-migration.md.`)
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

// metricFileNameFlagIsSet tells whether the user explicitly passed
// -metric-file-name on the command line.
func metricFileNameFlagIsSet() bool {
	isSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "metric-file-name" {
			isSet = true
		}
	})

	return isSet
}

// parseFlag applies the runner flag to runner object
func (r *Config) parseFlag() *Config {
	r.parseSimulationFlags()
	r.parseGPUFlag()

	return r
}

func (r *Config) parseSimulationFlags() {
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

	r.ArchType = parseArchFlag()
	r.GPUType = parseGPUTypeFlag()
	if *mi300xClockMHz > 1_000_000 {
		panic("-mi300x-clock-mhz exceeds the simulator's 1 ps clock resolution")
	}
	r.MI300XFreq = timing.Freq(*mi300xClockMHz) * timing.MHz
	if r.MI300XFreq != 0 && (!r.Timing || r.GPUType != "mi300x") {
		panic("-mi300x-clock-mhz requires -timing and -gpu=mi300x")
	}

	width, height, err := parseMeshShape(*waferMeshFlag)
	if err != nil {
		panic(err)
	}
	r.WaferMeshWidth, r.WaferMeshHeight = width, height
}

// parseMeshShape parses a mesh shape written as WIDTHxHEIGHT, such as 7x7.
func parseMeshShape(shape string) (width, height int, err error) {
	w, h, found := strings.Cut(strings.ToLower(shape), "x")
	if found {
		width, err = strconv.Atoi(w)
	}
	if found && err == nil {
		height, err = strconv.Atoi(h)
	}
	if !found || err != nil || width < 1 || height < 1 {
		return 0, 0, fmt.Errorf("invalid -wafer-mesh %q: want WIDTHxHEIGHT, such as 7x7", shape)
	}

	return width, height, nil
}

func (r *Config) parseGPUFlag() {
	if *gpuFlag == "" && *unifiedGPUFlag == "" {
		r.GPUIDs = []int{1}
		return
	}

	if *gpuFlag != "" && *unifiedGPUFlag != "" {
		panic("cannot use -gpus and -unified-gpus together")
	}

	list := *gpuFlag
	if list == "" {
		list = *unifiedGPUFlag
	}

	// "all" names every tile of a wafer; other GPU types have no fixed
	// number of GPUs.
	numAll := 0
	if _, ok := waferscale.ParseGPUType(r.GPUType); ok {
		numAll = r.waferMesh().NumTiles()
	}

	gpuIDs, err := parseGPUList(list, numAll)
	if err != nil {
		panic(err)
	}

	r.GPUIDs = gpuIDs
}

// parseGPUList parses a list of GPU IDs, numbered from 1, the way a print
// dialog takes pages: IDs and ranges separated by commas, such as "1,3-5,8",
// or "all" for GPUs 1 to numAll. numAll is 0 when the GPU type has no fixed
// number of GPUs; then "all" is an error and IDs have no upper bound.
func parseGPUList(list string, numAll int) ([]int, error) {
	list = strings.TrimSpace(list)
	if strings.EqualFold(list, "all") {
		if numAll == 0 {
			return nil, errors.New(`"all" needs -gpu waferscale; list the GPUs instead, such as 1-4`)
		}

		ids := make([]int, numAll)
		for i := range ids {
			ids[i] = i + 1
		}

		return ids, nil
	}

	var ids []int
	seen := make(map[int]bool)
	for _, item := range strings.Split(list, ",") {
		first, last, err := parseGPURange(strings.TrimSpace(item))
		if err != nil {
			return nil, fmt.Errorf("invalid GPU list %q: %w", list, err)
		}
		if numAll != 0 && last > numAll {
			return nil, fmt.Errorf("invalid GPU list %q: the wafer has GPUs 1-%d", list, numAll)
		}

		for id := first; id <= last; id++ {
			if seen[id] {
				return nil, fmt.Errorf("invalid GPU list %q: GPU %d appears twice", list, id)
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}

	return ids, nil
}

// parseGPURange parses one item of a GPU list: an ID such as "3" or a range
// such as "1-4".
func parseGPURange(item string) (first, last int, err error) {
	low, high, isRange := strings.Cut(item, "-")
	first, err = strconv.Atoi(low)
	last = first
	if err == nil && isRange {
		last, err = strconv.Atoi(high)
	}
	if err != nil || first < 1 || last < first {
		return 0, 0, fmt.Errorf("%q is neither a GPU ID nor a range such as 1-4", item)
	}

	return first, last, nil
}

func parseArchFlag() arch.Type {
	switch strings.ToLower(*archFlag) {
	case "cdna3", "gfx942":
		return arch.CDNA3
	default:
		return arch.GCN3
	}
}

func parseGPUTypeFlag() string {
	return strings.ToLower(*gpuTypeFlag)
}
