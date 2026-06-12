// dwt2d.cpp -- HIP benchmark for Rodinia 2D Discrete Wavelet Transform
// Applies Haar wavelet transform to rows then columns of an NxN image.
// Supports multiple decomposition levels.

#include "bench_common_hip.h"

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

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int image_size = parseIntParam(argc, argv, "--image_size", 1024);
    int levels = parseIntParam(argc, argv, "--levels", 3);
    int block_sz = parseIntParam(argc, argv, "--block_size", 128);
    // wavelet_type is accepted but Haar is always used (type 1)
    (void)parseIntParam(argc, argv, "--wavelet_type", 1);

    int width = image_size;
    int height = image_size;
    size_t total = (size_t)width * height;

    // Generate random image data
    float *h_image = (float *)malloc(total * sizeof(float));
    srand(42);
    for (size_t i = 0; i < total; i++) {
        h_image[i] = (float)(rand() % 10000) / 100.0f;
    }

    // Device allocations -- two buffers for ping-pong
    float *d_input, *d_output;
    HIP_CHECK(hipMalloc(&d_input, total * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_output, total * sizeof(float)));

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%dx%d", width, height);

    printCSVHeader();

    BenchResult r = runBenchmark("dwt2d", problemSize, iterations, [&]() {
        // Upload image
        HIP_CHECK(hipMemcpy(d_input, h_image, total * sizeof(float),
                            hipMemcpyHostToDevice));

        int lw = width;
        int lh = height;

        for (int l = 0; l < levels; l++) {
            if (lw < 2 || lh < 2) break;

            // Row transform
            {
                int half = lw / 2;
                dim3 block2d(block_sz, 1);
                dim3 grid2d((half + block2d.x - 1) / block2d.x,
                            (lh + block2d.y - 1) / block2d.y);
                dwt_row<<<grid2d, block2d>>>(d_input, d_output, width,
                                             lh, lw);
                HIP_CHECK(hipDeviceSynchronize());
            }

            // Copy result back to input for column pass
            HIP_CHECK(hipMemcpy(d_input, d_output, total * sizeof(float),
                                hipMemcpyDeviceToDevice));

            // Column transform
            {
                int half = lh / 2;
                dim3 block2d(block_sz, 1);
                dim3 grid2d((lw + block2d.x - 1) / block2d.x,
                            (half + block2d.y - 1) / block2d.y);
                dwt_col<<<grid2d, block2d>>>(d_input, d_output, width,
                                             lh, lh);
                HIP_CHECK(hipDeviceSynchronize());
            }

            // Copy result back to input for next level
            HIP_CHECK(hipMemcpy(d_input, d_output, total * sizeof(float),
                                hipMemcpyDeviceToDevice));

            lw /= 2;
            lh /= 2;
        }
    });
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_input));
    HIP_CHECK(hipFree(d_output));
    free(h_image);

    return 0;
}
