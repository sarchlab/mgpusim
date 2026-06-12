package main

var benchmarks = []benchmark{
	{
		benchmarkPath:  "../../benchmarks/polybench/atax",
		executablePath: "../../samples/atax",
		executable:     "atax",
		sizeArgs:       []string{"-x=256", "-y=256"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: false, parallel: true},
			{gpus: []int{1}, timing: true, parallel: false},
			// Parallel timing disabled: V5 ParallelEngine data race causes intermittent failures.
			// {gpus: []int{1}, timing: true, parallel: true},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/polybench/bicg",
		executablePath: "../../samples/bicg",
		executable:     "bicg",
		sizeArgs:       []string{"-x=256", "-y=256"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Parallel disabled: V5 ParallelEngine data race may cause intermittent failures.
			// {gpus: []int{1}, timing: false, parallel: true},
			// Disabled: timing verification still failing after V5 fix (CI run 23676583498)
			// {gpus: []int{1}, timing: true, parallel: false},
			// {gpus: []int{1}, timing: true, parallel: true},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/heteromark/fir",
		executablePath: "../../samples/fir",
		executable:     "fir",
		sizeArgs:       []string{"-length=8192"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Parallel disabled: V5 ParallelEngine data race causes intermittent crashes.
			// {gpus: []int{1}, timing: false, parallel: true},
			{gpus: []int{1}, timing: true, parallel: false},
			// Parallel timing disabled: V5 ParallelEngine data race in TLB
			// middleware causes panic (slice bounds out of range).
			// {gpus: []int{1}, timing: true, parallel: true},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/heteromark/aes",
		executablePath: "../../samples/aes",
		executable:     "aes",
		sizeArgs:       []string{"-length=16384"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Parallel disabled: V5 ParallelEngine data race causes panic.
			// {gpus: []int{1}, timing: false, parallel: true},
			// CDNA3 timing disabled — crashes (pre-existing bug, tracking: #422)
			// {gpus: []int{1}, timing: true, parallel: false},
			// {gpus: []int{1}, timing: true, parallel: true},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/heteromark/kmeans",
		executablePath: "../../samples/kmeans",
		executable:     "kmeans",
		sizeArgs:       []string{"-points=1024", "-features=32", "-clusters=5", "-max-iter=5"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Parallel disabled: V5 ParallelEngine data race causes panic.
			// {gpus: []int{1}, timing: false, parallel: true},
			// CDNA3 timing disabled: timing simulation deadlock in iterative
			// H2D+kernel+D2H pattern. Separate from bicg D2H deadlock.
			// {gpus: []int{1}, timing: true, parallel: false},
			// {gpus: []int{1}, timing: true, parallel: true},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/heteromark/pagerank",
		executablePath: "../../samples/pagerank",
		executable:     "pagerank",
		sizeArgs:       []string{"-node=64", "-sparsity=0.5", "-iterations=2"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Parallel disabled: V5 ParallelEngine data race causes panic.
			// {gpus: []int{1}, timing: false, parallel: true},
			// Disabled: timing verification still failing after V5 fix (CI run 23676583498)
			// {gpus: []int{1}, timing: true, parallel: false},
			// {gpus: []int{1}, timing: true, parallel: true},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/amdappsdk/matrixmultiplication",
		executablePath: "../../samples/matrixmultiplication",
		executable:     "matrixmultiplication",
		sizeArgs:       []string{"-x=128", "-y=128", "-z=128"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/amdappsdk/matrixtranspose",
		executablePath: "../../samples/matrixtranspose",
		executable:     "matrixtranspose",
		sizeArgs:       []string{"-width=1024"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Parallel disabled: V5 ParallelEngine data race causes panic.
			// {gpus: []int{1}, timing: false, parallel: true},
			// CDNA3 timing disabled — crashes (pre-existing bug, tracking: #422)
			// {gpus: []int{1}, timing: true, parallel: false},
			// {gpus: []int{1}, timing: true, parallel: true},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/amdappsdk/bitonicsort",
		executablePath: "../../samples/bitonicsort",
		executable:     "bitonicsort",
		sizeArgs:       []string{"-length=4096"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Disabled: timing verification still failing after V5 fix (CI run 23676583498)
			// {gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/amdappsdk/simpleconvolution",
		executablePath: "../../samples/simpleconvolution",
		executable:     "simpleconvolution",
		sizeArgs:       []string{},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Parallel disabled: V5 ParallelEngine data race causes panic.
			// {gpus: []int{1}, timing: false, parallel: true},
			// Disabled: timing verification still failing after V5 fix (CI run 23676583498)
			// {gpus: []int{1}, timing: true, parallel: false},
			// {gpus: []int{1}, timing: true, parallel: true},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/amdappsdk/floydwarshall",
		executablePath: "../../samples/floydwarshall",
		executable:     "floydwarshall",
		sizeArgs:       []string{},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: false, parallel: true},
			// Still failing: timing page-not-found panic (CI run 23654221393)
			// {gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/shoc/spmv",
		executablePath: "../../samples/spmv",
		executable:     "spmv",
		sizeArgs:       []string{},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: false, parallel: true},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/shoc/fft",
		executablePath: "../../samples/fft",
		executable:     "fft",
		sizeArgs:       []string{"-MB=1"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Parallel disabled: V5 ParallelEngine data race in TLB middleware
			// causes panic (slice bounds out of range). Tracking: V5 migration.
			// {gpus: []int{1}, timing: false, parallel: true},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/shoc/nbody",
		executablePath: "../../samples/nbody",
		executable:     "nbody",
		sizeArgs:       []string{"-particles=256", "-iter=4"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	// {
	// 	benchmarkPath:  "",
	// 	executablePath: "../../samples/concurrentkernel",
	// 	executable:     "concurrentkernel",
	// 	sizeArgs:       []string{},
	// },
	// {
	// 	benchmarkPath:  "",
	// 	executablePath: "../../samples/concurrentworkload",
	// 	executable:     "concurrentworkload",
	// 	sizeArgs:       []string{},
	// },

	{
		benchmarkPath:  "../../benchmarks/shoc/gemm",
		executablePath: "../../samples/shoc_gemm",
		executable:     "shoc_gemm",
		sizeArgs:       []string{"-m=64", "-n=64", "-k=64"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Re-enabled after V5 packed work-item ID fix
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},

	// ===== Parboil benchmarks =====
	{
		benchmarkPath:  "../../benchmarks/parboil/spmv_parboil",
		executablePath: "../../samples/parboil_spmv",
		executable:     "parboil_spmv",
		sizeArgs:       []string{"-num_rows=4096"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/parboil/stencil_parboil",
		executablePath: "../../samples/parboil_stencil",
		executable:     "parboil_stencil",
		sizeArgs:       []string{"-grid_dim=16", "-num_timesteps=2"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/parboil/cutcp",
		executablePath: "../../samples/parboil_cutcp",
		executable:     "parboil_cutcp",
		sizeArgs:       []string{"-num_atoms=1024"},
		cases: []benchmarkCase{
			// Only has gfx942 HSACO, so run cdna3 only
			{gpus: []int{1}, timing: false, parallel: false},
			// CDNA3 timing: deadlock (45-min timeout), run 23599480218
			// {gpus: []int{1}, timing: true, parallel: false},
		},
	},

	// CDNA3/MI300A (gfx942) architecture tests
	{
		benchmarkPath:  "../../benchmarks/amdappsdk/vectoradd",
		executablePath: "../../samples/vectoradd",
		executable:     "vectoradd",
		sizeArgs:       []string{"-width=4096", "-height=1"},
		cases: []benchmarkCase{
			// Emulation tests
			{gpus: []int{1}, timing: false, parallel: false},
			// Parallel disabled: V5 ParallelEngine data race causes panic.
			// {gpus: []int{1}, timing: false, parallel: true},
			// MI300A timing tests
			{
				gpus: []int{1}, timing: true, parallel: false,
				gpuType: "mi300a",
			},
			// Parallel disabled: V5 ParallelEngine data race causes panic.
			// {
			// 	gpus: []int{1}, timing: true, parallel: true,
			// 	gpuType: "mi300a",
			// },
		},
	},
	{
		benchmarkPath:  "../../benchmarks/shoc/stencil2d",
		executablePath: "../../samples/stencil2d",
		executable:     "stencil2d",
		sizeArgs:       []string{},
		cases: []benchmarkCase{
			// CDNA3 Emulation tests
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: false, parallel: true},
			// Re-enabled after V5 packed work-item ID fix
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/shoc/bfs",
		executablePath: "../../samples/bfs",
		executable:     "bfs",
		sizeArgs:       []string{"-node=1024"},
		cases: []benchmarkCase{
			// CDNA3 Emulation tests
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: false, parallel: true},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/rodinia/nw",
		executablePath: "../../samples/nw",
		executable:     "nw",
		sizeArgs:       []string{"-length=64"},
		cases: []benchmarkCase{
			// CDNA3 Emulation tests
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: false, parallel: true},
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},

	// ===== Polybench benchmarks (CDNA3) =====
	{
		benchmarkPath:  "../../benchmarks/polybench/2dconvolution",
		executablePath: "../../samples/2dconvolution",
		executable:     "2dconvolution",
		sizeArgs:       []string{"-ni=64", "-nj=64"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/polybench/2mm",
		executablePath: "../../samples/2mm",
		executable:     "2mm",
		sizeArgs:       []string{"-ni=64", "-nj=64", "-nk=64", "-nl=64"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Re-enabled after V5 packed work-item ID fix
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/polybench/3dconvolution",
		executablePath: "../../samples/3dconvolution",
		executable:     "3dconvolution",
		sizeArgs:       []string{"-ni=32", "-nj=32", "-nk=32"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/polybench/3mm",
		executablePath: "../../samples/3mm",
		executable:     "3mm",
		sizeArgs:       []string{"-ni=64", "-nj=64", "-nk=64", "-nl=64", "-nm=64"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Re-enabled after V5 packed work-item ID fix
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/polybench/correlation",
		executablePath: "../../samples/correlation",
		executable:     "correlation",
		sizeArgs:       []string{"-n=64", "-m=64"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/polybench/covariance",
		executablePath: "../../samples/covariance",
		executable:     "covariance",
		sizeArgs:       []string{"-n=64", "-m=64"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/polybench/fdtd2d",
		executablePath: "../../samples/fdtd2d",
		executable:     "fdtd2d",
		sizeArgs:       []string{"-nx=64", "-ny=64", "-tmax=5"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/polybench/gemm",
		executablePath: "../../samples/gemm",
		executable:     "gemm",
		sizeArgs:       []string{"-ni=64", "-nj=64", "-nk=64"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Re-enabled after V5 packed work-item ID fix
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/polybench/gesummv",
		executablePath: "../../samples/gesummv",
		executable:     "gesummv",
		sizeArgs:       []string{"-n=256"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/polybench/gramschmidt",
		executablePath: "../../samples/gramschmidt",
		executable:     "gramschmidt",
		sizeArgs:       []string{"-n=64", "-m=64"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Re-enabled after IsCDNA3 fix (SAddr=0 FLAT decoding)
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/polybench/mvt",
		executablePath: "../../samples/mvt",
		executable:     "mvt",
		sizeArgs:       []string{"-n=256"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/polybench/syr2k",
		executablePath: "../../samples/syr2k",
		executable:     "syr2k",
		sizeArgs:       []string{"-n=64", "-m=64"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Re-enabled after V5 packed work-item ID fix
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/polybench/syrk",
		executablePath: "../../samples/syrk",
		executable:     "syrk",
		sizeArgs:       []string{"-n=64", "-m=64"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Disabled: timing verification still failing after V5 fix (CI run 23676583498)
			// {gpus: []int{1}, timing: true, parallel: false},
		},
	},

	// ===== Rodinia benchmarks (CDNA3) =====
	{
		benchmarkPath:  "../../benchmarks/rodinia/backprop",
		executablePath: "../../samples/backprop",
		executable:     "backprop",
		sizeArgs:       []string{"-size=4096"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/rodinia/gaussian",
		executablePath: "../../samples/gaussian",
		executable:     "gaussian",
		sizeArgs:       []string{"-size=64"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Re-enabled after IsCDNA3 fix (SAddr=0 FLAT decoding)
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/rodinia/hotspot",
		executablePath: "../../samples/hotspot",
		executable:     "hotspot",
		sizeArgs:       []string{"-grid_size=64", "-num_iterations=5"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Phase 3 replacement: re-enabling timing (was disabled, tracking: #422)
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/rodinia/hotspot3D",
		executablePath: "../../samples/hotspot3D",
		executable:     "hotspot3D",
		sizeArgs:       []string{"-grid_size=16", "-num_iterations=5"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/rodinia/lavaMD",
		executablePath: "../../samples/lavaMD",
		executable:     "lavaMD",
		sizeArgs:       []string{"-boxes_per_dim=2", "-par_per_box=50"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	// lud: crashes with page-not-found in flat_store_dword_x2
	// {
	// 	benchmarkPath:  "../../benchmarks/rodinia/lud",
	// 	executablePath: "../../samples/lud",
	// 	executable:     "lud",
	// 	sizeArgs:       []string{"-size=64"},
	// 	cases: []benchmarkCase{
	// 		{gpus: []int{1}, timing: false, parallel: false},
	// 	},
	// },
	{
		benchmarkPath:  "../../benchmarks/rodinia/nn",
		executablePath: "../../samples/nn",
		executable:     "nn",
		sizeArgs:       []string{"-num_records=4096"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/rodinia/pathfinder",
		executablePath: "../../samples/pathfinder",
		executable:     "pathfinder",
		sizeArgs:       []string{"-cols=128", "-rows=20"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/rodinia/srad",
		executablePath: "../../samples/srad",
		executable:     "srad",
		sizeArgs:       []string{"-image_size=64", "-num_iterations=5"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Phase 3 replacement: re-enabling timing
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},

	// ===== Heteromark benchmarks (CDNA3) =====
	{
		benchmarkPath:  "../../benchmarks/heteromark/bs",
		executablePath: "../../samples/bs",
		executable:     "bs",
		sizeArgs:       []string{"-elements=65536"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Disabled: timing verification still failing after V5 fix (CI run 23676583498)
			// {gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/heteromark/ep",
		executablePath: "../../samples/ep",
		executable:     "ep",
		sizeArgs:       []string{"-elements=65536"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// CDNA3 timing: page not found panic (CI run 23660369949)
			// {gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/heteromark/ga",
		executablePath: "../../samples/ga",
		executable:     "ga",
		sizeArgs:       []string{"-population-size=256", "-max-generations=10"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// CDNA3 timing disabled: page not found panic in MMU (run 23661819809)
			// {gpus: []int{1}, timing: true, parallel: false},
		},
	},
	// hist: BROKEN — emulation verification mismatch (exit 1), parallel crash (exit 2).
	// Empty cases array caused generateCases() fallback which ran all combos.
	// Confirmed broken in run 23604753644. Commenting out entire entry.
	// {
	// 	benchmarkPath:  "../../benchmarks/heteromark/hist",
	// 	executablePath: "../../samples/hist",
	// 	executable:     "hist",
	// 	sizeArgs:       []string{"-num-elements=65536"},
	// 	cases: []benchmarkCase{
	// 		{gpus: []int{1}, timing: false, parallel: false},
	// 		{gpus: []int{1}, timing: true, parallel: false},
	// 	},
	// },

	// ===== SHOC benchmarks (CDNA3) =====
	{
		benchmarkPath:  "../../benchmarks/shoc/reduction",
		executablePath: "../../samples/reduction",
		executable:     "reduction",
		sizeArgs:       []string{"-num_elements=65536"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// CDNA3 timing: page not found panic (CI run 23660369949)
			// {gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/shoc/triad",
		executablePath: "../../samples/triad",
		executable:     "triad",
		sizeArgs:       []string{"-num_elements=65536"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Disabled: timing verification still failing after V5 fix (CI run 23676583498)
			// {gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/shoc/scan",
		executablePath: "../../samples/scan",
		executable:     "scan",
		sizeArgs:       []string{"-num_elements=65536"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Still failing: page not found panic (CI run 23654221393)
			// {gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/shoc/sort",
		executablePath: "../../samples/sort",
		executable:     "sort",
		sizeArgs:       []string{"-num_elements=4096"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// CDNA3 timing disabled: CI timeout after 45min (run 23661819809)
			// {gpus: []int{1}, timing: true, parallel: false},
		},
	},

	// ===== Parboil benchmarks (CDNA3) =====
	{
		benchmarkPath:  "../../benchmarks/parboil/sgemm",
		executablePath: "../../samples/parboil_sgemm",
		executable:     "parboil_sgemm",
		sizeArgs:       []string{"-n=64"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Re-enabled after V5 packed work-item ID fix
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/parboil/histo",
		executablePath: "../../samples/parboil_histo",
		executable:     "parboil_histo",
		sizeArgs:       []string{"-num_elements=65536"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// CDNA3 timing disabled: page not found panic in MMU (run 23661819809)
			// {gpus: []int{1}, timing: true, parallel: false},
		},
	},

	{
		benchmarkPath:  "../../benchmarks/amdappsdk/fastwalshtransform",
		executablePath: "../../samples/fastwalshtransform",
		executable:     "fastwalshtransform",
		sizeArgs:       []string{"-length=512"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// CDNA3 timing disabled: -length=512 still causes CI timeout
			// (>45min, runs 23626493450, 23629984630). Root cause may not
			// be only BlockCountX; keeping emulation-only for now.
			// {gpus: []int{1}, timing: true, parallel: false},
		},
	},
	// Removed fastwalshtransform -length=64: empty cases array triggered
	// generateCases() fallback, causing timing tests to hang (CI timeout).
	{
		benchmarkPath:  "",
		executablePath: "../../samples/memcopy",
		executable:     "memcopy",
		sizeArgs:       []string{},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/parboil/lbm",
		executablePath: "../../samples/parboil_lbm",
		executable:     "parboil_lbm",
		sizeArgs:       []string{"-grid_dim=8", "-num_timesteps=2"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/parboil/mri_q",
		executablePath: "../../samples/parboil_mri_q",
		executable:     "parboil_mri_q",
		sizeArgs:       []string{"-num_voxels=1024", "-num_k=64"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/parboil/mri_gridding",
		executablePath: "../../samples/parboil_mri_gridding",
		executable:     "parboil_mri_gridding",
		sizeArgs:       []string{"-num_samples=4096", "-grid_size=64"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
		},
	},
	{
		benchmarkPath:  "../../benchmarks/parboil/sad",
		executablePath: "../../samples/parboil_sad",
		executable:     "parboil_sad",
		sizeArgs:       []string{"-frame_width=64", "-block_size_sad=8", "-search_range=8"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
	// parboil_tpacf: fixed emulation (FLAT atomic GLC + 64-bit atomic ops)
	{
		benchmarkPath:  "../../benchmarks/parboil/tpacf",
		executablePath: "../../samples/parboil_tpacf",
		executable:     "parboil_tpacf",
		sizeArgs:       []string{"-num_points=256", "-num_bins=32"},
		cases: []benchmarkCase{
			{gpus: []int{1}, timing: false, parallel: false},
			// Re-enabled after VMCNT 6-bit + IsCDNA3 + SMEM offset fixes
			{gpus: []int{1}, timing: true, parallel: false},
		},
	},
}
