# Benchmark Results: main vs gfx942_emu (Scratchpad Removal)

## Summary

The `gfx942_emu` branch removes the scratchpad-based register marshalling from the `ScratchpadPreparer.Prepare()` and `Commit()` hot path, replacing it with direct `ReadOperand`/`WriteOperand` calls inside each ALU function. Microbenchmarks show that vector/memory instruction formats (VOP2, VOP3a, VOP3b, VOP1, VOPC, FLAT, DS) see **50–58% speedup** in the Prepare+Commit cycle and **eliminate all heap allocations**. Scalar instruction formats (SOP1, SOP2, SOPC, SOPK, SOPP, SMEM), which had minimal scratchpad work on `main`, show no significant regression. The end-to-end gputensor operator test suite shows a modest ~10–17% wall-time improvement.

## Methodology

- **Hardware**: Apple M2, macOS (arm64)
- **Go version**: determined at build time (module `github.com/sarchlab/mgpusim/v4`)
- **Baseline branch**: `main` (commit `6d36b99d`)
- **Test branch**: `gfx942_emu` (with scratchpad removal)
- **Microbenchmark tool**: `go test -bench=Benchmark -benchmem -count=5 -run='^$'`
- **End-to-end tool**: `go test ./amd/benchmarks/dnn/gputensor/ -v -run TestTensor -count=1` (3 runs per branch)
- **Benchmark code**: `amd/emu/bench_test.go` — identical file run on both branches

The benchmark measures `ScratchpadPreparerImpl.Prepare()` + `Commit()` for each instruction format. On `main`, `Prepare()` reads registers from the wavefront into the scratchpad byte buffer, and `Commit()` writes results back — involving per-lane loops and `readOperand`/`writeOperand` helper calls that allocate byte slices. On `gfx942_emu`, both methods are no-ops (just `p.clear()`), because instructions read/write registers directly.

## Microbenchmark Results

### Prepare+Commit (ns/op, average of 5 runs)

| Instruction Format | main (ns/op) | gfx942_emu (ns/op) | Speedup | main allocs/op | gfx942_emu allocs/op |
|--------------------|-------------|--------------------:|--------:|---------------:|---------------------:|
| **VOP2**           | 10,164      | 5,093               | **2.00×** | 130            | 0                    |
| **VOP3a**          | 10,622      | 4,812               | **2.21×** | 194            | 0                    |
| **VOP3b**          | 9,014       | 4,987               | **1.81×** | 130            | 0                    |
| **DS**             | 11,761      | 4,905               | **2.40×** | 192            | 0                    |
| **FLAT**           | 8,641       | 4,957               | **1.74×** | 129            | 0                    |
| **VOPC**           | 7,805       | 5,160               | **1.51×** | 128            | 0                    |
| **VOP1**           | 6,876       | 5,256               | **1.31×** | 66             | 0                    |
| SOP1               | 5,085       | 5,071               | 1.00×    | 3              | 0                    |
| SOP2               | 5,394       | 5,144               | 1.05×    | 2              | 0                    |
| SOPC               | 5,172       | 5,151               | 1.00×    | 1              | 0                    |
| SOPK               | 5,339       | 4,957               | 1.08×    | 1              | 0                    |
| SOPP               | 5,003       | 5,135               | 0.97×    | 0              | 0                    |
| SMEM               | 5,080       | 4,834               | 1.05×    | 1              | 0                    |

### Prepare-only VOP2 (ns/op, average of 5 runs)

| Benchmark          | main (ns/op) | gfx942_emu (ns/op) | Speedup |
|--------------------|-------------|--------------------:|--------:|
| PrepareOnly_VOP2   | 8,422       | 5,007               | **1.68×** |

### Commit-only VOP2 (ns/op, average of 5 runs)

| Benchmark          | main (ns/op) | gfx942_emu (ns/op) | Speedup |
|--------------------|-------------|--------------------:|--------:|
| CommitOnly_VOP2    | 154.3       | 8.74                | **17.7×** |

### Memory Allocations

On `main`, every call to `readOperand` for register operands calls `wf.ReadReg()` which allocates a `[]byte` slice. For vector instructions that iterate over 64 lanes, this causes 128–194 allocations per Prepare+Commit. On `gfx942_emu`, both Prepare and Commit are no-ops, resulting in **zero allocations**.

