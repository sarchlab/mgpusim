// mri-gridding.cpp -- HIP benchmark for Parboil MRI Reconstruction via Gridding
// Kernel: Scatter data from non-uniform k-space samples onto a Cartesian grid
// using a Kaiser-Bessel window. Each thread processes one sample and uses
// atomicAdd to contribute to nearby grid points.

#include "bench_common_hip.h"

#ifndef M_PI
#define M_PI 3.14159265358979323846f
#endif

// Kaiser-Bessel window function approximation
__device__ float kaiser_bessel(float dist, float width, float beta) {
    float x = dist / (width * 0.5f);
    if (fabsf(x) >= 1.0f) return 0.0f;
    float arg = beta * sqrtf(1.0f - x * x);
    // Approximate I0(arg) using a polynomial for the modified Bessel function
    // I0(x) ≈ 1 + (x/2)^2 + (x/2)^4/4 + (x/2)^6/36 + ...
    float half_arg = arg * 0.5f;
    float ha2 = half_arg * half_arg;
    float i0 = 1.0f + ha2 * (1.0f + ha2 * (0.25f + ha2 * (1.0f/36.0f + ha2 * (1.0f/576.0f + ha2 * (1.0f/14400.0f)))));
    return i0;
}

// Gridding kernel: scatter each sample's contribution to nearby grid points
// samples_x, samples_y: k-space sample coordinates (normalized to grid)
// samples_real, samples_imag: complex sample values
// grid_real, grid_imag: output Cartesian grid (complex)
// num_samples: number of k-space samples
// grid_size: dimension of the output grid (grid_size x grid_size)
// kernel_width: width of the Kaiser-Bessel window in grid cells
// beta: Kaiser-Bessel beta parameter
__global__ void gridding_kernel(float *samples_x, float *samples_y,
                                float *samples_real, float *samples_imag,
                                float *grid_real, float *grid_imag,
                                int num_samples, int grid_size,
                                int kernel_width, float beta) {
    int sid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (sid >= num_samples) return;

    float sx = samples_x[sid];
    float sy = samples_y[sid];
    float sr = samples_real[sid];
    float si = samples_imag[sid];

    float half_width = kernel_width * 0.5f;

    // Determine the range of grid points this sample contributes to
    int gx_min = (int)floorf(sx - half_width);
    int gx_max = (int)ceilf(sx + half_width);
    int gy_min = (int)floorf(sy - half_width);
    int gy_max = (int)ceilf(sy + half_width);

    // Clamp to grid bounds
    if (gx_min < 0) gx_min = 0;
    if (gy_min < 0) gy_min = 0;
    if (gx_max >= grid_size) gx_max = grid_size - 1;
    if (gy_max >= grid_size) gy_max = grid_size - 1;

    for (int gy = gy_min; gy <= gy_max; gy++) {
        float dy = (float)gy - sy;
        for (int gx = gx_min; gx <= gx_max; gx++) {
            float dx = (float)gx - sx;
            float dist = sqrtf(dx * dx + dy * dy);

            if (dist <= half_width) {
                float weight = kaiser_bessel(dist, (float)kernel_width, beta);
                int grid_idx = gy * grid_size + gx;
                atomicAdd(&grid_real[grid_idx], sr * weight);
                atomicAdd(&grid_imag[grid_idx], si * weight);
            }
        }
    }
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int num_samples = parseIntParam(argc, argv, "--num_samples", 65536);
    int grid_size = parseIntParam(argc, argv, "--grid_size", 256);
    int kernel_width = parseIntParam(argc, argv, "--kernel_width", 4);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);

    // Kaiser-Bessel beta parameter (simplified standard choice)
    float beta = 5.0f * (float)kernel_width / 4.0f;

    size_t samples_bytes = (size_t)num_samples * sizeof(float);
    size_t grid_bytes = (size_t)grid_size * grid_size * sizeof(float);

    // Allocate host memory
    float *h_sx = (float *)malloc(samples_bytes);
    float *h_sy = (float *)malloc(samples_bytes);
    float *h_sr = (float *)malloc(samples_bytes);
    float *h_si = (float *)malloc(samples_bytes);
    float *h_grid_real = (float *)calloc(grid_size * grid_size, sizeof(float));
    float *h_grid_imag = (float *)calloc(grid_size * grid_size, sizeof(float));

    // Initialize k-space samples programmatically
    srand(42);
    for (int i = 0; i < num_samples; i++) {
        // Random positions within the grid (with margin for kernel width)
        h_sx[i] = kernel_width + (float)(rand() % 10000) / 10000.0f * (grid_size - 2 * kernel_width);
        h_sy[i] = kernel_width + (float)(rand() % 10000) / 10000.0f * (grid_size - 2 * kernel_width);
        // Random complex values
        h_sr[i] = (float)(rand() % 2000 - 1000) / 1000.0f;
        h_si[i] = (float)(rand() % 2000 - 1000) / 1000.0f;
    }

    // Device allocations
    float *d_sx, *d_sy, *d_sr, *d_si, *d_grid_real, *d_grid_imag;
    HIP_CHECK(hipMalloc(&d_sx, samples_bytes));
    HIP_CHECK(hipMalloc(&d_sy, samples_bytes));
    HIP_CHECK(hipMalloc(&d_sr, samples_bytes));
    HIP_CHECK(hipMalloc(&d_si, samples_bytes));
    HIP_CHECK(hipMalloc(&d_grid_real, grid_bytes));
    HIP_CHECK(hipMalloc(&d_grid_imag, grid_bytes));

    HIP_CHECK(hipMemcpy(d_sx, h_sx, samples_bytes, hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_sy, h_sy, samples_bytes, hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_sr, h_sr, samples_bytes, hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_si, h_si, samples_bytes, hipMemcpyHostToDevice));

    dim3 block(block_size);
    dim3 grid((num_samples + block_size - 1) / block_size);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_samples);

    printCSVHeader();

    // Benchmark gridding kernel
    BenchResult r = runBenchmark(
        "gridding_kernel", problemSize, iterations, [&]() {
            // Clear grid before each run
            HIP_CHECK(hipMemset(d_grid_real, 0, grid_bytes));
            HIP_CHECK(hipMemset(d_grid_imag, 0, grid_bytes));
            gridding_kernel<<<grid, block>>>(d_sx, d_sy, d_sr, d_si,
                                             d_grid_real, d_grid_imag,
                                             num_samples, grid_size,
                                             kernel_width, beta);
        });
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_sx));
    HIP_CHECK(hipFree(d_sy));
    HIP_CHECK(hipFree(d_sr));
    HIP_CHECK(hipFree(d_si));
    HIP_CHECK(hipFree(d_grid_real));
    HIP_CHECK(hipFree(d_grid_imag));
    free(h_sx);
    free(h_sy);
    free(h_sr);
    free(h_si);
    free(h_grid_real);
    free(h_grid_imag);

    return 0;
}
