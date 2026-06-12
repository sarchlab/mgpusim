#include <hip/hip_runtime.h>

// Kernel: compute observation likelihood for each particle
__global__ void likelihood_kernel(const float *particles, const float *obs,
                                  float *weights, int num_particles,
                                  int state_dims) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (tid >= num_particles) return;

    // Gaussian likelihood: exp(-0.5 * sum((particle - obs)^2))
    float sum = 0.0f;
    for (int d = 0; d < state_dims; d++) {
        float diff = particles[tid * state_dims + d] - obs[d];
        sum += diff * diff;
    }
    weights[tid] = expf(-0.5f * sum);
}

// Kernel: normalize weights -- first compute sum via atomic, then normalize
// Step 1: compute partial sums using atomicAdd
__global__ void weight_sum_kernel(const float *weights, float *weight_sum,
                                  int num_particles) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (tid >= num_particles) return;

    atomicAdd(weight_sum, weights[tid]);
}

// Step 2: normalize each weight
__global__ void weight_normalize_kernel(float *weights, const float *weight_sum,
                                        int num_particles) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (tid >= num_particles) return;

    float total = *weight_sum;
    if (total > 0.0f) {
        weights[tid] /= total;
    } else {
        weights[tid] = 1.0f / num_particles;
    }
}

// Kernel: build CDF from normalized weights (sequential prefix sum on GPU
// for simplicity -- single-threaded)
__global__ void build_cdf_kernel(const float *weights, float *cdf,
                                 int num_particles) {
    if (hipThreadIdx_x == 0 && hipBlockIdx_x == 0) {
        cdf[0] = weights[0];
        for (int i = 1; i < num_particles; i++) {
            cdf[i] = cdf[i - 1] + weights[i];
        }
    }
}

// Kernel: systematic resampling -- each thread finds its resampled index
__global__ void resample_kernel(const float *cdf, const float *particles_in,
                                float *particles_out, int num_particles,
                                int state_dims, float u0) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (tid >= num_particles) return;

    float u = u0 + (float)tid / (float)num_particles;
    if (u >= 1.0f) u -= 1.0f;

    // Binary search for the resampled index
    int lo = 0, hi = num_particles - 1;
    while (lo < hi) {
        int mid = (lo + hi) / 2;
        if (cdf[mid] < u) {
            lo = mid + 1;
        } else {
            hi = mid;
        }
    }

    for (int d = 0; d < state_dims; d++) {
        particles_out[tid * state_dims + d] =
            particles_in[lo * state_dims + d];
    }
}

// Kernel: add random noise to particles (state transition)
__global__ void transition_kernel(float *particles, int num_particles,
                                  int state_dims, unsigned int seed) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (tid >= num_particles) return;

    // Simple LCG-based noise per particle
    unsigned int rng = seed + tid * 1099087573u;
    for (int d = 0; d < state_dims; d++) {
        rng = rng * 1664525u + 1013904223u;
        float noise = ((float)(rng & 0xFFFFFF) / (float)0xFFFFFF - 0.5f) * 2.0f;
        particles[tid * state_dims + d] += noise;
    }
}
