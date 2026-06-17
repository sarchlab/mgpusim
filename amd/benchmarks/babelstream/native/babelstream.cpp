/*
 * HIP kernels for BabelStream (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier4/babelstream.
 *
 * BabelStream measures memory bandwidth via four elementwise vector
 * operations over float arrays of length n:
 *   copy:   c[i] = a[i]
 *   scale:  b[i] = s * c[i]
 *   add:    c[i] = a[i] + b[i]
 *   triad:  a[i] = b[i] + s * c[i]
 *
 * Each kernel is a simple 1D grid-stride-free elementwise map, launched with
 * a constant block size of 256. The block dimension is a compile-time literal
 * (BLOCK_SIZE) and the kernels read only blockIdx.x / threadIdx.x, so the
 * compiler emits no hidden ABI arguments. Each kernarg_segment_size is small:
 * two 8-byte pointers + an optional 4-byte scalar + a 4-byte int.
 */
#include "hip/hip_runtime.h"

#define BLOCK_SIZE 256

extern "C" __global__ void copy_kernel(
    const float* __restrict__ a,
    float* __restrict__ c,
    int n)
{
    int i = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    if (i < n) c[i] = a[i];
}

extern "C" __global__ void scale_kernel(
    const float* __restrict__ c,
    float* __restrict__ b,
    float s,
    int n)
{
    int i = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    if (i < n) b[i] = s * c[i];
}

extern "C" __global__ void add_kernel(
    const float* __restrict__ a,
    const float* __restrict__ b,
    float* __restrict__ c,
    int n)
{
    int i = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    if (i < n) c[i] = a[i] + b[i];
}

extern "C" __global__ void triad_kernel(
    const float* __restrict__ b,
    const float* __restrict__ c,
    float* __restrict__ a,
    float s,
    int n)
{
    int i = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    if (i < n) a[i] = b[i] + s * c[i];
}
