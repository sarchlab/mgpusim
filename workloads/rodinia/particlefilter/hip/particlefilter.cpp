// particlefilter.cpp -- HIP benchmark for Rodinia Particle Filter
// Sequential Monte Carlo estimator with likelihood, weight normalization,
// and systematic resampling kernels.

#include "bench_common_hip.h"

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

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int num_particles =
        parseIntParam(argc, argv, "--num_particles", 65536);
    int num_timesteps = parseIntParam(argc, argv, "--num_timesteps", 10);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);
    int state_dims = parseIntParam(argc, argv, "--state_dimensions", 2);

    // Generate initial particle states and observation sequence
    float *h_particles =
        (float *)malloc(num_particles * state_dims * sizeof(float));
    float *h_observations =
        (float *)malloc(num_timesteps * state_dims * sizeof(float));

    srand(42);
    for (int i = 0; i < num_particles * state_dims; i++) {
        h_particles[i] = (float)(rand() % 1000) / 100.0f;
    }
    for (int i = 0; i < num_timesteps * state_dims; i++) {
        h_observations[i] = (float)(rand() % 1000) / 100.0f;
    }

    // Device allocations
    float *d_particles, *d_particles_tmp, *d_weights, *d_cdf;
    float *d_obs, *d_weight_sum;
    HIP_CHECK(hipMalloc(&d_particles,
                        num_particles * state_dims * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_particles_tmp,
                        num_particles * state_dims * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_weights, num_particles * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_cdf, num_particles * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_obs, state_dims * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_weight_sum, sizeof(float)));

    dim3 block(block_size);
    dim3 grid((num_particles + block_size - 1) / block_size);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_particles);

    printCSVHeader();

    BenchResult r = runBenchmark(
        "particlefilter", problemSize, iterations, [&]() {
            // Reset particles for each benchmark iteration
            HIP_CHECK(hipMemcpy(d_particles, h_particles,
                                num_particles * state_dims * sizeof(float),
                                hipMemcpyHostToDevice));

            for (int t = 0; t < num_timesteps; t++) {
                // Upload current observation
                HIP_CHECK(hipMemcpy(d_obs, &h_observations[t * state_dims],
                                    state_dims * sizeof(float),
                                    hipMemcpyHostToDevice));

                // 1. Compute likelihoods
                likelihood_kernel<<<grid, block>>>(
                    d_particles, d_obs, d_weights, num_particles, state_dims);

                // 2. Normalize weights
                float zero = 0.0f;
                HIP_CHECK(hipMemcpy(d_weight_sum, &zero, sizeof(float),
                                    hipMemcpyHostToDevice));
                weight_sum_kernel<<<grid, block>>>(d_weights, d_weight_sum,
                                                   num_particles);
                HIP_CHECK(hipDeviceSynchronize());

                weight_normalize_kernel<<<grid, block>>>(
                    d_weights, d_weight_sum, num_particles);

                // 3. Build CDF (single-threaded kernel)
                build_cdf_kernel<<<1, 1>>>(d_weights, d_cdf, num_particles);
                HIP_CHECK(hipDeviceSynchronize());

                // 4. Systematic resampling
                float u0 = (float)(rand() % 10000) / 10000.0f / num_particles;
                resample_kernel<<<grid, block>>>(
                    d_cdf, d_particles, d_particles_tmp, num_particles,
                    state_dims, u0);
                HIP_CHECK(hipDeviceSynchronize());

                // 5. State transition (add noise)
                transition_kernel<<<grid, block>>>(
                    d_particles_tmp, num_particles, state_dims, (unsigned int)t);

                // Swap particle buffers
                float *tmp = d_particles;
                d_particles = d_particles_tmp;
                d_particles_tmp = tmp;
            }
            HIP_CHECK(hipDeviceSynchronize());
        });
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_particles));
    HIP_CHECK(hipFree(d_particles_tmp));
    HIP_CHECK(hipFree(d_weights));
    HIP_CHECK(hipFree(d_cdf));
    HIP_CHECK(hipFree(d_obs));
    HIP_CHECK(hipFree(d_weight_sum));
    free(h_particles);
    free(h_observations);

    return 0;
}
