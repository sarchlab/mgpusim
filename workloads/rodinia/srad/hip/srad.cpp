// srad.cpp -- HIP benchmark for Rodinia SRAD
// Speckle Reducing Anisotropic Diffusion (image processing).
// 2 kernels: srad1 (compute diffusion coefficients) and srad2 (update image).

#include "bench_common_hip.h"

// Kernel 1: compute diffusion coefficients
__global__ void srad1(float *J, float *dN, float *dS, float *dW, float *dE,
                      float *c, int rows, int cols, float q0sqr) {
    int col = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int row = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (row >= rows || col >= cols) return;

    int idx = row * cols + col;

    // Neighbors with clamping at boundaries
    int iN = (row > 0) ? (row - 1) : 0;
    int iS = (row < rows - 1) ? (row + 1) : (rows - 1);
    int jW = (col > 0) ? (col - 1) : 0;
    int jE = (col < cols - 1) ? (col + 1) : (cols - 1);

    float Jc = J[idx];

    // Directional derivatives
    dN[idx] = J[iN * cols + col] - Jc;
    dS[idx] = J[iS * cols + col] - Jc;
    dW[idx] = J[row * cols + jW] - Jc;
    dE[idx] = J[row * cols + jE] - Jc;

    float G2 = (dN[idx] * dN[idx] + dS[idx] * dS[idx] +
                dW[idx] * dW[idx] + dE[idx] * dE[idx]) / (Jc * Jc);
    float L = (dN[idx] + dS[idx] + dW[idx] + dE[idx]) / Jc;
    float num = (0.5f * G2) - ((1.0f / 16.0f) * (L * L));
    float den = 1.0f + (0.25f * L);
    float qsqr = num / (den * den);

    // Diffusion coefficient (Perona-Malik)
    den = (qsqr - q0sqr) / (q0sqr * (1.0f + q0sqr));
    c[idx] = 1.0f / (1.0f + den);
    if (c[idx] < 0.0f) c[idx] = 0.0f;
    if (c[idx] > 1.0f) c[idx] = 1.0f;
}

// Kernel 2: update image using diffusion coefficients
__global__ void srad2(float *J, float *dN, float *dS, float *dW, float *dE,
                      float *c, int rows, int cols, float lambda) {
    int col = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int row = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (row >= rows || col >= cols) return;

    int idx = row * cols + col;

    // Neighbor diffusion coefficients
    int iS = (row < rows - 1) ? (row + 1) : (rows - 1);
    int jE = (col < cols - 1) ? (col + 1) : (cols - 1);

    float cN = c[idx];
    float cS = c[iS * cols + col];
    float cW = c[idx];
    float cE = c[row * cols + jE];

    // Divergence
    float D = cN * dN[idx] + cS * dS[idx] + cW * dW[idx] + cE * dE[idx];

    // Update
    J[idx] += 0.25f * lambda * D;
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int image_size = parseIntParam(argc, argv, "--image_size", 512);
    int num_iters = parseIntParam(argc, argv, "--num_iterations", 10);
    int block_size = parseIntParam(argc, argv, "--block_size", 16);

    // Parse lambda as float
    float lambda = 0.25f;
    for (int i = 1; i < argc - 1; i++) {
        if (strcmp(argv[i], "--lambda") == 0) {
            lambda = atof(argv[i + 1]);
            break;
        }
    }

    int rows = image_size;
    int cols = image_size;
    int size = rows * cols;

    // Generate image data
    float *h_J = (float *)malloc(size * sizeof(float));
    srand(42);
    for (int i = 0; i < size; i++) {
        h_J[i] = (rand() / (float)RAND_MAX) * 255.0f + 1.0f;
    }

    // Save original for re-init
    float *h_J_orig = (float *)malloc(size * sizeof(float));
    memcpy(h_J_orig, h_J, size * sizeof(float));

    // Device allocations
    float *d_J, *d_dN, *d_dS, *d_dW, *d_dE, *d_c;
    HIP_CHECK(hipMalloc(&d_J, size * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_dN, size * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_dS, size * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_dW, size * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_dE, size * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_c, size * sizeof(float)));

    dim3 block2d(block_size, block_size);
    dim3 grid2d((cols + block_size - 1) / block_size,
                (rows + block_size - 1) / block_size);

    // q0sqr: variance of uniform noise
    float q0sqr = 0.05f;

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", image_size);

    printCSVHeader();

    BenchResult r = runBenchmark("srad", problemSize, iterations, [&]() {
        // Copy image to device
        HIP_CHECK(hipMemcpy(d_J, h_J_orig, size * sizeof(float),
                            hipMemcpyHostToDevice));

        for (int iter = 0; iter < num_iters; iter++) {
            srad1<<<grid2d, block2d>>>(d_J, d_dN, d_dS, d_dW, d_dE, d_c,
                                        rows, cols, q0sqr);
            HIP_CHECK(hipDeviceSynchronize());

            srad2<<<grid2d, block2d>>>(d_J, d_dN, d_dS, d_dW, d_dE, d_c,
                                        rows, cols, lambda);
            HIP_CHECK(hipDeviceSynchronize());
        }
    });
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_J));
    HIP_CHECK(hipFree(d_dN));
    HIP_CHECK(hipFree(d_dS));
    HIP_CHECK(hipFree(d_dW));
    HIP_CHECK(hipFree(d_dE));
    HIP_CHECK(hipFree(d_c));
    free(h_J);
    free(h_J_orig);

    return 0;
}
