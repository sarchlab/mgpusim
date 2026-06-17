/*
 * HIP kernel for the fp64_throughput microbenchmark (gfx942 / CDNA3).
 * Extracted from sarchlab/gpu_benchmarks tier1/fp64_throughput.
 *
 * Each thread runs a long chain of FP64 fused multiply-adds (FMA) over
 * four independent accumulators, then writes the sum of its accumulators
 * to out[globalThreadId]. The kernel reads its FP64 inputs from a small
 * per-thread "in" buffer and performs a single trailing store per thread,
 * so it stresses FP64 arithmetic throughput.
 *
 * Differences from the upstream HIP source, and why each is needed for the
 * MGPUSim CDNA3 model:
 *
 *  1. Every thread stores its own result to out[gid] (gid = blockIdx.x *
 *     BLOCK_SIZE + threadIdx.x), instead of only thread (0,0) storing.
 *     A kernel whose ONLY memory access is a single store from one lane
 *     trips a "page not found" panic in the MGPUSim timing MMU model; a
 *     per-thread store (as in the working gemm/fir kernels) is well-behaved
 *     and makes the result fully verifiable.
 *
 *  2. The per-thread seed is NOT perturbed by threadIdx.x. Upstream seeds
 *     a0 with "1.0 + threadIdx.x*0.001"; that path needs v_cvt_f64_u32 of
 *     the work-item-ID register, which the MGPUSim CDNA3 emulator reads as 0
 *     on every lane. A thread-independent seed keeps the FP64 FMA workload
 *     intact while staying on paths the emulator handles correctly.
 *
 *  3. BLOCK_SIZE is a compile-time constant (so blockDim.x is not read),
 *     keeping the kernel free of hidden ABI args.
 *
 *  4. All FP64 operands (the four accumulator seeds and the FMA
 *     multiplier/addend) are read from a per-thread slice of the read-only
 *     "in" buffer, indexed by gid so the compiler cannot constant-fold them.
 *     This forces the compiler to bring the doubles into VGPR pairs with
 *     global_load_dwordx2 (a true 64-bit pair load) and to keep them there.
 *     Inline/immediate FP64 constants would instead be materialized with
 *     v_mov_b64_e32 (VOP1 opcode 56), which this CDNA3 model decodes as the
 *     32-bit v_movrelsd_b32 and so only copies the low 32 bits of each
 *     double, silently corrupting every 64-bit value. The FMA loop stays a
 *     chain of v_fma_f64 (VOP3, supported), so the benchmark still measures
 *     fused FP64 FMAs.
 *
 * Per-thread "in" layout (STRIDE = 8 doubles per thread, at
 * in[gid*STRIDE + k]). A power-of-two stride lets the compiler form the load
 * base with v_lshl_add_u64 (a supported shift-add) rather than a 64-bit
 * integer multiply (v_mad_i64_i32, VOP3 opcode 489), which is unimplemented
 * in this CDNA3 model. Only the first six slots carry data:
 *   k=0 : seed for a0
 *   k=1 : seed for a1
 *   k=2 : seed for a2
 *   k=3 : seed for a3
 *   k=4 : FMA multiplier
 *   k=5 : FMA addend
 *   k=6..7 : padding (unused)
 *
 * The Go Verify() reproduces the stored value exactly for every thread.
 */
#include "hip/hip_runtime.h"

#define BLOCK_SIZE 64
#define STRIDE 8

extern "C" __global__ void fp64_fma_kernel(
    double* __restrict__ out,
    const double* __restrict__ in,
    int fmas_per_thread)
{
    int gid = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    const double* p = in + (long)gid * STRIDE;

    double a0 = p[0];
    double a1 = p[1];
    double a2 = p[2];
    double a3 = p[3];
    double mul = p[4];
    double add = p[5];

    for (int i = 0; i < fmas_per_thread; i += 4) {
        a0 = fma(a0, mul, add);
        a1 = fma(a1, mul, add);
        a2 = fma(a2, mul, add);
        a3 = fma(a3, mul, add);
    }

    out[gid] = a0 + a1 + a2 + a3;
}
