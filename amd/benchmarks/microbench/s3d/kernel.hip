#include <hip/hip_runtime.h>

// S3D chemical kinetics: compute Arrhenius reaction rates per grid cell.
// k_i = A_i * T^n_i * exp(-E_i / (R * T)) for each species i.
// Each thread processes one cell, computing rates for numSpecies species.
__global__ void s3d_kernel(
    const float* __restrict__ T,
    const float* __restrict__ C,
    float* __restrict__ rates,
    int N,
    int numSpecies) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid >= N) return;

    float temp = T[gid];
    float Rgas = 8.314f;
    float invRT = 1.0f / (Rgas * temp);

    int base = gid * numSpecies;

    // Pre-factors and activation energies for representative combustion species
    // Using simplified Arrhenius parameters
    for (int s = 0; s < numSpecies; s++) {
        // Vary parameters per species for realistic workload
        float A = 1.0e10f + (float)s * 1.0e8f;
        float n = 0.5f + (float)s * 0.1f;
        float Ea = 20000.0f + (float)s * 5000.0f;

        float k = A * powf(temp, n) * expf(-Ea * invRT);
        float conc = C[base + s];

        rates[base + s] = k * conc;
    }
}
