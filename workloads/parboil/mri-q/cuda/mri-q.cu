// mri-q.cu -- CUDA benchmark for Parboil MRI-Q matrix computation
// Kernel: For each voxel, iterate over all k-space samples computing
// Q[v] += phi * exp(2*pi*i*(kx*x + ky*y + kz*z))
// Real and imaginary parts stored separately using sinf/cosf.

#include "bench_common_cuda.h"

#ifndef M_PI
#define M_PI 3.14159265358979323846f
#endif

// Kernel: compute Q matrix (real and imaginary parts) for each voxel
__global__ void ComputeQ(float *x, float *y, float *z,
                         float *kx, float *ky, float *kz,
                         float *phi_r, float *phi_i,
                         float *Qr, float *Qi,
                         int num_voxels, int num_k) {
    int v = blockIdx.x * blockDim.x + threadIdx.x;
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

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int num_voxels = parseIntParam(argc, argv, "--num_voxels", 65536);
    int num_k_samples = parseIntParam(argc, argv, "--num_k_samples", 256);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);
    // precision_mode parsed but not changing kernel logic (reserved for future)
    // int precision_mode = parseIntParam(argc, argv, "--precision_mode", 2);

    // Allocate host memory
    float *h_x = (float *)malloc(num_voxels * sizeof(float));
    float *h_y = (float *)malloc(num_voxels * sizeof(float));
    float *h_z = (float *)malloc(num_voxels * sizeof(float));
    float *h_kx = (float *)malloc(num_k_samples * sizeof(float));
    float *h_ky = (float *)malloc(num_k_samples * sizeof(float));
    float *h_kz = (float *)malloc(num_k_samples * sizeof(float));
    float *h_phi_r = (float *)malloc(num_k_samples * sizeof(float));
    float *h_phi_i = (float *)malloc(num_k_samples * sizeof(float));
    float *h_Qr = (float *)malloc(num_voxels * sizeof(float));
    float *h_Qi = (float *)malloc(num_voxels * sizeof(float));

    // Initialize data programmatically
    srand(42);
    for (int i = 0; i < num_voxels; i++) {
        h_x[i] = (float)(rand() % 1000) / 1000.0f - 0.5f;
        h_y[i] = (float)(rand() % 1000) / 1000.0f - 0.5f;
        h_z[i] = (float)(rand() % 1000) / 1000.0f - 0.5f;
        h_Qr[i] = 0.0f;
        h_Qi[i] = 0.0f;
    }
    for (int i = 0; i < num_k_samples; i++) {
        h_kx[i] = (float)(rand() % 1000) / 500.0f - 1.0f;
        h_ky[i] = (float)(rand() % 1000) / 500.0f - 1.0f;
        h_kz[i] = (float)(rand() % 1000) / 500.0f - 1.0f;
        h_phi_r[i] = (float)(rand() % 1000) / 1000.0f;
        h_phi_i[i] = (float)(rand() % 1000) / 1000.0f;
    }

    // Device allocations
    float *d_x, *d_y, *d_z;
    float *d_kx, *d_ky, *d_kz;
    float *d_phi_r, *d_phi_i;
    float *d_Qr, *d_Qi;

    CUDA_CHECK(cudaMalloc(&d_x, num_voxels * sizeof(float)));
    CUDA_CHECK(cudaMalloc(&d_y, num_voxels * sizeof(float)));
    CUDA_CHECK(cudaMalloc(&d_z, num_voxels * sizeof(float)));
    CUDA_CHECK(cudaMalloc(&d_kx, num_k_samples * sizeof(float)));
    CUDA_CHECK(cudaMalloc(&d_ky, num_k_samples * sizeof(float)));
    CUDA_CHECK(cudaMalloc(&d_kz, num_k_samples * sizeof(float)));
    CUDA_CHECK(cudaMalloc(&d_phi_r, num_k_samples * sizeof(float)));
    CUDA_CHECK(cudaMalloc(&d_phi_i, num_k_samples * sizeof(float)));
    CUDA_CHECK(cudaMalloc(&d_Qr, num_voxels * sizeof(float)));
    CUDA_CHECK(cudaMalloc(&d_Qi, num_voxels * sizeof(float)));

    CUDA_CHECK(cudaMemcpy(d_x, h_x, num_voxels * sizeof(float),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_y, h_y, num_voxels * sizeof(float),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_z, h_z, num_voxels * sizeof(float),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_kx, h_kx, num_k_samples * sizeof(float),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_ky, h_ky, num_k_samples * sizeof(float),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_kz, h_kz, num_k_samples * sizeof(float),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_phi_r, h_phi_r, num_k_samples * sizeof(float),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_phi_i, h_phi_i, num_k_samples * sizeof(float),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_Qr, h_Qr, num_voxels * sizeof(float),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_Qi, h_Qi, num_voxels * sizeof(float),
                          cudaMemcpyHostToDevice));

    // Kernel launch config
    dim3 block(block_size);
    dim3 grid((num_voxels + block_size - 1) / block_size);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_voxels);

    printCSVHeader();

    // Benchmark ComputeQ
    BenchResult r1 = runBenchmark(
        "ComputeQ", problemSize, iterations, [&]() {
            // Reset Q to zero before each run
            CUDA_CHECK(cudaMemset(d_Qr, 0, num_voxels * sizeof(float)));
            CUDA_CHECK(cudaMemset(d_Qi, 0, num_voxels * sizeof(float)));
            ComputeQ<<<grid, block>>>(d_x, d_y, d_z,
                                      d_kx, d_ky, d_kz,
                                      d_phi_r, d_phi_i,
                                      d_Qr, d_Qi,
                                      num_voxels, num_k_samples);
        });
    printCSVRow(r1);

    // Cleanup
    CUDA_CHECK(cudaFree(d_x));
    CUDA_CHECK(cudaFree(d_y));
    CUDA_CHECK(cudaFree(d_z));
    CUDA_CHECK(cudaFree(d_kx));
    CUDA_CHECK(cudaFree(d_ky));
    CUDA_CHECK(cudaFree(d_kz));
    CUDA_CHECK(cudaFree(d_phi_r));
    CUDA_CHECK(cudaFree(d_phi_i));
    CUDA_CHECK(cudaFree(d_Qr));
    CUDA_CHECK(cudaFree(d_Qi));
    free(h_x);
    free(h_y);
    free(h_z);
    free(h_kx);
    free(h_ky);
    free(h_kz);
    free(h_phi_r);
    free(h_phi_i);
    free(h_Qr);
    free(h_Qi);

    return 0;
}
