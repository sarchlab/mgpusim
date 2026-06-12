#include <hip/hip_runtime.h>

// Tiled SGEMM implementation (device function, not a kernel entry point)
// Templates cannot use extern "C", so we keep this as a __device__ inline function.
template <int TILE_WIDTH>
__device__ void sgemm_tiled_impl(const float *A, const float *B, float *C,
                                  int N, float alpha, float beta) {
    __shared__ float tileA[TILE_WIDTH][TILE_WIDTH];
    __shared__ float tileB[TILE_WIDTH][TILE_WIDTH];

    int row = hipBlockIdx_y * TILE_WIDTH + hipThreadIdx_y;
    int col = hipBlockIdx_x * TILE_WIDTH + hipThreadIdx_x;

    float sum = 0.0f;
    int numTiles = (N + TILE_WIDTH - 1) / TILE_WIDTH;

    for (int t = 0; t < numTiles; t++) {
        // Load tile of A
        int aCol = t * TILE_WIDTH + hipThreadIdx_x;
        if (row < N && aCol < N)
            tileA[hipThreadIdx_y][hipThreadIdx_x] = A[row * N + aCol];
        else
            tileA[hipThreadIdx_y][hipThreadIdx_x] = 0.0f;

        // Load tile of B
        int bRow = t * TILE_WIDTH + hipThreadIdx_y;
        if (bRow < N && col < N)
            tileB[hipThreadIdx_y][hipThreadIdx_x] = B[bRow * N + col];
        else
            tileB[hipThreadIdx_y][hipThreadIdx_x] = 0.0f;

        __syncthreads();

        // Compute partial dot product for this tile
        for (int k = 0; k < TILE_WIDTH; k++) {
            sum += tileA[hipThreadIdx_y][k] * tileB[k][hipThreadIdx_x];
        }

        __syncthreads();
    }

    // Write result: C = alpha * A*B + beta * C
    if (row < N && col < N) {
        int idx = row * N + col;
        C[idx] = alpha * sum + beta * C[idx];
    }
}

// Non-template extern "C" wrapper so the simulator can find kernel by name.
// __global__ expands to: extern "C" __attribute__((amdgpu_kernel)) on AMDGCN.
__global__ void sgemm_tiled(const float *A, const float *B, float *C,
                             int N, float alpha, float beta) {
    sgemm_tiled_impl<16>(A, B, C, N, alpha, beta);
}
