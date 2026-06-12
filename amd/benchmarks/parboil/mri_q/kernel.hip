#include <hip/hip_runtime.h>

#ifndef M_PI
#define M_PI 3.14159265358979323846f
#endif

// Kernel: compute Q matrix (real and imaginary parts) for each voxel.
// For each voxel v, sums contributions from all k-space samples:
//   Q[v] += phi[k] * exp(2*pi*i*(kx*x + ky*y + kz*z))
// Real and imaginary parts are stored separately.
extern "C" __global__ void ComputeQ(float *x, float *y, float *z,
                                     float *kx, float *ky, float *kz,
                                     float *phi_r, float *phi_i,
                                     float *Qr, float *Qi,
                                     int num_voxels, int num_k) {
    int v = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (v >= num_voxels) return;

    float xv = x[v];
    float yv = y[v];
    float zv = z[v];

    float qr_acc = 0.0f;
    float qi_acc = 0.0f;

    for (int k = 0; k < num_k; k++) {
        float phase = 2.0f * M_PI *
                      (kx[k] * xv + ky[k] * yv + kz[k] * zv);
        float cos_val = cosf(phase);
        float sin_val = sinf(phase);
        // Q[v] += phi[k] * exp(i * phase)
        // Real part: phi_r * cos - phi_i * sin
        // Imag part: phi_r * sin + phi_i * cos
        qr_acc += phi_r[k] * cos_val - phi_i[k] * sin_val;
        qi_acc += phi_r[k] * sin_val + phi_i[k] * cos_val;
    }

    Qr[v] += qr_acc;
    Qi[v] += qi_acc;
}
