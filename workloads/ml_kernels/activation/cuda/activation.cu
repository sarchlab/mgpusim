// activation.cu -- CUDA activation function benchmark (ReLU, GELU, SiLU)
// Elementwise activation functions used in transformer FFN layers:
// - ReLU:  y = max(0, x)
// - GELU (approximate): y = 0.5 * x * (1 + tanh(sqrt(2/pi) * (x + 0.044715 * x^3)))
// - SiLU (Swish): y = x * sigmoid(x) = x / (1 + exp(-x))
// Supports elements_per_thread vectorization.
// Reports bandwidth in GB/s and throughput in Gops/s.

#include "bench_common_cuda.h"

// ---------------------------------------------------------------------------
// Activation kernel -- elementwise, with elements_per_thread vectorization
// act_type: 1 = ReLU, 2 = GELU (approximate), 3 = SiLU (Swish)
// ---------------------------------------------------------------------------
__global__ void activation_kernel(const float* __restrict__ input,
                                  float* __restrict__ output,
                                  int N, int act_type,
                                  int elements_per_thread) {
    int base = (blockIdx.x * blockDim.x + threadIdx.x) * elements_per_thread;

    for (int e = 0; e < elements_per_thread; e++) {
        int gid = base + e;
        if (gid >= N) return;

        float x = input[gid];
        float y;

        switch (act_type) {
            case 1: // ReLU
                y = fmaxf(0.0f, x);
                break;
            case 2: // GELU (approximate)
                y = 0.5f * x * (1.0f + tanhf(0.7978845608f *
                    (x + 0.044715f * x * x * x)));
                break;
            case 3: // SiLU (Swish)
                y = x / (1.0f + expf(-x));
                break;
            default:
                y = x;
        }

        output[gid] = y;
    }
}

int main(int argc, char** argv) {
    int iterations         = parseIterations(argc, argv);
    int num_elements       = parseIntParam(argc, argv, "--num_elements", 16777216);
    int block_size         = parseIntParam(argc, argv, "--block_size", 256);
    int act_type           = parseIntParam(argc, argv, "--act_type", 2);
    int elements_per_thread = parseIntParam(argc, argv, "--elements_per_thread", 1);

    size_t data_bytes = (size_t)num_elements * sizeof(float);

    // Allocate and initialize host data
    float* h_input = (float*)malloc(data_bytes);

    srand(42);
    for (int i = 0; i < num_elements; i++) {
        h_input[i] = ((float)rand() / RAND_MAX) * 10.0f - 5.0f; // range [-5, 5]
    }

    // Allocate device memory
    float *d_input, *d_output;
    CUDA_CHECK(cudaMalloc(&d_input, data_bytes));
    CUDA_CHECK(cudaMalloc(&d_output, data_bytes));

    CUDA_CHECK(cudaMemcpy(d_input, h_input, data_bytes,
                          cudaMemcpyHostToDevice));

    // Problem size string
    const char* act_name;
    switch (act_type) {
        case 1:  act_name = "relu";  break;
        case 2:  act_name = "gelu";  break;
        case 3:  act_name = "silu";  break;
        default: act_name = "identity"; break;
    }

    char problemSize[128];
    snprintf(problemSize, sizeof(problemSize), "n%d_bs%d_ept%d_%s",
             num_elements, block_size, elements_per_thread, act_name);

    printCSVHeader();

    // Adjust grid size for elements_per_thread
    int threads_needed = (num_elements + elements_per_thread - 1) / elements_per_thread;
    dim3 grid((threads_needed + block_size - 1) / block_size);
    dim3 block(block_size);

    BenchResult r = runBenchmark(act_name, problemSize, iterations, [&]() {
        activation_kernel<<<grid, block>>>(d_input, d_output,
                                           num_elements, act_type,
                                           elements_per_thread);
    });

    printCSVRow(r);

    // Bandwidth: read input + write output = 2 * data_bytes
    double bytes_processed = 2.0 * (double)data_bytes;
    double gb_per_sec = (bytes_processed / 1.0e9) / (r.avg_ms / 1000.0);
    double gops_per_sec = ((double)num_elements / 1.0e9) / (r.avg_ms / 1000.0);

    printf("# num_elements=%d, block_size=%d, elements_per_thread=%d, "
           "act_type=%d (%s)\n",
           num_elements, block_size, elements_per_thread, act_type, act_name);
    printf("# Total bytes processed: %.2e\n", bytes_processed);
    printf("# Bandwidth: %.2f GB/s\n", gb_per_sec);
    printf("# Throughput: %.2f Gops/s\n", gops_per_sec);

    // Cleanup
    CUDA_CHECK(cudaFree(d_input));
    CUDA_CHECK(cudaFree(d_output));
    free(h_input);

    return 0;
}
