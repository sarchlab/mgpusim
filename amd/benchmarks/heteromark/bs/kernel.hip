#include <hip/hip_runtime.h>

// ---------------------------------------------------------------------------
// Device helper: Cumulative Normal Distribution (Abramowitz & Stegun approx)
// ---------------------------------------------------------------------------
__device__ float CND(float d) {
    float K = 1.0f / (1.0f + 0.2316419f * fabsf(d));
    float cnd = 0.3989422804f * expf(-0.5f * d * d) *
        (K * (0.319381530f + K * (-0.356563782f + K * (1.781477937f +
         K * (-1.821255978f + K * 1.330274429f)))));
    if (d > 0) cnd = 1.0f - cnd;
    return cnd;
}

// ---------------------------------------------------------------------------
// Kernel: Black-Scholes option pricing
// ---------------------------------------------------------------------------
__global__ void bs_kernel(const float* __restrict__ stockPrice,
                          const float* __restrict__ strikePrice,
                          const float* __restrict__ years,
                          float* __restrict__ callPrice,
                          float* __restrict__ putPrice,
                          float riskFreeRate,
                          float volatility,
                          int N) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (i >= N) return;

    float S = stockPrice[i];
    float X = strikePrice[i];
    float T = years[i];
    float R = riskFreeRate;
    float V = volatility;

    float sqrtT = sqrtf(T);
    float d1 = (logf(S / X) + (R + 0.5f * V * V) * T) / (V * sqrtT);
    float d2 = d1 - V * sqrtT;

    callPrice[i] = S * CND(d1) - X * expf(-R * T) * CND(d2);
    putPrice[i]  = X * expf(-R * T) * CND(-d2) - S * CND(-d1);
}
