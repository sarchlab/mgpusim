// GPU Benchmarks v1.1, tier2/tango_binomial_options/hip/tango_binomial_options.hip.
#include <hip/hip_runtime.h>
#include <cmath>

struct OptionData {
    float S, K, T, r, sigma;
};

extern __shared__ float shared_values[];

extern "C" __global__ void binomial_kernel(
    const OptionData* __restrict__ options,
    float*            __restrict__ prices,
    int               numSteps)
{
    int optIdx = blockIdx.x;
    int tid    = threadIdx.x;
    int bdim   = blockDim.x;

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

    // Double-buffer the tree level. A single in-place
    // buffer races: computing node j reads shared_values[j+1] while the thread
    // owning node j+1 overwrites it in the same sweep — only one barrier per
    // level, so on sm_70+ this corrupts even within a warp. `cur` is read-only
    // for the level, `nxt` is written, then they swap.
    float* cur = shared_values;
    float* nxt = shared_values + numNodes;

    // Step 1: Compute terminal payoffs (stride loop)
    for (int j = tid; j < numNodes; j += bdim) {
        float ST = opt.S * powf(u, (float)(2 * j - numSteps));
        float payoff = fmaxf(opt.K - ST, 0.0f);
        cur[j] = payoff;
    }
    __syncthreads();

    // Step 2: Backward induction
    for (int step = numSteps; step > 0; --step) {
        for (int j = tid; j < step; j += bdim) {
            // Expected value discounted
            float cont = Rinv * (p * cur[j + 1] + q * cur[j]);
            // American option: can exercise early
            float ST = opt.S * powf(u, (float)(2 * j - (step - 1)));
            float exercise = fmaxf(opt.K - ST, 0.0f);
            nxt[j] = fmaxf(cont, exercise);
        }
        __syncthreads();
        float* tmp = cur; cur = nxt; nxt = tmp;
    }

    // Thread 0 writes the option price (final result lives in `cur` after the
    // last swap).
    if (tid == 0) {
        prices[optIdx] = cur[0];
    }
}