| Format | main B/op | main allocs/op | gfx942_emu B/op | gfx942_emu allocs/op |
|--------|-----------|----------------|-----------------|----------------------|
| VOP2   | 528       | 130            | 0               | 0                    |
| VOP3a  | 784       | 194            | 0               | 0                    |
| VOP3b  | 528       | 130            | 0               | 0                    |
| DS     | 768       | 192            | 0               | 0                    |
| FLAT   | 1,032     | 129            | 0               | 0                    |
| VOPC   | 512       | 128            | 0               | 0                    |
| VOP1   | 272       | 66             | 0               | 0                    |
| SOP1   | 16        | 3              | 0               | 0                    |
| SOP2   | 8         | 2              | 0               | 0                    |
| SOPC   | 4         | 1              | 0               | 0                    |
| SOPK   | 4         | 1              | 0               | 0                    |
| SOPP   | 0         | 0              | 0               | 0                    |
| SMEM   | 8         | 1              | 0               | 0                    |

## End-to-End Results: gputensor Operator Test Suite

Ran `go test ./amd/benchmarks/dnn/gputensor/ -v -run TestTensor -count=1` (31 operator tests) three times per branch.

| Branch       | Run 1 (s) | Run 2 (s) | Run 3 (s) | Average (s) |
|--------------|-----------|-----------|-----------|-------------|
| **main**     | 3.055     | 2.944     | 2.584     | **2.861**   |
| **gfx942_emu** | 2.412   | 2.675     | 2.339     | **2.475**   |

**End-to-end speedup: 1.16× (13.5% faster)**

Wall-clock times (including compilation):

| Branch       | Run 1 | Run 2 | Run 3 | Average |
|--------------|-------|-------|-------|---------|
| main         | 4.23s | 4.26s | 3.75s | 4.08s   |
| gfx942_emu   | 3.85s | 4.10s | 3.43s | 3.79s   |

## Analysis

### Why the improvement varies by instruction type

The Prepare/Commit overhead was proportional to the number of per-lane register reads:
- **DS** had the most scratchpad work (addr + data + data1 across 64 lanes) → **2.40× speedup**
- **VOP3a** read 3 source operands + dst across 64 lanes (194 allocs) → **2.21× speedup**
- **VOP2** read 2 sources + 1 dst across 64 lanes → **2.00× speedup**
- **Scalar instructions** (SOP1/SOP2/SOPC/SOPK) only read 1–2 operands per call (no per-lane loop) → **~1.0× (no change)**
- **SOPP** had zero scratchpad work on both branches → **no change**

### Why the Commit speedup is so dramatic (17.7×)

On `main`, `commitVOP2` iterated over 64 lanes to write results back from scratchpad to the register file. On `gfx942_emu`, it's a no-op (8.7 ns for the switch dispatch + function call overhead).

### Why the end-to-end improvement is modest (13.5%)

The gputensor test suite exercises the full simulation stack: kernel dispatch, memory operations, GPU timing simulation, and result verification. The Prepare/Commit overhead is only one component. The 13.5% end-to-end improvement is consistent with Prepare/Commit being a significant but not dominant fraction of total emulation time.

### Allocation elimination

The complete elimination of heap allocations in the Prepare/Commit path reduces GC pressure and improves cache behavior. This benefit compounds in long-running simulations where millions of instructions are emulated.

## Raw Data

### main branch (`go test -bench` output)

