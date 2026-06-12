#include <hip/hip_runtime.h>

typedef float DATA_TYPE;

#define AMB_TEMP 80.0f
#define K_SI 100.0f
#define C_SI 1.75e6f
#define T_CHIP 0.0005f

__global__ void hotspot3D_kernel(DATA_TYPE *temp_src, DATA_TYPE *temp_dst,
                                 DATA_TYPE *power, int nx, int ny, int nz,
                                 DATA_TYPE ce, DATA_TYPE cw,
                                 DATA_TYPE cn, DATA_TYPE cs,
                                 DATA_TYPE ct, DATA_TYPE cb,
                                 DATA_TYPE cc, DATA_TYPE step_div_cap) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;  // x
    int j = hipBlockIdx_y * hipBlockDim_y + hipThreadIdx_y;  // y

    if (i < nx && j < ny) {
        for (int k = 0; k < nz; k++) {
            int idx = k * nx * ny + j * nx + i;

            DATA_TYPE temp_c = temp_src[idx];

            // Neighbors with boundary clamping
            DATA_TYPE temp_w = (i > 0)      ? temp_src[idx - 1]        : temp_c;
            DATA_TYPE temp_e = (i < nx - 1) ? temp_src[idx + 1]        : temp_c;
            DATA_TYPE temp_n = (j > 0)      ? temp_src[idx - nx]       : temp_c;
            DATA_TYPE temp_s = (j < ny - 1) ? temp_src[idx + nx]       : temp_c;
            DATA_TYPE temp_b = (k > 0)      ? temp_src[idx - nx * ny]  : temp_c;
            DATA_TYPE temp_t = (k < nz - 1) ? temp_src[idx + nx * ny]  : temp_c;

            temp_dst[idx] = temp_c * cc +
                            temp_w * cw + temp_e * ce +
                            temp_n * cn + temp_s * cs +
                            temp_b * cb + temp_t * ct +
                            step_div_cap * power[idx] +
                            ct * (AMB_TEMP - temp_c);
        }
    }
}
