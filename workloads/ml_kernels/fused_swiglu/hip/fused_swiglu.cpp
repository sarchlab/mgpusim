// fused_swiglu.cpp -- HIP fused SwiGLU activation benchmark
// SwiGLU is the activation function used in Llama-2's FFN layers.
// It fuses the SiLU (Swish) gating with a linear up-projection:
//   SwiGLU(gate, up) = SiLU(gate) * up
//                    = (gate / (1 + exp(-gate))) * up
//
// This is a fused elementwise operation: read 2 inputs (gate, up),
// compute SiLU on gate, multiply with up, write 1 output.
// Supports elements_per_thread vectorization for ILP.
// Reports bandwidth in GB/s and throughput in Gops/s.

#include "bench_common_hip.h"

// ---------------------------------------------------------------------------
// Fused SwiGLU kernel -- elementwise with elements_per_thread vectorization
// precision: 1 = FP32, 2 = FP32 with fast-math (compiler handles via flags),
//            3 = FP32 with output clamping (clamp to [-65504, 65504])
// ---------------------------------------------------------------------------
__global__ void fused_swiglu_kernel(const float* __restrict__ gate,
                                    const float* __restrict__ up,
                                    float* __restrict__ output,
                                    int N,
                                    int elements_per_thread,
                                    int precision) {
    int base = (hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x) * elements_per_thread;

    for (int e = 0; e < elements_per_thread; e++) {
        int gid = base + e;
        if (gid >= N) return;

        float g = gate[gid];
        float u = up[gid];

        // SiLU(gate) * up
        float silu_g = g / (1.0f + expf(-g));
        float result = silu_g * u;

        if (precision == 3) {
            // Clamp output to FP16-safe range
            result = fminf(fmaxf(result, -65504.0f), 65504.0f);
        }

        output[gid] = result;
    }
}

int main(int argc, char** argv) {
    int iterations          = parseIterations(argc, argv);
    int num_elements        = parseIntParam(argc, argv, "--num_elements", 16777216);
    int block_size          = parseIntParam(argc, argv, "--block_size", 256);
    int elements_per_thread = parseIntParam(argc, argv, "--elements_per_thread", 1);
    int precision           = parseIntParam(argc, argv, "--precision", 1);

    size_t data_bytes = (size_t)num_elements * sizeof(float);

    // Allocate and initialize host data
    float* h_gate = (float*)malloc(data_bytes);
    float* h_up   = (float*)malloc(data_bytes);

    srand(42);
    for (int i = 0; i < num_elements; i++) {
        h_gate[i] = ((float)rand() / (float)RAND_MAX) * 6.0f - 3.0f; // range [-3, 3]
        h_up[i]   = ((float)rand() / (float)RAND_MAX) * 6.0f - 3.0f; // range [-3, 3]
    }

    // Allocate device memory
    float *d_gate, *d_up, *d_output;
    HIP_CHECK(hipMalloc(&d_gate, data_bytes));
    HIP_CHECK(hipMalloc(&d_up, data_bytes));
    HIP_CHECK(hipMalloc(&d_output, data_bytes));

    HIP_CHECK(hipMemcpy(d_gate, h_gate, data_bytes, hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_up, h_up, data_bytes, hipMemcpyHostToDevice));

    // Problem size string
    char problemSize[128];
    snprintf(problemSize, sizeof(problemSize), "n%d_bs%d_ept%d_p%d",
             num_elements, block_size, elements_per_thread, precision);

    printCSVHeader();

    // Adjust grid size for elements_per_thread
    int threads_needed = (num_elements + elements_per_thread - 1) / elements_per_thread;
    dim3 grid((threads_needed + block_size - 1) / block_size);
    dim3 block(block_size);

    BenchResult r = runBenchmark("fused_swiglu", problemSize, iterations, [&]() {
        hipLaunchKernelGGL(fused_swiglu_kernel, grid, block, 0, 0,
                           d_gate, d_up, d_output, num_elements,
                           elements_per_thread, precision);
    });

    printCSVRow(r);

    // Bandwidth: 2 reads (gate, up) + 1 write (output) = 3 * data_bytes
    double bytes_processed = 3.0 * (double)data_bytes;
    double gb_per_sec = (bytes_processed / 1.0e9) / (r.avg_ms / 1000.0);
    double gops_per_sec = ((double)num_elements / 1.0e9) / (r.avg_ms / 1000.0);

    printf("# num_elements=%d, block_size=%d, elements_per_thread=%d, "
           "precision=%d\n",
           num_elements, block_size, elements_per_thread, precision);
    printf("# Total bytes processed: %.2e (2 reads + 1 write)\n",
           bytes_processed);
    printf("# Bandwidth: %.2f GB/s\n", gb_per_sec);
    printf("# Throughput: %.2f Gops/s\n", gops_per_sec);

    // Cleanup
    HIP_CHECK(hipFree(d_gate));
    HIP_CHECK(hipFree(d_up));
    HIP_CHECK(hipFree(d_output));
    free(h_gate);
    free(h_up);

    return 0;
}
