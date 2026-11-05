package main

// Page migration between GPUs (sarchlab/akita#588). Every case runs the timing
// simulation with unified memory on separate GPUs, so the GPUs share pages
// that migrate. A case passes when -verify passes and no queue stalls: a
// faulted or held access that is never released shows up as a stalled queue.
//
// The cases cover the three policies (fault with and without prefetching,
// access counters) on 2 and 4 GPUs, and sizes that touch many shared pages at
// once:
//   - atax 1024x1024 on 2 GPUs starts a kernel on more than 512 cold remote
//     pages, which fills both fault batches;
//   - matrixmultiplication 512^3 on 4 GPUs with a batch large enough that no
//     fault is dropped, so several GPUs fault on units that are already
//     migrating.
var pageMigrationBenchmarks = []benchmark{
	{
		benchmarkPath:  "../../benchmarks/heteromark/fir",
		executablePath: "../../samples/fir",
		executable:     "fir",
		sizeArgs:       []string{"-length=8192"},
		cases:          pageMigrationPolicyCases([][]int{{1, 2}, {1, 2, 3, 4}}),
	},
	{
		benchmarkPath:  "../../benchmarks/polybench/atax",
		executablePath: "../../samples/atax",
		executable:     "atax",
		sizeArgs:       []string{"-x=1024", "-y=1024"},
		cases:          pageMigrationPolicyCases([][]int{{1, 2}}),
	},
	{
		benchmarkPath:  "../../benchmarks/amdappsdk/matrixmultiplication",
		executablePath: "../../samples/matrixmultiplication",
		executable:     "matrixmultiplication",
		sizeArgs:       []string{"-x=512", "-y=512", "-z=512"},
		cases: []benchmarkCase{
			pageMigrationCase([]int{1, 2, 3, 4}, "fault", "-pm-batch=1000000"),
		},
	},
}

// pageMigrationPolicyCases returns, for each GPU set, a case for faults with
// prefetching (the default), faults without prefetching, and access counters
// with a threshold low enough that short runs migrate.
func pageMigrationPolicyCases(gpuSets [][]int) []benchmarkCase {
	cases := []benchmarkCase{}

	for _, gpus := range gpuSets {
		cases = append(cases,
			pageMigrationCase(gpus, "fault"),
			pageMigrationCase(gpus, "fault", "-pm-prefetch-threshold=0"),
			pageMigrationCase(gpus, "counter", "-pm-access-counter-threshold=16"),
		)
	}

	return cases
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
	benchmarks = append(benchmarks, pageMigrationBenchmarks...)
}
