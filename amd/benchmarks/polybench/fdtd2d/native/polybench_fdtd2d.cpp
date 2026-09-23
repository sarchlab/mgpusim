/*
 * HIP kernels for PolyBench FDTD-2D (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/polybench_fdtd2d.
 *
 * 2D Finite Difference Time Domain electromagnetic simulation over three
 * NX x NY field arrays: ex, ey, hz.  Per time step:
 *   ex[0][j]  = 0;  ex[i][j] += 0.5*(hz[i][j] - hz[i-1][j])   (i>=1)
 *   ey[i][0]  = 0;  ey[i][j] += 0.5*(hz[i][j] - hz[i][j-1])   (j>=1)
 *   hz[i][j] -= 0.7*(ex[i][j+1]-ex[i][j] + ey[i+1][j]-ey[i][j]) (i<NX-1,j<NY-1)
 *
 * The original benchmark launches a 16x16 block.  We hard-code the block
 * dimension as a constant (BLOCK = 16) instead of reading blockDim.x/y so the
 * compiler emits no hidden ABI arguments (kernarg_segment_size = 24 per
 * kernel: two 8-byte pointers + two 4-byte ints, packed with no padding).
 */
#include "hip/hip_runtime.h"

#define BLOCK 16

// Kernel 1: Update ex
extern "C" __global__ void fdtd_update_ex(
    float* __restrict__ ex,
    const float* __restrict__ hz,
    int NX, int NY)
{
    int j = blockIdx.x * BLOCK + threadIdx.x;
    int i = blockIdx.y * BLOCK + threadIdx.y;

    if (i >= NX || j >= NY) return;

    if (i == 0) {
        ex[0 * NY + j] = 0.0f;
    } else {
        ex[i * NY + j] += 0.5f * (hz[i * NY + j] - hz[(i - 1) * NY + j]);
    }
}

// Kernel 2: Update ey
extern "C" __global__ void fdtd_update_ey(
    float* __restrict__ ey,
    const float* __restrict__ hz,
    int NX, int NY)
{
    int j = blockIdx.x * BLOCK + threadIdx.x;
    int i = blockIdx.y * BLOCK + threadIdx.y;

    if (i >= NX || j >= NY) return;

    if (j == 0) {
        ey[i * NY + 0] = 0.0f;
    } else {
        ey[i * NY + j] += 0.5f * (hz[i * NY + j] - hz[i * NY + (j - 1)]);
    }
}

// Kernel 3: Update hz
extern "C" __global__ void fdtd_update_hz(
    const float* __restrict__ ex,
    const float* __restrict__ ey,
    float* __restrict__ hz,
    int NX, int NY)
{
    int j = blockIdx.x * BLOCK + threadIdx.x;
    int i = blockIdx.y * BLOCK + threadIdx.y;

    if (i >= NX - 1 || j >= NY - 1) return;

    hz[i * NY + j] -= 0.7f * (ex[i * NY + (j + 1)] - ex[i * NY + j] +
                               ey[(i + 1) * NY + j] - ey[i * NY + j]);
}
