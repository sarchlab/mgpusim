/*
 * HIP kernels for PolyBench 3MM (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/polybench_3mm.
 *
 * Three chained matrix multiplications:
 *   E = A * B  (NI×NK · NK×NJ → NI×NJ)
 *   F = C * D  (NJ×NM · NM×NL → NJ×NL)
 *   G = E * F  (NI×NJ · NJ×NL → NI×NL)
 *
 * Each thread computes one output element with a simple dot-product loop.
 * Block dimensions are CONSTANT 16×16, so the compiler emits no hidden ABI
 * arguments (kernarg_segment_size = 32 per kernel: 3 pointers + 3 int32).
 */
#include "hip/hip_runtime.h"

#define BLOCK_SIZE 16

// Kernel 1: E = A * B   (A: NI×NK, B: NK×NJ → E: NI×NJ)
extern "C" __global__ void mm3_kernel1(const float* __restrict__ A,
                                       const float* __restrict__ B,
                                       float* __restrict__       E,
                                       int NI, int NK, int NJ) {
    int j = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    int i = blockIdx.y * BLOCK_SIZE + threadIdx.y;
    if (i < NI && j < NJ) {
        float sum = 0.0f;
        for (int k = 0; k < NK; k++)
            sum += A[i * NK + k] * B[k * NJ + j];
        E[i * NJ + j] = sum;
    }
}

// Kernel 2: F = C * D   (C: NJ×NM, D: NM×NL → F: NJ×NL)
extern "C" __global__ void mm3_kernel2(const float* __restrict__ C,
                                       const float* __restrict__ D,
                                       float* __restrict__       F,
                                       int NJ, int NM, int NL) {
    int l = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    int j = blockIdx.y * BLOCK_SIZE + threadIdx.y;
    if (j < NJ && l < NL) {
        float sum = 0.0f;
        for (int m = 0; m < NM; m++)
            sum += C[j * NM + m] * D[m * NL + l];
        F[j * NL + l] = sum;
    }
}

// Kernel 3: G = E * F   (E: NI×NJ, F: NJ×NL → G: NI×NL)
extern "C" __global__ void mm3_kernel3(const float* __restrict__ E,
                                       const float* __restrict__ F,
                                       float* __restrict__       G,
                                       int NI, int NJ, int NL) {
    int l = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    int i = blockIdx.y * BLOCK_SIZE + threadIdx.y;
    if (i < NI && l < NL) {
        float sum = 0.0f;
        for (int j = 0; j < NJ; j++)
            sum += E[i * NJ + j] * F[j * NL + l];
        G[i * NL + l] = sum;
    }
}
