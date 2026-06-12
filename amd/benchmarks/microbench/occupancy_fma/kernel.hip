#include <hip/hip_runtime.h>

// Occupancy test: FMA with tunable register pressure
// Uses many temporary variables to increase register usage
__global__ void occupancy_fma_kernel(float* __restrict__ A,
                                     float* __restrict__ B,
                                     int N) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid < N) {
        float a0 = A[gid];
        float a1 = a0 * 1.01f;
        float a2 = a0 * 1.02f;
        float a3 = a0 * 1.03f;
        float a4 = a0 * 1.04f;
        float a5 = a0 * 1.05f;
        float a6 = a0 * 1.06f;
        float a7 = a0 * 1.07f;
        float a8 = a0 * 1.08f;
        float a9 = a0 * 1.09f;
        float a10 = a0 * 1.10f;
        float a11 = a0 * 1.11f;
        float a12 = a0 * 1.12f;
        float a13 = a0 * 1.13f;
        float a14 = a0 * 1.14f;
        float a15 = a0 * 1.15f;

        for (int i = 0; i < 256; i++) {
            a0  = a0  * a1  + a2;
            a1  = a1  * a2  + a3;
            a2  = a2  * a3  + a4;
            a3  = a3  * a4  + a5;
            a4  = a4  * a5  + a6;
            a5  = a5  * a6  + a7;
            a6  = a6  * a7  + a8;
            a7  = a7  * a8  + a9;
            a8  = a8  * a9  + a10;
            a9  = a9  * a10 + a11;
            a10 = a10 * a11 + a12;
            a11 = a11 * a12 + a13;
            a12 = a12 * a13 + a14;
            a13 = a13 * a14 + a15;
            a14 = a14 * a15 + a0;
            a15 = a15 * a0  + a1;
        }

        B[gid] = a0 + a1 + a2 + a3 + a4 + a5 + a6 + a7 +
                 a8 + a9 + a10 + a11 + a12 + a13 + a14 + a15;
    }
}
