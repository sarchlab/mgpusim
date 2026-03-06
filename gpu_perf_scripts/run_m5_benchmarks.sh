#!/bin/bash
# M5 Benchmark Runner Script
# Run benchmarks on ares/m5-snop-and-tuning branch

REPO="/Users/yifan/.thebotcompany/dev/src/github.com/sarchlab/mgpusim-dev/repo"
RESULTS_FILE="$REPO/gpu_perf_scripts/sim_results_m5.csv"
TIMEOUT_CMD="perl -e 'alarm 120; exec @ARGV' --"

echo "kernel_name,problem_size,sim_time_ms" > "$RESULTS_FILE"

run_benchmark() {
    local name=$1
    local dir=$2
    local problem_size=$3
    shift 3
    local flags="$@"
    
    echo "=== Running $name ($problem_size) ==="
    cd "$REPO/amd/samples/$dir"
    
    # Clean up any old sqlite files
    rm -f akita_sim_*.sqlite3
    
    # Run with timeout
    $TIMEOUT_CMD ./$dir -timing -arch cdna3 -gpu mi300a -disable-rtm $flags 2>&1 | tail -5
    local exit_code=${PIPESTATUS[0]}
    
    if [ $exit_code -ne 0 ]; then
        echo "FAILED or TIMEOUT: $name ($problem_size) exit=$exit_code"
        return
    fi
    
    # Extract kernel_time
    local db_file=$(ls akita_sim_*.sqlite3 2>/dev/null | head -1)
    if [ -z "$db_file" ]; then
        echo "NO DB FILE: $name ($problem_size)"
        return
    fi
    
    local kernel_time=$(sqlite3 "$db_file" "SELECT Value FROM mgpusim_metrics WHERE What='kernel_time' AND Location='Driver'" 2>/dev/null)
    if [ -z "$kernel_time" ]; then
        echo "NO KERNEL_TIME: $name ($problem_size)"
        rm -f akita_sim_*.sqlite3
        return
    fi
    
    # Convert seconds to ms
    local time_ms=$(echo "$kernel_time * 1000" | bc -l)
    echo "$name,$problem_size,$time_ms" >> "$RESULTS_FILE"
    echo "  kernel_time = ${kernel_time}s = ${time_ms}ms"
    
    # Clean up
    rm -f akita_sim_*.sqlite3
}

echo "Starting M5 benchmarks at $(date)"

# 1. vectoradd - pick 2 sizes
run_benchmark vectoradd vectoradd "1048576" -length 1048576
run_benchmark vectoradd vectoradd "16777216" -length 16777216

# 2. relu - pick 2 sizes
run_benchmark relu relu "1048576" -length 1048576
run_benchmark relu relu "16777216" -length 16777216

# 3. stencil2d - pick 2 sizes (use -row -col, default iter=5)
run_benchmark stencil2d stencil2d "256x256" -row 256 -col 256
run_benchmark stencil2d stencil2d "1024x1024" -row 1024 -col 1024

# 4. matrixmultiplication - pick 2 sizes
run_benchmark matrixmultiplication matrixmultiplication "256x256x256" -x 256 -y 256 -z 256
run_benchmark matrixmultiplication matrixmultiplication "512x512x512" -x 512 -y 512 -z 512

# 5. floydwarshall - pick 2 sizes
run_benchmark floydwarshall floydwarshall "128_nodes" -node 128
run_benchmark floydwarshall floydwarshall "256_nodes" -node 256

# 6. nbody - pick 2 sizes (-iter 1)
run_benchmark nbody nbody "1024_particles" -particles 1024 -iter 1
run_benchmark nbody nbody "4096_particles" -particles 4096 -iter 1

# 7. nw - pick 2 sizes
run_benchmark nw nw "length=256" -length 256
run_benchmark nw nw "length=512" -length 512

# 8. fir - pick 2 sizes (taps fixed at 16)
run_benchmark fir fir "16384_taps16" -length 16384
run_benchmark fir fir "262144_taps16" -length 262144

# 9. kmeans - pick 2 sizes
run_benchmark kmeans kmeans "pts4096_feat32_clus5" -points 4096 -features 32 -clusters 5 -max-iter 1
run_benchmark kmeans kmeans "pts65536_feat32_clus5" -points 65536 -features 32 -clusters 5 -max-iter 1

# 10. atax - pick 2 sizes
run_benchmark atax atax "256x256" -x 256 -y 256
run_benchmark atax atax "1024x1024" -x 1024 -y 1024

# 11. spmv - pick 2 sizes (spmv_csr_scalar in reference - use dim and sparsity)
# 4096x4096 with nnz=16384 => sparsity = nnz/(dim*dim) = 16384/16777216 ≈ 0.001
# 16384x16384 with nnz=65536 => sparsity ≈ 0.00024
run_benchmark spmv_csr_scalar spmv "4096x4096_nnz16384" -dim 4096 -sparsity 0.001
run_benchmark spmv_csr_scalar spmv "16384x16384_nnz65536" -dim 16384 -sparsity 0.000244

# === Newly-unlocked benchmarks (should work now with s_nop fix) ===

# 12. bitonicsort - pick 2 sizes
run_benchmark bitonicsort bitonicsort "1024" -length 1024
run_benchmark bitonicsort bitonicsort "8192" -length 8192

# 13. simpleconvolution - pick 2 sizes
run_benchmark simpleconvolution simpleconvolution "256x256_mask3" -width 256 -height 256 -mask-size 3
run_benchmark simpleconvolution simpleconvolution "1024x1024_mask3" -width 1024 -height 1024 -mask-size 3

# 14. matrixtranspose - pick 2 sizes
run_benchmark matrixtranspose matrixtranspose "256x256" -width 256
run_benchmark matrixtranspose matrixtranspose "1024x1024" -width 1024

# 15. bfs - pick 2 sizes (use -node flag)
run_benchmark bfs bfs "nodes1024" -node 1024
run_benchmark bfs bfs "nodes4096" -node 4096

# 16. fastwalshtransform - pick 2 sizes
run_benchmark fastwalshtransform fastwalshtransform "4096" -length 4096
run_benchmark fastwalshtransform fastwalshtransform "65536" -length 65536

echo ""
echo "=== M5 Benchmarks Complete at $(date) ==="
echo "Results saved to: $RESULTS_FILE"
cat "$RESULTS_FILE"
