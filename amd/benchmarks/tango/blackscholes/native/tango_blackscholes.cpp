/*
 * HIP kernel for Tango Black-Scholes (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/tango_blackscholes.
 *
 * Each thread prices one European option (call + put) with the
 * Black-Scholes closed-form formula, using a polynomial approximation
 * of the cumulative normal distribution (Abramowitz & Stegun 26.2.17).
 *
 * Uses a constant BLOCK_SIZE (not blockDim) for the block geometry so the
 * compiler emits no hidden ABI arguments.
 */
#include "hip/hip_runtime.h"

#define BLOCK_SIZE 256

__device__ inline float cnd(float d) {
    const float A1 = 0.31938153f;
    const float A2 = -0.356563782f;
    const float A3 = 1.781477937f;
    const float A4 = -1.821255978f;
    const float A5 = 1.330274429f;
    const float RSQRT2PI = 0.39894228040143267793994605993438f;

    float K = 1.0f / (1.0f + 0.2316419f * fabsf(d));
    float cnd_val = RSQRT2PI * expf(-0.5f * d * d) *
                    (K * (A1 + K * (A2 + K * (A3 + K * (A4 + K * A5)))));

    if (d > 0.0f) cnd_val = 1.0f - cnd_val;
    return cnd_val;
}

extern "C" __global__ void blackscholes_kernel(
    const float* __restrict__ S,       // stock price
    const float* __restrict__ K,       // strike price
    const float* __restrict__ T,       // time to expiration
    const float* __restrict__ sigma,   // volatility
    float        r,                    // risk-free rate
    float*       callPrice,
    float*       putPrice,
    int          N)
{
    int idx = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    if (idx >= N) return;

    float s  = S[idx];
    float k  = K[idx];
    float t  = T[idx];
    float v  = sigma[idx];

    float sqrtT  = sqrtf(t);
    float d1     = (logf(s / k) + (r + 0.5f * v * v) * t) / (v * sqrtT);
    float d2     = d1 - v * sqrtT;

    float expRT  = expf(-r * t);
    float cnd_d1 = cnd(d1);
    float cnd_d2 = cnd(d2);

    callPrice[idx] = s * cnd_d1 - k * expRT * cnd_d2;
    putPrice[idx]  = k * expRT * (1.0f - cnd_d2) - s * (1.0f - cnd_d1);
}
