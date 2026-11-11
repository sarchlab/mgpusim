package main

import "fmt"

// Page migration between GPUs (sarchlab/akita#588). Every case runs the timing
// simulation with unified memory on separate GPUs, so the GPUs share pages
// that migrate. A case passes when -verify passes and no queue stalls: a
// faulted or held access that is never released shows up as a stalled queue.
//
// The suite is generated, not listed (sarchlab/mgpusim#343): the dimensions
// below are combined into a pairwise covering array, so every value pair of
// every two dimensions runs at least once without running the full product.
// A few regression cases that must always run are listed explicitly.

// pageMigrationWorkloads are the benchmarks the generated cases draw from,
// each sized so many shared pages are touched at once.
var pageMigrationWorkloads = map[string]benchmark{
	// Streaming access over shared buffers.
	"fir": {
		benchmarkPath:  "../../benchmarks/heteromark/fir",
		executablePath: "../../samples/fir",
		executable:     "fir",
		sizeArgs:       []string{"-length=8192"},
	},
	// A kernel starts on more than 512 cold remote pages: a burst of faults.
	"atax": {
		benchmarkPath:  "../../benchmarks/polybench/atax",
		executablePath: "../../samples/atax",
		executable:     "atax",
		sizeArgs:       []string{"-x=1024", "-y=1024"},
	},
	// Every GPU reads all of B: several GPUs fault on the same units, and
	// units move back and forth.
	"matrixmultiplication": {
		benchmarkPath:  "../../benchmarks/amdappsdk/matrixmultiplication",
		executablePath: "../../samples/matrixmultiplication",
		executable:     "matrixmultiplication",
		sizeArgs:       []string{"-x=512", "-y=512", "-z=512"},
	},
}

var pageMigrationDimensions = []dimension{
	{"workload", []string{"fir", "atax", "matrixmultiplication"}},
	{"gpu", []string{"r9nano", "mi300x"}},
	{"gpus", []string{"2", "4"}},
	{"policy", []string{"fault", "fault-no-prefetch", "counter"}},
	{"unit", []string{"4KB", "64KB"}},
	{"stress", []string{"none", "thrashing"}},
}

// pageMigrationValid rejects configurations that cannot run or cost too much
// for CI.
func pageMigrationValid(c config) bool {
	d := pageMigrationDimensions

	// Four MI300X GPUs (4 x 304 CUs) are too slow for CI.
	if c.value(d, "gpu") == "mi300x" && c.value(d, "gpus") == "4" {
		return false
	}

	// On MI300X, only atax: fir on two or more CDNA3 GPUs leaves the second
	// GPU's output at 0 even without page migration (the second GPU seems to
	// ignore the global offset), and matrixmultiplication 512^3 on two MI300X
	// GPUs does not finish within 40 minutes.
	if c.value(d, "gpu") == "mi300x" && c.value(d, "workload") != "atax" {
		return false
	}

	return true
}

// pageMigrationPolicyArgs are the -page-migration value and the extra flags
// for each policy.
var pageMigrationPolicyArgs = map[string]struct {
	policy string
	args   []string
}{
	"fault":             {"fault", nil},
	"fault-no-prefetch": {"fault", []string{"-pm-prefetch-threshold=0"}},
	// A low threshold so short runs migrate.
	"counter": {"counter", []string{"-pm-access-counter-threshold=16"}},
}

var pageMigrationStressArgs = map[string][]string{
	"none": nil,
	// Every migration within 100 ms of the previous one is a thrashing event,
	// one event pins the unit, and the pin expires after 50 us, so pinning,
	// remote access to pinned units, and unpinning all run.
	"thrashing": {
		"-pm-thrashing-threshold=1",
		"-pm-thrashing-lapse-ns=100000000",
		"-pm-pin-ns=50000",
	},
}

// pageMigrationCaseFor turns a configuration into a case.
func pageMigrationCaseFor(c config) benchmarkCase {
	d := pageMigrationDimensions
	p := pageMigrationPolicyArgs[c.value(d, "policy")]

	args := append([]string{}, p.args...)
	if c.value(d, "unit") == "64KB" {
		args = append(args, "-pm-unit-kb=64")
	}

	args = append(args, pageMigrationStressArgs[c.value(d, "stress")]...)

	bc := pageMigrationCase(gpuList(c.value(d, "gpus")), p.policy, args...)
	if c.value(d, "gpu") == "mi300x" {
		bc.arch = "cdna3"
		bc.gpuType = "mi300x"
	}

	return bc
}

// pageMigrationRegressionCases always run, on top of the generated ones.
var pageMigrationRegressionCases = map[string][]benchmarkCase{
	// A burst of cold faults on two GPUs; faults used to be dropped when both
	// batches were full.
	"atax": {pageMigrationCase([]int{1, 2}, "fault")},
	// Four GPUs fault on units that are already migrating; those faults used
	// to be ignored and their accesses never released.
	"matrixmultiplication": {pageMigrationCase([]int{1, 2, 3, 4}, "fault")},
}

// pageMigrationBenchmarks builds the suite: the generated cases plus the
// regression cases, grouped by workload in a fixed order.
func pageMigrationBenchmarks() []benchmark {
	cases := map[string][]benchmarkCase{}

	for _, c := range pairwise(pageMigrationDimensions, pageMigrationValid) {
		w := c.value(pageMigrationDimensions, "workload")
		cases[w] = append(cases[w], pageMigrationCaseFor(c))
	}

	out := []benchmark{}

	for _, w := range pageMigrationDimensions[0].values {
		b := pageMigrationWorkloads[w]
		b.cases = withoutDuplicates(b, append(
			append([]benchmarkCase{}, pageMigrationRegressionCases[w]...),
			cases[w]...))
		out = append(out, b)
	}

	return out
}

// withoutDuplicates drops cases that run the same command as an earlier one.
func withoutDuplicates(b benchmark, cases []benchmarkCase) []benchmarkCase {
	seen := map[string]bool{}
	out := []benchmarkCase{}

	for _, c := range cases {
		key := fmt.Sprint(b.populateArgs(c))
		if !seen[key] {
			seen[key] = true
			out = append(out, c)
		}
	}

	return out
}

func gpuList(n string) []int {
	switch n {
	case "2":
		return []int{1, 2}
	case "4":
		return []int{1, 2, 3, 4}
	default:
		panic(fmt.Sprintf("acceptance: unsupported GPU count %q", n))
	}
}

func pageMigrationCase(
	gpus []int,
	policy string,
	pmArgs ...string,
) benchmarkCase {
	return benchmarkCase{
		gpus:          gpus,
		timing:        true,
		unifiedMemory: true,
		pageMigration: policy,
		pmArgs:        pmArgs,
	}
}

func init() {
	benchmarks = append(benchmarks, pageMigrationBenchmarks()...)
}
