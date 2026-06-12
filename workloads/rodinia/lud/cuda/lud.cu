// lud.cu -- CUDA benchmark for Rodinia LU Decomposition
// In-place LU decomposition using 3 kernels:
// lud_diagonal, lud_perimeter, lud_internal.

#include "bench_common_cuda.h"

// Diagonal block kernel: process the diagonal block in-place
__global__ void lud_diagonal(float *m, int matrix_dim, int offset) {
    int tx = threadIdx.x;
    __shared__ float shadow[32][32];

    int array_offset = offset * matrix_dim + offset;

    // Load diagonal block into shared memory
    for (int i = 0; i < 32; i++) {
        if (offset + i < matrix_dim && offset + tx < matrix_dim) {
            shadow[i][tx] = m[array_offset + i * matrix_dim + tx];
        } else {
            shadow[i][tx] = 0.0f;
        }
    }
    __syncthreads();

    // LU factorization on the diagonal block
    for (int i = 0; i < 32 - 1; i++) {
        if (tx > i) {
            // Compute L factor
            shadow[tx][i] /= shadow[i][i];
            for (int j = i + 1; j < 32; j++) {
                shadow[tx][j] -= shadow[tx][i] * shadow[i][j];
            }
        }
        __syncthreads();
    }

    // Write back
    for (int i = 0; i < 32; i++) {
        if (offset + i < matrix_dim && offset + tx < matrix_dim) {
            m[array_offset + i * matrix_dim + tx] = shadow[i][tx];
        }
    }
}

// Perimeter kernel: process row and column blocks touching the diagonal
__global__ void lud_perimeter(float *m, int matrix_dim, int offset) {
    int bx = blockIdx.x;
    int tx = threadIdx.x;

    __shared__ float dia[32][32];
    __shared__ float peri_row[32][32];
    __shared__ float peri_col[32][32];

    int array_offset = offset * matrix_dim + offset;

    // Load diagonal block
    for (int i = 0; i < 32; i++) {
        dia[i][tx] = m[array_offset + i * matrix_dim + tx];
    }
    __syncthreads();

    // Process row block
    int row_offset = array_offset + (bx + 1) * 32;
    for (int i = 0; i < 32; i++) {
        peri_row[i][tx] = m[row_offset + i * matrix_dim + tx];
    }

    for (int i = 0; i < 32; i++) {
        for (int j = 0; j < i; j++) {
            peri_row[i][tx] -= dia[i][j] * peri_row[j][tx];
        }
    }
    __syncthreads();

    for (int i = 0; i < 32; i++) {
        m[row_offset + i * matrix_dim + tx] = peri_row[i][tx];
    }

    // Process column block
    int col_offset = array_offset + (bx + 1) * 32 * matrix_dim;
    for (int i = 0; i < 32; i++) {
        peri_col[i][tx] = m[col_offset + i * matrix_dim + tx];
    }

    for (int i = 0; i < 32; i++) {
        for (int j = 0; j < i; j++) {
            peri_col[tx][i] -= peri_col[tx][j] * dia[j][i];
        }
        peri_col[tx][i] /= dia[i][i];
    }
    __syncthreads();

    for (int i = 0; i < 32; i++) {
        m[col_offset + i * matrix_dim + tx] = peri_col[i][tx];
    }
}

// Internal kernel: update remaining blocks
__global__ void lud_internal(float *m, int matrix_dim, int offset) {
    int bx = blockIdx.x;
    int by = blockIdx.y;
    int tx = threadIdx.x;
    int ty = threadIdx.y;

    int row = offset + (by + 1) * 32 + ty;
    int col = offset + (bx + 1) * 32 + tx;

    __shared__ float peri_row[32][32];
    __shared__ float peri_col[32][32];

    // Load perimeter blocks into shared memory
    peri_row[ty][tx] = m[(offset + ty) * matrix_dim + col];
    peri_col[ty][tx] = m[row * matrix_dim + (offset + tx)];
    __syncthreads();

    float sum = 0.0f;
    for (int k = 0; k < 32; k++) {
        sum += peri_col[ty][k] * peri_row[k][tx];
    }

    if (row < matrix_dim && col < matrix_dim) {
        m[row * matrix_dim + col] -= sum;
    }
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int matrix_size = parseIntParam(argc, argv, "--matrix_size", 256);
    int block_size_param = parseIntParam(argc, argv, "--block_size", 16);

    // For LUD, the internal block size is fixed at 32 for the kernel
    // The block_size_param affects the grid configuration
    int bs = 32; // Internal block size for LUD kernels

    // Ensure matrix_size is a multiple of bs
    if (matrix_size % bs != 0) {
        matrix_size = ((matrix_size + bs - 1) / bs) * bs;
    }

    int seed_val = parseIntParam(argc, argv, "--seed", 42);

    // Generate matrix data
    float *h_m = (float *)malloc(matrix_size * matrix_size * sizeof(float));

    srand(seed_val);
    // Generate a diagonally dominant matrix for numerical stability
    for (int i = 0; i < matrix_size; i++) {
        for (int j = 0; j < matrix_size; j++) {
            h_m[i * matrix_size + j] = (rand() / (float)RAND_MAX) * 2.0f -
                                        1.0f;
        }
        // Make diagonally dominant
        h_m[i * matrix_size + i] += (float)matrix_size;
    }

    // Save original for re-init between iterations
    float *h_m_orig = (float *)malloc(matrix_size * matrix_size *
                                       sizeof(float));
    memcpy(h_m_orig, h_m, matrix_size * matrix_size * sizeof(float));

    // Device allocation
    float *d_m;
    CUDA_CHECK(cudaMalloc(&d_m, matrix_size * matrix_size * sizeof(float)));

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", matrix_size);

    printCSVHeader();

    BenchResult r = runBenchmark("lud", problemSize, iterations, [&]() {
        // Copy matrix to device for each run
        CUDA_CHECK(cudaMemcpy(d_m, h_m_orig,
                              matrix_size * matrix_size * sizeof(float),
                              cudaMemcpyHostToDevice));

        int num_blocks = matrix_size / bs;

        for (int i = 0; i < num_blocks; i++) {
            int offset = i * bs;

            // Diagonal
            lud_diagonal<<<1, 32>>>(d_m, matrix_size, offset);
            CUDA_CHECK(cudaDeviceSynchronize());

            if (i < num_blocks - 1) {
                // Perimeter
                int peri_blocks = num_blocks - i - 1;
                lud_perimeter<<<peri_blocks, 32>>>(d_m, matrix_size,
                                                    offset);
                CUDA_CHECK(cudaDeviceSynchronize());

                // Internal
                dim3 internal_grid(peri_blocks, peri_blocks);
                dim3 internal_block(bs, bs);
                // Limit block size for internal kernel
                if (bs > 32) {
                    internal_block = dim3(32, 32);
                }
                lud_internal<<<internal_grid, internal_block>>>(
                    d_m, matrix_size, offset);
                CUDA_CHECK(cudaDeviceSynchronize());
            }
        }
    });
    printCSVRow(r);

    // Cleanup
    CUDA_CHECK(cudaFree(d_m));
    free(h_m);
    free(h_m_orig);

    return 0;
}
