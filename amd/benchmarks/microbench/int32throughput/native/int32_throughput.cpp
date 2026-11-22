// Device source from GPU Benchmarks v1.1 (a8ca84f8), int32_throughput.hip.
#include <hip/hip_runtime.h>

extern "C" __global__ void int32_mad_kernel(unsigned *out, int mads_per_thread,
                                 unsigned mul, unsigned add)
{
    unsigned a0 = 1u + threadIdx.x;
    unsigned a1 = a0 + 111u;
    unsigned a2 = a0 + 222u;
    unsigned a3 = a0 + 333u;

    for (int i = 0; i < mads_per_thread; i += 4) {
        a0 = a0 * mul + add;
        a1 = a1 * mul + add;
        a2 = a2 * mul + add;
        a3 = a3 * mul + add;
    }

    // Store one checksum per global thread (not just thread 0) so every
    // launched thread's MAD chain is observable. A single guarded write would
    // let the optimizer treat the other threads' recurrences as dead and
    // eliminate them, undercounting the work the grid reports. The write is a
    // single store after the timed loop, negligible next to the per-thread MAD
    // chain.
    const unsigned tid = blockIdx.x * blockDim.x + threadIdx.x;
    out[tid] = a0 + a1 + a2 + a3;
}
