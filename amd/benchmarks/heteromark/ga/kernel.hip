#include <hip/hip_runtime.h>

#define DEFAULT_TARGET "Hello World!"
#define MAX_TARGET_LEN 128

// ---------------------------------------------------------------------------
// Kernel: Compute fitness (number of matching characters)
// ---------------------------------------------------------------------------
__global__ void fitness_kernel(const char* __restrict__ population,
                               int* __restrict__ fitness,
                               const char* __restrict__ target,
                               int targetLen,
                               int populationSize) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (i >= populationSize) return;

    int fit = 0;
    for (int j = 0; j < targetLen; ++j) {
        if (population[i * targetLen + j] == target[j]) {
            fit++;
        }
    }
    fitness[i] = fit;
}

// ---------------------------------------------------------------------------
// Kernel: Tournament selection
// ---------------------------------------------------------------------------
__global__ void selection_kernel(const char* __restrict__ population,
                                 const int* __restrict__ fitness,
                                 char* __restrict__ selected,
                                 int targetLen,
                                 int populationSize,
                                 unsigned int seed) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (i >= populationSize) return;

    // Simple PRNG
    unsigned int rng = seed + i * 1103515245u;
    rng = (rng * 1103515245u + 12345u) & 0x7fffffffu;
    int a = rng % populationSize;
    rng = (rng * 1103515245u + 12345u) & 0x7fffffffu;
    int b = rng % populationSize;

    // Tournament: pick the fitter one
    int winner = (fitness[a] >= fitness[b]) ? a : b;

    for (int j = 0; j < targetLen; ++j) {
        selected[i * targetLen + j] = population[winner * targetLen + j];
    }
}

// ---------------------------------------------------------------------------
// Kernel: Single-point crossover
// ---------------------------------------------------------------------------
__global__ void crossover_kernel(char* __restrict__ population,
                                 int targetLen,
                                 int populationSize,
                                 unsigned int seed) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    // Process pairs: thread i handles pair (2i, 2i+1)
    int pair = i;
    int idx1 = pair * 2;
    int idx2 = pair * 2 + 1;
    if (idx2 >= populationSize) return;

    // PRNG for crossover point
    unsigned int rng = seed + i * 1103515245u;
    rng = (rng * 1103515245u + 12345u) & 0x7fffffffu;
    int crossPoint = (rng % (targetLen - 1)) + 1;

    // Swap from crossover point onward
    for (int j = crossPoint; j < targetLen; ++j) {
        char tmp = population[idx1 * targetLen + j];
        population[idx1 * targetLen + j] = population[idx2 * targetLen + j];
        population[idx2 * targetLen + j] = tmp;
    }
}

// ---------------------------------------------------------------------------
// Kernel: Random character mutation
// ---------------------------------------------------------------------------
__global__ void mutation_kernel(char* __restrict__ population,
                                int targetLen,
                                int populationSize,
                                float mutationRate,
                                unsigned int seed) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (i >= populationSize) return;

    unsigned int rng = seed + i * 1103515245u;

    for (int j = 0; j < targetLen; ++j) {
        rng = (rng * 1103515245u + 12345u) & 0x7fffffffu;
        float r = (float)(rng % 10000) / 10000.0f;
        if (r < mutationRate) {
            rng = (rng * 1103515245u + 12345u) & 0x7fffffffu;
            // Printable ASCII range: 32-126
            population[i * targetLen + j] = (char)(32 + (rng % 95));
        }
    }
}
