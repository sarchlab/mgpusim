#include <hip/hip_runtime.h>

#ifndef M_PI
#define M_PI 3.14159265358979323846
#endif

typedef float DATA_TYPE;

// Kernel: compute angular separations and bin into histogram
// Each thread handles one point i and compares against all points j > i
__global__ void tpacf_kernel(DATA_TYPE *ra, DATA_TYPE *dec,
                             unsigned long long *hist,
                             int num_points, int num_bins,
                             DATA_TYPE min_angle, DATA_TYPE bin_width) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    if (i < num_points) {
        DATA_TYPE ra_i = ra[i];
        DATA_TYPE dec_i = dec[i];
        DATA_TYPE sin_dec_i = sinf(dec_i);
        DATA_TYPE cos_dec_i = cosf(dec_i);

        for (int j = i + 1; j < num_points; j++) {
            DATA_TYPE ra_j = ra[j];
            DATA_TYPE dec_j = dec[j];

            // Angular separation using spherical law of cosines
            DATA_TYPE cos_angle = sin_dec_i * sinf(dec_j) +
                                  cos_dec_i * cosf(dec_j) * cosf(ra_i - ra_j);

            // Clamp to [-1, 1] to avoid NaN from acos
            if (cos_angle > 1.0f) cos_angle = 1.0f;
            if (cos_angle < -1.0f) cos_angle = -1.0f;

            DATA_TYPE angle = acosf(cos_angle);

            // Bin the angle
            int bin = (int)((angle - min_angle) / bin_width);
            if (bin >= 0 && bin < num_bins) {
                atomicAdd(&hist[bin], 1ULL);
            }
        }
    }
}
