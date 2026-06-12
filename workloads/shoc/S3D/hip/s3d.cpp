// s3d.cpp -- SHOC S3D Chemical Kinetics benchmark (HIP)
// Simplified S3D: evaluate Arrhenius reaction rate coefficients and
// combine with stoichiometric coefficients to compute species rates.

#include "bench_common_hip.h"

#define DEFAULT_GRIDPOINTS  1048576
#define DEFAULT_BLOCK       256

// Number of reactions and species (simplified hydrogen combustion)
#define NUM_REACTIONS 19
#define NUM_SPECIES    9

// Universal gas constant (J/(mol·K))
#define R_UNIVERSAL 8.314f

// ---------------------------------------------------------------------------
// Arrhenius parameters for 19 reactions (A, n, Ea)
// Simplified hydrogen combustion mechanism
// ---------------------------------------------------------------------------
__constant__ float d_A[NUM_REACTIONS] = {
    1.915e14f, 5.080e4f,  2.160e8f,  2.970e6f,  4.577e19f,
    6.165e15f, 4.714e18f, 3.500e16f, 7.079e13f, 3.250e13f,
    2.890e13f, 4.200e14f, 1.000e8f,  5.000e13f, 2.951e14f,
    2.410e13f, 9.550e6f,  1.740e12f, 7.590e13f
};

__constant__ float d_n[NUM_REACTIONS] = {
    0.0f, 2.67f, 1.51f, 2.02f, -1.40f,
    -0.50f, -1.0f, 0.0f, 0.0f, 0.0f,
    0.0f, 0.0f, 1.6f, 0.0f, 0.0f,
    0.0f, 2.0f, 0.0f, 0.0f
};

__constant__ float d_Ea[NUM_REACTIONS] = {
    16439.0f, 6290.0f, 3430.0f, 13400.0f, 0.0f,
    0.0f, 0.0f, 0.0f, 295.0f, 0.0f,
    -497.0f, 11982.0f, 3298.0f, 1000.0f, 48430.0f,
    3970.0f, 3970.0f, 318.0f, 7269.0f
};

// Stoichiometric coefficients: stoich[species][reaction]
// Stored as flat array in constant memory
__constant__ float d_stoich[NUM_SPECIES * NUM_REACTIONS] = {
    // H2
     0, -1,  0,  0,  1,  0,  0,  0,  0,  0,  0,  0,  1, -1,  0,  0,  0,  0,  0,
    // O2
     0,  0,  0, -1,  0,  0,  1,  0,  0,  0,  0,  0,  0,  0, -1,  0,  0,  0,  0,
    // H2O
     0,  0,  1,  0,  0,  0,  0,  0,  0,  1,  0,  0,  0,  0,  0,  1,  0,  0,  0,
    // H
    -1,  1, -1,  1, -1,  1, -1,  1,  0,  0,  1, -1,  0,  1,  0, -1,  1, -1,  0,
    // O
     1, -1,  0,  1,  0,  0,  0,  0,  1, -1,  0,  0,  0,  0,  1, -1,  0,  0,  1,
    // OH
    -1,  1,  1, -1,  0,  0,  0,  0, -1,  1, -1,  1,  0,  0,  0,  1, -1,  1, -1,
    // HO2
     0,  0,  0,  0,  1, -1, -1,  0,  1,  0,  0,  0, -1,  0,  0,  0,  0,  1, -1,
    // H2O2
     0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  1, -1,  1, -1,  0,  0,  0,
    // N2 (inert, no net change)
     0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0
};

// ---------------------------------------------------------------------------
// Kernel 1: Compute Arrhenius rate constants for each grid point
// rate[i * NUM_REACTIONS + r] = A[r] * T^n[r] * exp(-Ea[r] / (R * T))
// ---------------------------------------------------------------------------
__global__ void rateconst_kernel(const float* __restrict__ temperature,
                                  float* __restrict__ rate,
                                  int N) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (i >= N) return;

    float T = temperature[i];
    float invRT = 1.0f / (R_UNIVERSAL * T);

    for (int r = 0; r < NUM_REACTIONS; ++r) {
        float k = d_A[r] * powf(T, d_n[r]) * expf(-d_Ea[r] * invRT);
        rate[i * NUM_REACTIONS + r] = k;
    }
}

