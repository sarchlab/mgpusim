/*
 * HIP kernel for Rodinia Hotspot (gfx942 / CDNA3)
 * Extracted from sarchlab/gpu_benchmarks tier2/rodinia_hotspot.
 *
 * Iterative 2D stencil thermal simulation. Each cell's temperature is updated
 * from its four neighbors (N/S/E/W), the local power density, and thermal
 * resistances.
 *
 * Uses a constant BLOCK_SIZE (not blockDim) for the block geometry so the
 * compiler emits no hidden ABI arguments. One thread per grid cell.
 */
#include "hip/hip_runtime.h"

#define BLOCK_SIZE 16
#define AMB_TEMP   80.0f

extern "C" __global__ void hotspot_kernel(
    const float* __restrict__ temp_src,
    float* __restrict__ temp_dst,
    const float* __restrict__ power,
    int grid_cols, int grid_rows,
    float step_div_cap,
    float Rx_1, float Ry_1, float Rz_1)
{
    int col = blockIdx.x * BLOCK_SIZE + threadIdx.x;
    int row = blockIdx.y * BLOCK_SIZE + threadIdx.y;

    if (col < grid_cols && row < grid_rows) {
        int idx = row * grid_cols + col;

        float temp_c = temp_src[idx];
        float temp_n = (row > 0)             ? temp_src[(row - 1) * grid_cols + col] : temp_c;
        float temp_s = (row < grid_rows - 1) ? temp_src[(row + 1) * grid_cols + col] : temp_c;
        float temp_w = (col > 0)             ? temp_src[row * grid_cols + (col - 1)] : temp_c;
        float temp_e = (col < grid_cols - 1) ? temp_src[row * grid_cols + (col + 1)] : temp_c;

        float delta = step_div_cap *
            (power[idx]
             + (temp_n + temp_s - 2.0f * temp_c) * Ry_1
             + (temp_w + temp_e - 2.0f * temp_c) * Rx_1
             + (AMB_TEMP - temp_c) * Rz_1);

        temp_dst[idx] = temp_c + delta;
    }
}
