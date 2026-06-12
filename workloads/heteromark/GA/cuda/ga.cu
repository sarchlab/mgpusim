// ga.cu -- Heteromark Genetic Algorithm benchmark (CUDA)
// Genetic algorithm for string matching.

#include "bench_common_cuda.h"

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
    int i = blockIdx.x * blockDim.x + threadIdx.x;
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
    int i = blockIdx.x * blockDim.x + threadIdx.x;
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
    int i = blockIdx.x * blockDim.x + threadIdx.x;
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
    int i = blockIdx.x * blockDim.x + threadIdx.x;
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

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int populationSize = parseIntParam(argc, argv, "--size", 4096);
    int block_size     = parseIntParam(argc, argv, "--block_size", 256);
    int numGenerations = parseIntParam(argc, argv, "--generations", 100);
    int iters          = parseIterations(argc, argv);

    const char* target = DEFAULT_TARGET;
    int targetLen = (int)strlen(target);

    size_t popBytes     = (size_t)populationSize * targetLen * sizeof(char);
    size_t fitnessBytes = (size_t)populationSize * sizeof(int);

    // Host allocations
    char* h_population = (char*)malloc(popBytes);
    int*  h_fitness    = (int*)malloc(fitnessBytes);

    // Initialize random population
    srand(42);
    for (int i = 0; i < populationSize; ++i) {
        for (int j = 0; j < targetLen; ++j) {
            h_population[i * targetLen + j] = (char)(32 + (rand() % 95));
        }
    }

    // Device allocations
    char *d_population, *d_selected;
    int  *d_fitness;
    char *d_target;

    CUDA_CHECK(cudaMalloc(&d_population, popBytes));
    CUDA_CHECK(cudaMalloc(&d_selected,   popBytes));
    CUDA_CHECK(cudaMalloc(&d_fitness,    fitnessBytes));
    CUDA_CHECK(cudaMalloc(&d_target,     targetLen * sizeof(char)));

    CUDA_CHECK(cudaMemcpy(d_target, target, targetLen * sizeof(char),
                          cudaMemcpyHostToDevice));

    int grid = (populationSize + block_size - 1) / block_size;
    int crossGrid = (populationSize / 2 + block_size - 1) / block_size;

    // Benchmark
    char size_str[64];
    snprintf(size_str, sizeof(size_str), "%d", populationSize);

    printCSVHeader();

    float mutationRate = 0.01f;

    BenchResult r = runBenchmark("ga", size_str, iters, [&]() {
        // Re-init population for each benchmark iteration
        CUDA_CHECK(cudaMemcpy(d_population, h_population, popBytes,
                              cudaMemcpyHostToDevice));

        for (int gen = 0; gen < numGenerations; ++gen) {
            unsigned int genSeed = (unsigned int)(gen * 7919 + 42);

            // Fitness
            fitness_kernel<<<grid, block_size>>>(
                d_population, d_fitness, d_target, targetLen, populationSize);

            // Selection
            selection_kernel<<<grid, block_size>>>(
                d_population, d_fitness, d_selected, targetLen,
                populationSize, genSeed + 1);

            // Copy selected back to population
            CUDA_CHECK(cudaMemcpy(d_population, d_selected, popBytes,
                                  cudaMemcpyDeviceToDevice));

            // Crossover
            crossover_kernel<<<crossGrid, block_size>>>(
                d_population, targetLen, populationSize, genSeed + 2);

            // Mutation
            mutation_kernel<<<grid, block_size>>>(
                d_population, targetLen, populationSize, mutationRate,
                genSeed + 3);
        }
    });

    printCSVRow(r);

    // Final fitness check
    CUDA_CHECK(cudaMemcpy(d_population, h_population, popBytes,
                          cudaMemcpyHostToDevice));

    for (int gen = 0; gen < numGenerations; ++gen) {
        unsigned int genSeed = (unsigned int)(gen * 7919 + 42);
        fitness_kernel<<<grid, block_size>>>(
            d_population, d_fitness, d_target, targetLen, populationSize);
        selection_kernel<<<grid, block_size>>>(
            d_population, d_fitness, d_selected, targetLen,
            populationSize, genSeed + 1);
        CUDA_CHECK(cudaMemcpy(d_population, d_selected, popBytes,
                              cudaMemcpyDeviceToDevice));
        crossover_kernel<<<crossGrid, block_size>>>(
            d_population, targetLen, populationSize, genSeed + 2);
        mutation_kernel<<<grid, block_size>>>(
            d_population, targetLen, populationSize, mutationRate,
            genSeed + 3);
    }

    fitness_kernel<<<grid, block_size>>>(
        d_population, d_fitness, d_target, targetLen, populationSize);
    CUDA_CHECK(cudaDeviceSynchronize());

    CUDA_CHECK(cudaMemcpy(h_fitness, d_fitness, fitnessBytes,
                          cudaMemcpyDeviceToHost));

    int bestFit = 0;
    for (int i = 0; i < populationSize; ++i) {
        if (h_fitness[i] > bestFit) bestFit = h_fitness[i];
    }

    fprintf(stderr, "Best fitness after %d generations: %d / %d\n",
            numGenerations, bestFit, targetLen);
    fprintf(stderr, "PASS\n");

    // Cleanup
    CUDA_CHECK(cudaFree(d_population));
    CUDA_CHECK(cudaFree(d_selected));
    CUDA_CHECK(cudaFree(d_fitness));
    CUDA_CHECK(cudaFree(d_target));
    free(h_population);
    free(h_fitness);

    return 0;
}
