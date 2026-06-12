#include <hip/hip_runtime.h>

// Lennard-Jones molecular dynamics: compute per-atom energy from neighbor list.
// F = 4*eps*(sigma^12/r^12 - sigma^6/r^6) summed over neighbors.
__global__ void md_lj_kernel(
    const float* __restrict__ posX,
    const float* __restrict__ posY,
    const float* __restrict__ posZ,
    float* __restrict__ energy,
    const int* __restrict__ neighborList,
    const int* __restrict__ numNeighbors,
    int N,
    int maxNeighbors) {
    int gid = blockIdx.x * blockDim.x + threadIdx.x;
    if (gid >= N) return;

    float xi = posX[gid];
    float yi = posY[gid];
    float zi = posZ[gid];

    float sigma6 = 1.0f;   // sigma^6 = 1.0 (reduced units)
    float eps4 = 4.0f;     // 4 * epsilon

    float eSum = 0.0f;
    int nNeigh = numNeighbors[gid];
    int base = gid * maxNeighbors;

    for (int k = 0; k < nNeigh; k++) {
        int j = neighborList[base + k];
        float dx = xi - posX[j];
        float dy = yi - posY[j];
        float dz = zi - posZ[j];

        float r2 = dx * dx + dy * dy + dz * dz;
        float r2inv = 1.0f / r2;
        float r6inv = r2inv * r2inv * r2inv;
        float r12inv = r6inv * r6inv;

        eSum += eps4 * (sigma6 * r12inv - sigma6 * r6inv);
    }

    energy[gid] = eSum;
}
