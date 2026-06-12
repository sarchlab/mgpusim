#include <hip/hip_runtime.h>

// Diagonal block kernel: process the diagonal block in-place
__global__ void lud_diagonal(float *m, int matrix_dim, int offset) {
    int tx = hipThreadIdx_x;
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
    int bx = hipBlockIdx_x;
    int tx = hipThreadIdx_x;

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
    int bx = hipBlockIdx_x;
    int by = hipBlockIdx_y;
    int tx = hipThreadIdx_x;
    int ty = hipThreadIdx_y;

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
