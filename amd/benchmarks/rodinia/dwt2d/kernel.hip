#include <hip/hip_runtime.h>

// Kernel: apply Haar wavelet transform along rows
__global__ void dwt_row(const float *input, float *output, int width,
                        int height, int level_width) {
    int row = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;
    int col = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    int half = level_width / 2;
    if (row >= height || col >= half) return;

    int idx_a = row * width + 2 * col;
    int idx_b = row * width + 2 * col + 1;

    float a = input[idx_a];
    float b = input[idx_b];

    // Low-frequency (average) in left half
    output[row * width + col] = (a + b) * 0.5f;
    // High-frequency (difference) in right half
    output[row * width + half + col] = (a - b) * 0.5f;
}

// Kernel: apply Haar wavelet transform along columns
__global__ void dwt_col(const float *input, float *output, int width,
                        int height, int level_height) {
    int row = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;
    int col = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    int half = level_height / 2;
    if (row >= half || col >= width) return;

    int idx_a = (2 * row) * width + col;
    int idx_b = (2 * row + 1) * width + col;

    float a = input[idx_a];
    float b = input[idx_b];

    // Low-frequency (average) in top half
    output[row * width + col] = (a + b) * 0.5f;
    // High-frequency (difference) in bottom half
    output[(half + row) * width + col] = (a - b) * 0.5f;
}