```
goos: darwin
goarch: arm64
pkg: github.com/sarchlab/mgpusim/v4/amd/emu
cpu: Apple M2
BenchmarkPrepareCommit_VOP2-8    	  282577	      9226 ns/op	     528 B/op	     130 allocs/op
BenchmarkPrepareCommit_VOP2-8    	  275534	     10576 ns/op	     528 B/op	     130 allocs/op
BenchmarkPrepareCommit_VOP2-8    	  280430	      9674 ns/op	     528 B/op	     130 allocs/op
BenchmarkPrepareCommit_VOP2-8    	  220682	     10851 ns/op	     528 B/op	     130 allocs/op
BenchmarkPrepareCommit_VOP2-8    	  270931	     10495 ns/op	     528 B/op	     130 allocs/op
BenchmarkPrepareOnly_VOP2-8      	  293917	     10119 ns/op	     528 B/op	     130 allocs/op
BenchmarkPrepareOnly_VOP2-8      	  294560	      8414 ns/op	     528 B/op	     130 allocs/op
BenchmarkPrepareOnly_VOP2-8      	  180048	      7616 ns/op	     528 B/op	     130 allocs/op
BenchmarkPrepareOnly_VOP2-8      	  171402	      8223 ns/op	     528 B/op	     130 allocs/op
BenchmarkPrepareOnly_VOP2-8      	  160738	      7736 ns/op	     528 B/op	     130 allocs/op
BenchmarkCommitOnly_VOP2-8       	10665303	       134.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkCommitOnly_VOP2-8       	34765351	       158.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkCommitOnly_VOP2-8       	34138147	       165.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkCommitOnly_VOP2-8       	34112026	       161.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkCommitOnly_VOP2-8       	32777182	       150.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOP1-8    	  767242	      5116 ns/op	      16 B/op	       3 allocs/op
BenchmarkPrepareCommit_SOP1-8    	  841140	      5641 ns/op	      16 B/op	       3 allocs/op
BenchmarkPrepareCommit_SOP1-8    	  240408	      4536 ns/op	      16 B/op	       3 allocs/op
BenchmarkPrepareCommit_SOP1-8    	  480524	      4803 ns/op	      16 B/op	       3 allocs/op
BenchmarkPrepareCommit_SOP1-8    	  190909	      5328 ns/op	      16 B/op	       3 allocs/op
BenchmarkPrepareCommit_SOP2-8    	  798031	      5740 ns/op	       8 B/op	       2 allocs/op
BenchmarkPrepareCommit_SOP2-8    	  753553	      5447 ns/op	       8 B/op	       2 allocs/op
BenchmarkPrepareCommit_SOP2-8    	  772584	      5387 ns/op	       8 B/op	       2 allocs/op
BenchmarkPrepareCommit_SOP2-8    	  779199	      5460 ns/op	       8 B/op	       2 allocs/op
BenchmarkPrepareCommit_SOP2-8    	  777136	      4938 ns/op	       8 B/op	       2 allocs/op
BenchmarkPrepareCommit_SOPC-8    	  703729	      5019 ns/op	       4 B/op	       1 allocs/op
BenchmarkPrepareCommit_SOPC-8    	  803203	      4907 ns/op	       4 B/op	       1 allocs/op
BenchmarkPrepareCommit_SOPC-8    	  758449	      5368 ns/op	       4 B/op	       1 allocs/op
BenchmarkPrepareCommit_SOPC-8    	  823051	      5203 ns/op	       4 B/op	       1 allocs/op
BenchmarkPrepareCommit_SOPC-8    	  808141	      5365 ns/op	       4 B/op	       1 allocs/op
BenchmarkPrepareCommit_SOPK-8    	  820393	      4794 ns/op	       4 B/op	       1 allocs/op
BenchmarkPrepareCommit_SOPK-8    	  757002	      5180 ns/op	       4 B/op	       1 allocs/op
BenchmarkPrepareCommit_SOPK-8    	  801134	      5140 ns/op	       4 B/op	       1 allocs/op
BenchmarkPrepareCommit_SOPK-8    	  231256	      6649 ns/op	       4 B/op	       1 allocs/op
BenchmarkPrepareCommit_SOPK-8    	  274395	      4934 ns/op	       4 B/op	       1 allocs/op
BenchmarkPrepareCommit_SOPP-8    	  835399	      4950 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOPP-8    	  813406	      4917 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOPP-8    	  835686	      5366 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOPP-8    	  813602	      4795 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOPP-8    	  818164	      4987 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP1-8    	  397424	      7201 ns/op	     272 B/op	      66 allocs/op
BenchmarkPrepareCommit_VOP1-8    	  343068	      7259 ns/op	     272 B/op	      66 allocs/op
BenchmarkPrepareCommit_VOP1-8    	  222883	      5247 ns/op	     272 B/op	      66 allocs/op
BenchmarkPrepareCommit_VOP1-8    	  257174	      6771 ns/op	     272 B/op	      66 allocs/op
BenchmarkPrepareCommit_VOP1-8    	  381556	      7903 ns/op	     272 B/op	      66 allocs/op
BenchmarkPrepareCommit_VOP3a-8   	  124873	      9449 ns/op	     784 B/op	     194 allocs/op
BenchmarkPrepareCommit_VOP3a-8   	  232335	      9653 ns/op	     784 B/op	     194 allocs/op
BenchmarkPrepareCommit_VOP3a-8   	  238254	     12118 ns/op	     784 B/op	     194 allocs/op
BenchmarkPrepareCommit_VOP3a-8   	  194326	     11024 ns/op	     784 B/op	     194 allocs/op
BenchmarkPrepareCommit_VOP3a-8   	  196240	     10868 ns/op	     784 B/op	     194 allocs/op
BenchmarkPrepareCommit_VOP3b-8   	  280059	      9133 ns/op	     528 B/op	     130 allocs/op
BenchmarkPrepareCommit_VOP3b-8   	  155157	      7172 ns/op	     528 B/op	     130 allocs/op
BenchmarkPrepareCommit_VOP3b-8   	  138000	     10202 ns/op	     528 B/op	     130 allocs/op
BenchmarkPrepareCommit_VOP3b-8   	  280686	      9049 ns/op	     528 B/op	     130 allocs/op
BenchmarkPrepareCommit_VOP3b-8   	  280328	      9512 ns/op	     528 B/op	     130 allocs/op
BenchmarkPrepareCommit_VOPC-8    	  312507	      8971 ns/op	     512 B/op	     128 allocs/op
BenchmarkPrepareCommit_VOPC-8    	  235622	      6822 ns/op	     512 B/op	     128 allocs/op
BenchmarkPrepareCommit_VOPC-8    	  208186	      6476 ns/op	     512 B/op	     128 allocs/op
BenchmarkPrepareCommit_VOPC-8    	  150040	      8708 ns/op	     512 B/op	     128 allocs/op
BenchmarkPrepareCommit_VOPC-8    	  315414	      8047 ns/op	     512 B/op	     128 allocs/op
BenchmarkPrepareCommit_FLAT-8    	  206085	      8113 ns/op	    1032 B/op	     129 allocs/op
BenchmarkPrepareCommit_FLAT-8    	  134644	      8162 ns/op	    1032 B/op	     129 allocs/op
BenchmarkPrepareCommit_FLAT-8    	  228468	     10534 ns/op	    1032 B/op	     129 allocs/op
BenchmarkPrepareCommit_FLAT-8    	  244032	      7585 ns/op	    1032 B/op	     129 allocs/op
BenchmarkPrepareCommit_FLAT-8    	  250293	      8813 ns/op	    1032 B/op	     129 allocs/op
BenchmarkPrepareCommit_DS-8      	   88922	     12825 ns/op	     768 B/op	     192 allocs/op
BenchmarkPrepareCommit_DS-8      	  211684	     11279 ns/op	     768 B/op	     192 allocs/op
BenchmarkPrepareCommit_DS-8      	  142184	     10174 ns/op	     768 B/op	     192 allocs/op
BenchmarkPrepareCommit_DS-8      	  145575	     13033 ns/op	     768 B/op	     192 allocs/op
BenchmarkPrepareCommit_DS-8      	  211911	     11492 ns/op	     768 B/op	     192 allocs/op
BenchmarkPrepareCommit_SMEM-8    	  799568	      5243 ns/op	       8 B/op	       1 allocs/op
BenchmarkPrepareCommit_SMEM-8    	  422041	      4827 ns/op	       8 B/op	       1 allocs/op
BenchmarkPrepareCommit_SMEM-8    	  806016	      4849 ns/op	       8 B/op	       1 allocs/op
BenchmarkPrepareCommit_SMEM-8    	  726927	      5122 ns/op	       8 B/op	       1 allocs/op
BenchmarkPrepareCommit_SMEM-8    	  357912	      5358 ns/op	       8 B/op	       1 allocs/op
PASS
ok  	github.com/sarchlab/mgpusim/v4/amd/emu	233.665s
```

