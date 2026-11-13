package main

// Page migration between GPUs (sarchlab/akita#588). Every case runs the timing
// simulation with unified memory on separate GPUs, so the GPUs share pages
// that migrate on faults, the default of the platform. A case passes when
// -verify passes and no queue stalls: a fault that is lost leaves its access
// waiting forever, which shows up as a stalled queue.
//
// The cases cover R9 Nano and MI300X on 2 GPUs and R9 Nano on 4 GPUs, and
// sizes that touch many shared pages at once:
//   - atax 1024x1024 on 2 GPUs starts a kernel on more than 512 cold remote
//     pages, more faults than one batch of the driver takes;
//   - matrixmultiplication 512^3 on 4 GPUs lets several GPUs fault on pages
//     that are already migrating.
//
// On MI300X the workload is atax, which runs on the last GPU over memory
// that starts on the first; fir does not split its work between MI300X GPUs,
// with or without page migration.
//
// The policy and its parameters are part of the system specification, not of
// the command line; the access-counter policy, throttling, pins, and
// checkpoints are exercised by the tests in amd/tests/pagemigration.
var pageMigrationBenchmarks = []benchmark{
	{
		benchmarkPath:  "../../benchmarks/heteromark/fir",
		executablePath: "../../samples/fir",
		executable:     "fir",
		sizeArgs:       []string{"-length=8192"},
		cases: []benchmarkCase{
			pageMigrationCase([]int{1, 2}),
			pageMigrationCase([]int{1, 2, 3, 4}),
		},
	},
	{
		benchmarkPath:  "../../benchmarks/polybench/atax",
		executablePath: "../../samples/atax",
		executable:     "atax",
		sizeArgs:       []string{"-x=1024", "-y=1024"},
		cases:          []benchmarkCase{pageMigrationCase([]int{1, 2})},
	},
	{
		benchmarkPath:  "../../benchmarks/polybench/atax",
		executablePath: "../../samples/atax",
		executable:     "atax",
		sizeArgs:       []string{"-x=256", "-y=256"},
		cases:          []benchmarkCase{mi300xPageMigrationCase([]int{1, 2})},
	},
	{
		benchmarkPath:  "../../benchmarks/amdappsdk/matrixmultiplication",
		executablePath: "../../samples/matrixmultiplication",
		executable:     "matrixmultiplication",
		sizeArgs:       []string{"-x=512", "-y=512", "-z=512"},
		cases:          []benchmarkCase{pageMigrationCase([]int{1, 2, 3, 4})},
	},
}

func pageMigrationCase(gpus []int) benchmarkCase {
	return benchmarkCase{
		gpus:          gpus,
		timing:        true,
		unifiedMemory: true,
		pageMigration: true,
	}
}

func mi300xPageMigrationCase(gpus []int) benchmarkCase {
	c := pageMigrationCase(gpus)
	c.arch = "cdna3"
	c.gpuType = "mi300x"

	return c
}

func init() {
	benchmarks = append(benchmarks, pageMigrationBenchmarks...)
}
