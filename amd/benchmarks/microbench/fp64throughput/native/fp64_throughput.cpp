// Device source from GPU Benchmarks v1.1 (a8ca84f8), fp64_throughput.hip.
#include <hip/hip_runtime.h>

extern "C" __global__ void fp64_fma_kernel(double *out, int fmas_per_thread)
{
    double a0 = 1.0 + static_cast<double>(threadIdx.x) * 0.001;
    double a1 = a0 + 0.1;
    double a2 = a0 + 0.2;
    double a3 = a0 + 0.3;

    const double mul = 1.0000000001;
    const double add = 0.0000000001;

    for (int i = 0; i < fmas_per_thread; i += 4) {
        a0 = fma(a0, mul, add);
        a1 = fma(a1, mul, add);
        a2 = fma(a2, mul, add);
        a3 = fma(a3, mul, add);
    }

    // One result per launched thread makes every accumulator chain observable.
    // This final store is included in the timed kernel, outside the FMA loop.
    const size_t tid = static_cast<size_t>(blockIdx.x) * blockDim.x + threadIdx.x;
    out[tid] = a0 + a1 + a2 + a3;
}