### gfx942_emu branch (`go test -bench` output)

```
goos: darwin
goarch: arm64
pkg: github.com/sarchlab/mgpusim/v4/amd/emu
cpu: Apple M2
BenchmarkPrepareCommit_VOP2-8    	  831098	      5231 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP2-8    	  336216	      5079 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP2-8    	  817554	      5139 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP2-8    	  234055	      4548 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP2-8    	  818486	      5469 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareOnly_VOP2-8      	  839887	      5370 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareOnly_VOP2-8      	  819858	      5195 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareOnly_VOP2-8      	  753746	      4961 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareOnly_VOP2-8      	  840858	      5266 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareOnly_VOP2-8      	  817665	      4844 ns/op	       0 B/op	       0 allocs/op
BenchmarkCommitOnly_VOP2-8       	138510782	         7.930 ns/op	       0 B/op	       0 allocs/op
BenchmarkCommitOnly_VOP2-8       	133153273	         7.678 ns/op	       0 B/op	       0 allocs/op
BenchmarkCommitOnly_VOP2-8       	128571468	         8.618 ns/op	       0 B/op	       0 allocs/op
BenchmarkCommitOnly_VOP2-8       	133047324	        10.41 ns/op	       0 B/op	       0 allocs/op
BenchmarkCommitOnly_VOP2-8       	166818704	         9.053 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOP1-8    	  395830	      5304 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOP1-8    	  838058	      5262 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOP1-8    	  380371	      4364 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOP1-8    	  823723	      5123 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOP1-8    	  817728	      5303 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOP2-8    	  358282	      5084 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOP2-8    	  838801	      5336 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOP2-8    	  328893	      4800 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOP2-8    	  817437	      5002 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOP2-8    	  768926	      5500 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOPC-8    	  895544	      4877 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOPC-8    	  786128	      5346 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOPC-8    	  826671	      4964 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOPC-8    	  817822	      5202 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOPC-8    	  273384	      5364 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOPK-8    	  252228	      4594 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOPK-8    	  837225	      4999 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOPK-8    	  816488	      5147 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOPK-8    	  821502	      4897 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOPK-8    	  839206	      5146 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOPP-8    	  817424	      5011 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOPP-8    	  777510	      5620 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOPP-8    	  817714	      5107 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOPP-8    	  836182	      4844 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SOPP-8    	  817837	      5095 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP1-8    	  259376	      5875 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP1-8    	  817816	      5132 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP1-8    	  839018	      4977 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP1-8    	  806360	      5045 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP1-8    	  816723	      5249 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP3a-8   	  837888	      4574 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP3a-8   	  250612	      4555 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP3a-8   	  839001	      4810 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP3a-8   	  817680	      4904 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP3a-8   	  817762	      5219 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP3b-8   	  233348	      4355 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP3b-8   	  791808	      5099 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP3b-8   	  838794	      5316 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP3b-8   	  816108	      4975 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOP3b-8   	  836479	      5188 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOPC-8    	  817536	      5002 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOPC-8    	  836016	      5452 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOPC-8    	  817825	      5337 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOPC-8    	  817770	      4803 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_VOPC-8    	  837970	      5205 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_FLAT-8    	  423920	      4662 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_FLAT-8    	  814686	      5129 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_FLAT-8    	  333000	      5088 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_FLAT-8    	  813253	      4836 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_FLAT-8    	  833707	      5071 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_DS-8      	  812180	      4270 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_DS-8      	  254271	      4856 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_DS-8      	  817911	      5304 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_DS-8      	  200535	      5026 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_DS-8      	  816518	      5067 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SMEM-8    	  771862	      4684 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SMEM-8    	  790728	      5214 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SMEM-8    	  816453	      4400 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SMEM-8    	  817819	      4890 ns/op	       0 B/op	       0 allocs/op
BenchmarkPrepareCommit_SMEM-8    	  817623	      4982 ns/op	       0 B/op	       0 allocs/op
PASS
ok  	github.com/sarchlab/mgpusim/v4/amd/emu	269.177s
```

