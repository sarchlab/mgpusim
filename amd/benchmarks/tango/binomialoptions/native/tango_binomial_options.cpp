/*
 * HIP kernel for Tango Binomial Options (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/tango_binomial_options.
 *
 * Cox-Ross-Rubinstein (CRR) binomial tree model for American put option
 * pricing. One threadblock processes one option; threads collaborate on
 * backward induction through the tree using shared memory.
 *
 * Differences from the original HIP source, to keep the kernarg layout
 * simple (no hidden ABI args) and use only statically-sized LDS:
 *   - A constant compile-time BLOCK_SIZE is used instead of blockDim.x.
 *   - Shared memory is a static array of MAX_NODES floats instead of a
 *     dynamically-sized extern __shared__ buffer (so no LDS-size kernarg).
 * The numerical algorithm is byte-for-byte identical, so the CPU reference
 * in the Go driver reproduces the result exactly. The caller must launch
 * with block dim {BLOCK_SIZE,1,1} and numSteps+1 <= MAX_NODES.
 */
#include "hip/hip_runtime.h"

#define BLOCK_SIZE 256
#define MAX_NODES  256

struct OptionData {
    float S;      // stock price
    float K;      // strike price
    float T;      // time to expiration
    float r;      // risk-free rate
    float sigma;  // volatility
};

extern "C" __global__ void binomial_kernel(
    const OptionData* __restrict__ options,
    float*            __restrict__ prices,
    int               numSteps)
{
    __shared__ float shared_values[MAX_NODES];

    int optIdx = blockIdx.x;
    int tid    = threadIdx.x;
    int bdim   = BLOCK_SIZE;

    OptionData opt = options[optIdx];

    // CRR parameters
    float dt = opt.T / (float)numSteps;
    float u  = expf(opt.sigma * sqrtf(dt));        // up factor
    float d  = 1.0f / u;                           // down factor
    float R  = expf(opt.r * dt);                   // risk-free growth
    float Rinv = 1.0f / R;
    float p  = (R - d) / (u - d);                  // risk-neutral probability
    float q  = 1.0f - p;

    int numNodes = numSteps + 1;

    // Step 1: Compute terminal payoffs (stride loop)
    for (int j = tid; j < numNodes; j += bdim) {
        float ST = opt.S * powf(u, (float)(2 * j - numSteps));
        float payoff = fmaxf(opt.K - ST, 0.0f);
        shared_values[j] = payoff;
    }
    __syncthreads();

    // Step 2: Backward induction
    for (int step = numSteps; step > 0; --step) {
        for (int j = tid; j < step; j += bdim) {
            float cont = Rinv * (p * shared_values[j + 1] + q * shared_values[j]);
            float ST = opt.S * powf(u, (float)(2 * j - (step - 1)));
            float exercise = fmaxf(opt.K - ST, 0.0f);
            shared_values[j] = fmaxf(cont, exercise);
        }
        __syncthreads();
    }

    // Thread 0 writes the option price
    if (tid == 0) {
        prices[optIdx] = shared_values[0];
    }
}
