#include <hip/hip_runtime.h>

#ifndef M_PI
#define M_PI 3.14159265358979323846f
#endif

// Kaiser-Bessel window function approximation
// Used as a convolution kernel for MRI gridding.
__device__ float kaiser_bessel(float dist, float width, float beta) {
    float x = dist / (width * 0.5f);
    if (fabsf(x) >= 1.0f) return 0.0f;
    float arg = beta * sqrtf(1.0f - x * x);
    // Approximate I0(arg) using a polynomial for the modified Bessel function:
    // I0(x) ≈ 1 + (x/2)^2 + (x/2)^4/4 + (x/2)^6/36 + ...
    float half_arg = arg * 0.5f;
    float ha2 = half_arg * half_arg;
    float i0 = 1.0f + ha2 * (1.0f + ha2 * (0.25f + ha2 * (1.0f / 36.0f +
                ha2 * (1.0f / 576.0f + ha2 * (1.0f / 14400.0f)))));
    return i0;
}

// Gridding kernel: scatter each k-space sample's contribution to nearby grid
// points using a Kaiser-Bessel window. Each thread processes one sample and
// uses atomicAdd to accumulate contributions to the Cartesian output grid.
extern "C" __global__ void gridding_kernel(float *samples_x, float *samples_y,
                                            float *samples_real,
                                            float *samples_imag,
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
                float weight =
                    kaiser_bessel(dist, (float)kernel_width, beta);
                int grid_idx = gy * grid_size + gx;
                atomicAdd(&grid_real[grid_idx], sr * weight);
                atomicAdd(&grid_imag[grid_idx], si * weight);
            }
        }
    }
}