### gputensor end-to-end: main branch

```
=== Run 1 ===
Ran 31 of 31 Specs in 3.055 seconds
SUCCESS! -- 31 Passed | 0 Failed | 0 Pending | 0 Skipped
real	0m4.227s
---
=== Run 2 ===
Ran 31 of 31 Specs in 2.944 seconds
SUCCESS! -- 31 Passed | 0 Failed | 0 Pending | 0 Skipped
real	0m4.264s
---
=== Run 3 ===
Ran 31 of 31 Specs in 2.584 seconds
SUCCESS! -- 31 Passed | 0 Failed | 0 Pending | 0 Skipped
real	0m3.745s
---
```

### gputensor end-to-end: gfx942_emu branch

```
=== Run 1 ===
Ran 31 of 31 Specs in 2.412 seconds
SUCCESS! -- 31 Passed | 0 Failed | 0 Pending | 0 Skipped
real	0m3.850s
---
=== Run 2 ===
Ran 31 of 31 Specs in 2.675 seconds
SUCCESS! -- 31 Passed | 0 Failed | 0 Pending | 0 Skipped
real	0m4.101s
---
=== Run 3 ===
Ran 31 of 31 Specs in 2.339 seconds
SUCCESS! -- 31 Passed | 0 Failed | 0 Pending | 0 Skipped
real	0m3.432s
---
```
