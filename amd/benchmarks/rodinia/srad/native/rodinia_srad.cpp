/*
 * HIP kernels for Rodinia SRAD (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/rodinia_srad.
 *
 * Speckle-Reducing Anisotropic Diffusion (SRAD): iterative 2D image smoothing
 * using a Perona-Malik anisotropic diffusion scheme.
 *
 * Two-phase stencil per iteration:
 *   srad1: directional gradients (dN/dS/dW/dE) and diffusion coefficient c.
 *   srad2: image update using the diffusion coefficients.
 *
 * Uses a constant BLOCK_SIZE (not blockDim) for the block geometry so the
 * compiler emits no hidden ABI arguments. One thread per pixel.
 */
#include "hip/hip_runtime.h"

#define BLOCK_SIZE 16

extern "C" __global__ void srad1(
    const float* __restrict__ J,
    float* __restrict__ dN, float* __restrict__ dS,
    float* __restrict__ dW, float* __restrict__ dE,
    float* __restrict__ c,
    int rows, int cols, float q0sqr)
{
    int col = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    int row = blockIdx.y * BLOCK_SIZE + threadIdx.y;

    if (row >= rows || col >= cols) return;

    int idx = row * cols + col;

    int iN = (row > 0)        ? (row - 1) : 0;
    int iS = (row < rows - 1) ? (row + 1) : (rows - 1);
    int jW = (col > 0)        ? (col - 1) : 0;
    int jE = (col < cols - 1) ? (col + 1) : (cols - 1);

    float Jc = J[idx];

    float dn = J[iN * cols + col] - Jc;
    float ds = J[iS * cols + col] - Jc;
    float dw = J[row * cols + jW] - Jc;
    float de = J[row * cols + jE] - Jc;

    dN[idx] = dn;
    dS[idx] = ds;
    dW[idx] = dw;
    dE[idx] = de;

    float G2   = (dn*dn + ds*ds + dw*dw + de*de) / (Jc * Jc);
    float L    = (dn + ds + dw + de) / Jc;
    float num  = (0.5f * G2) - ((1.0f / 16.0f) * (L * L));
    float den  = 1.0f + (0.25f * L);
    float qsqr = num / (den * den);

    den = (qsqr - q0sqr) / (q0sqr * (1.0f + q0sqr));
    float ci = 1.0f / (1.0f + den);
    if (ci < 0.0f) ci = 0.0f;
    if (ci > 1.0f) ci = 1.0f;
    c[idx] = ci;
}

extern "C" __global__ void srad2(
    float* __restrict__ J,
    const float* __restrict__ dN, const float* __restrict__ dS,
    const float* __restrict__ dW, const float* __restrict__ dE,
    const float* __restrict__ c,
    int rows, int cols, float lambda)
{
    int col = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    int row = blockIdx.y * BLOCK_SIZE + threadIdx.y;

    if (row >= rows || col >= cols) return;

    int idx = row * cols + col;

    int iS = (row < rows - 1) ? (row + 1) : (rows - 1);
    int jE = (col < cols - 1) ? (col + 1) : (cols - 1);

    float cN = c[idx];
    float cS = c[iS * cols + col];
    float cW = c[idx];
    float cE = c[row * cols + jE];

    float D = cN * dN[idx] + cS * dS[idx] + cW * dW[idx] + cE * dE[idx];

    J[idx] += 0.25f * lambda * D;
}
