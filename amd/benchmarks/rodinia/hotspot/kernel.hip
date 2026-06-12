#include <hip/hip_runtime.h>

typedef float DATA_TYPE;

#define AMB_TEMP 80.0f

// Chip parameters (simplified)
#define CHIP_HEIGHT 0.016f
#define CHIP_WIDTH 0.016f
#define T_CHIP 0.0005f
#define K_SI 100.0f
#define C_SI 1.75e6f
#define FACTOR_CHIP (T_CHIP / K_SI)

__global__ void hotspot_kernel(DATA_TYPE *temp_src, DATA_TYPE *temp_dst,
                               DATA_TYPE *power, int grid_cols, int grid_rows,
                               DATA_TYPE step_div_cap, DATA_TYPE Rx_1,
                               DATA_TYPE Ry_1, DATA_TYPE Rz_1) {
    int col = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int row = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;

    if (col < grid_cols && row < grid_rows) {
        int idx = row * grid_cols + col;

        // Get neighbor temperatures (with boundary clamping)
        DATA_TYPE temp_c = temp_src[idx];
        DATA_TYPE temp_n =
            (row > 0) ? temp_src[(row - 1) * grid_cols + col] : temp_c;
        DATA_TYPE temp_s =
            (row < grid_rows - 1) ? temp_src[(row + 1) * grid_cols + col]
                                  : temp_c;
        DATA_TYPE temp_w =
            (col > 0) ? temp_src[row * grid_cols + (col - 1)] : temp_c;
        DATA_TYPE temp_e =
            (col < grid_cols - 1) ? temp_src[row * grid_cols + (col + 1)]
                                  : temp_c;

        // Compute the new temperature
        DATA_TYPE delta =
            step_div_cap *
            (power[idx] + (temp_n + temp_s - 2.0f * temp_c) * Ry_1 +
             (temp_w + temp_e - 2.0f * temp_c) * Rx_1 +
             (AMB_TEMP - temp_c) * Rz_1);

        temp_dst[idx] = temp_c + delta;
    }
}
