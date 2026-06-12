#!/usr/bin/env bash
set -euo pipefail

# Simulator build commands used locally on macOS. Cache paths stay inside the repo.
mkdir -p .gocache .gomodcache .gopath
env CGO_ENABLED=1 GOCACHE="$PWD/.gocache" GOMODCACHE="$PWD/.gomodcache" GOPATH="$PWD/.gopath" \
  go build -o _bench_l1_cache_bw ./amd/samples/l1_cache_bw/
env CGO_ENABLED=1 GOCACHE="$PWD/.gocache" GOMODCACHE="$PWD/.gomodcache" GOPATH="$PWD/.gopath" \
  go build -o _bench_l2_cache_bw ./amd/samples/l2_cache_bw/

# macOS did not have GNU timeout; actual local runs used Python subprocess.run(..., timeout=300).
# Representative simulator commands (run each in an isolated directory containing the binary):
./_bench_l1_cache_bw -timing -arch cdna3 -gpu mi300a -disable-rtm -num_elements 65536 -num_repeats 64
./_bench_l1_cache_bw -timing -arch cdna3 -gpu mi300a -disable-rtm -num_elements 131072 -num_repeats 64
./_bench_l1_cache_bw -timing -arch cdna3 -gpu mi300a -disable-rtm -num_elements 262144 -num_repeats 64
./_bench_l2_cache_bw -timing -arch cdna3 -gpu mi300a -disable-rtm -num_elements 65536 -num_repeats 64 -working_set_size 262144
./_bench_l2_cache_bw -timing -arch cdna3 -gpu mi300a -disable-rtm -num_elements 131072 -num_repeats 64 -working_set_size 262144
./_bench_l2_cache_bw -timing -arch cdna3 -gpu mi300a -disable-rtm -num_elements 262144 -num_repeats 64 -working_set_size 262144

# SQLite extraction used for sim_ms from Driver/kernel_time:
python3 -c "import sqlite3,glob; db=glob.glob('akita_sim_*.sqlite3')[-1]; c=sqlite3.connect(db).cursor(); c.execute(\"SELECT Value FROM mgpusim_metrics WHERE What='kernel_time' AND Location='Driver'\"); v=c.fetchone()[0]; print(f'{v/1e9:.6f}' if v>1e6 else f'{v*1000:.6f}')"

# Hardware runner commands to use on a ROCm/MI300A host (not run locally because hipcc/rocm-smi were absent):
# cd workloads/microbench/l1_cache_bw/hip && make clean all GPU_ARCH=gfx942
# ./l1_cache_bw --iterations 10 --num_elements 65536 --working_set_size 8192 --num_iterations 64 --block_size 256
# ./l1_cache_bw --iterations 10 --num_elements 131072 --working_set_size 8192 --num_iterations 64 --block_size 256
# ./l1_cache_bw --iterations 10 --num_elements 262144 --working_set_size 8192 --num_iterations 64 --block_size 256
# cd workloads/microbench/l2_cache_bw/hip && make clean all GPU_ARCH=gfx942
# ./l2_cache_bw --iterations 10 --num_elements 65536 --working_set_size 262144 --num_iterations 64 --block_size 256
# ./l2_cache_bw --iterations 10 --num_elements 131072 --working_set_size 262144 --num_iterations 64 --block_size 256
# ./l2_cache_bw --iterations 10 --num_elements 262144 --working_set_size 262144 --num_iterations 64 --block_size 256