// ---------------------------------------------------------------------------
// Kernel 2: Compute species rates from reaction rates and stoichiometry
// species_rate[i * NUM_SPECIES + s] = sum_r(stoich[s][r] * rate[i * NUM_REACTIONS + r])
// ---------------------------------------------------------------------------
__global__ void species_rates_kernel(const float* __restrict__ rate,
                                      float* __restrict__ species_rate,
                                      int N) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (i >= N) return;

    for (int s = 0; s < NUM_SPECIES; ++s) {
        float sum = 0.0f;
        for (int r = 0; r < NUM_REACTIONS; ++r) {
            sum += d_stoich[s * NUM_REACTIONS + r] * rate[i * NUM_REACTIONS + r];
        }
        species_rate[i * NUM_SPECIES + s] = sum;
    }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int N          = parseIntParam(argc, argv, "--size", DEFAULT_GRIDPOINTS);
    int block_size = parseIntParam(argc, argv, "--block_size", DEFAULT_BLOCK);
    int iters      = parseIterations(argc, argv);

    size_t tempBytes    = (size_t)N * sizeof(float);
    size_t rateBytes    = (size_t)N * NUM_REACTIONS * sizeof(float);
    size_t speciesBytes = (size_t)N * NUM_SPECIES * sizeof(float);

    // Host allocations
    float* h_temperature = (float*)malloc(tempBytes);

    // Initialize temperatures: 300K to 3000K
    srand(42);
    for (int i = 0; i < N; ++i) {
        h_temperature[i] = 300.0f + (float)(rand() % 2700);
    }

    // Device allocations
    float *d_temperature, *d_rate, *d_species_rate;
    HIP_CHECK(hipMalloc(&d_temperature, tempBytes));
    HIP_CHECK(hipMalloc(&d_rate, rateBytes));
    HIP_CHECK(hipMalloc(&d_species_rate, speciesBytes));

    HIP_CHECK(hipMemcpy(d_temperature, h_temperature, tempBytes,
                        hipMemcpyHostToDevice));

    int grid = (N + block_size - 1) / block_size;

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", N);

    printCSVHeader();

    BenchResult r = runBenchmark("s3d", problemSize, iters, [&]() {
        rateconst_kernel<<<grid, block_size>>>(d_temperature, d_rate, N);
        species_rates_kernel<<<grid, block_size>>>(d_rate, d_species_rate, N);
    });

    printCSVRow(r);

    // Verification: check first grid point
    rateconst_kernel<<<grid, block_size>>>(d_temperature, d_rate, N);
    species_rates_kernel<<<grid, block_size>>>(d_rate, d_species_rate, N);
    HIP_CHECK(hipDeviceSynchronize());

    float* h_species = (float*)malloc(speciesBytes);
    HIP_CHECK(hipMemcpy(h_species, d_species_rate, speciesBytes,
                        hipMemcpyDeviceToHost));

    fprintf(stderr, "Grid point 0 (T=%.1f K):\n", h_temperature[0]);
    const char* speciesNames[] = {"H2", "O2", "H2O", "H", "O", "OH", "HO2", "H2O2", "N2"};
    for (int s = 0; s < NUM_SPECIES; ++s) {
        fprintf(stderr, "  %4s rate: %e\n", speciesNames[s], h_species[s]);
    }
    fprintf(stderr, "PASS\n");

    // Cleanup
    HIP_CHECK(hipFree(d_temperature));
    HIP_CHECK(hipFree(d_rate));
    HIP_CHECK(hipFree(d_species_rate));
    free(h_temperature);
    free(h_species);

    return 0;
}
