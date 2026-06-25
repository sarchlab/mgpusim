/*
 * HIP kernel for the empty_kernel launch-overhead microbenchmark (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier1/empty_kernel.
 *
 * Does no work. It isolates pure kernel launch / dispatch / grid-scheduling /
 * completion overhead: the simulated kernel time for a single launch is the
 * model's per-launch cost, comparable to the real benchmark's launch_sync_us.
 *
 * It carries one unused pointer argument purely so the kernel has a non-empty
 * kernarg segment -- MGPUSim's driver cannot allocate a zero-byte kernarg buffer,
 * and the original is argument-less. The argument is never dereferenced, so the
 * kernel still performs no work and touches no memory.
 */
#include "hip/hip_runtime.h"

extern "C" __global__ void empty_kernel(int* out) {
    (void)out;
}
