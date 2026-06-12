// backprop.cu -- CUDA benchmark for Rodinia Back Propagation
// Two kernels: layerforward (forward pass with sigmoid) and adjust_weights
// (weight update with learning rate + momentum)

#include "bench_common_cuda.h"

typedef float DATA_TYPE;

// Forward pass kernel: compute output of one layer using sigmoid activation
__global__ void layerforward(DATA_TYPE *input, DATA_TYPE *weights,
                             DATA_TYPE *output, int num_input,
                             int num_output) {
    int idx = blockIdx.x * blockDim.x + threadIdx.x;
    if (idx < num_output) {
        DATA_TYPE sum = 0.0f;
        for (int j = 0; j < num_input; j++) {
            sum += input[j] * weights[idx * num_input + j];
        }
        // Add bias (last weight)
        sum += weights[idx * num_input + num_input];
        // Sigmoid activation
        output[idx] = 1.0f / (1.0f + expf(-sum));
    }
}

// Weight adjustment kernel: update weights using learning rate and momentum
__global__ void adjust_weights(DATA_TYPE *delta, DATA_TYPE *input,
                               DATA_TYPE *weights, DATA_TYPE *prev_weights,
                               int num_input, int num_output,
                               DATA_TYPE learning_rate, DATA_TYPE momentum) {
    int by = blockIdx.y;
    int tx = blockIdx.x * blockDim.x + threadIdx.x;

    if (by < num_output && tx <= num_input) {
        int idx = by * (num_input + 1) + tx;
        DATA_TYPE inp = (tx < num_input) ? input[tx] : 1.0f; // bias
        DATA_TYPE new_dw = learning_rate * delta[by] * inp +
                           momentum * prev_weights[idx];
        weights[idx] += new_dw;
        prev_weights[idx] = new_dw;
    }
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int num_input = parseIntParam(argc, argv, "--num_input", 65536);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);
    float learning_rate =
        parseIntParam(argc, argv, "--learning_rate_x100", 30) / 100.0f;
    float momentum_val =
        parseIntParam(argc, argv, "--momentum_x100", 50) / 100.0f;

    // Hidden layer size is 16 (fixed, as in original Rodinia)
    int num_hidden = 16;

    // Allocate host memory
    int input_weights_size = (num_input + 1) * num_hidden;
    DATA_TYPE *h_input = (DATA_TYPE *)malloc(num_input * sizeof(DATA_TYPE));
    DATA_TYPE *h_weights =
        (DATA_TYPE *)malloc(input_weights_size * sizeof(DATA_TYPE));
    DATA_TYPE *h_prev_weights =
        (DATA_TYPE *)malloc(input_weights_size * sizeof(DATA_TYPE));
    DATA_TYPE *h_hidden = (DATA_TYPE *)malloc(num_hidden * sizeof(DATA_TYPE));
    DATA_TYPE *h_delta = (DATA_TYPE *)malloc(num_hidden * sizeof(DATA_TYPE));

    // Initialize data programmatically
    srand(42);
    for (int i = 0; i < num_input; i++)
        h_input[i] = (DATA_TYPE)(rand() % 100) / 100.0f;
    for (int i = 0; i < input_weights_size; i++)
        h_weights[i] = (DATA_TYPE)(rand() % 100) / 1000.0f - 0.05f;
    for (int i = 0; i < input_weights_size; i++)
        h_prev_weights[i] = 0.0f;
    for (int i = 0; i < num_hidden; i++)
        h_delta[i] = (DATA_TYPE)(rand() % 100) / 1000.0f - 0.05f;

    // Device allocations
    DATA_TYPE *d_input, *d_weights, *d_prev_weights, *d_hidden, *d_delta;
    CUDA_CHECK(cudaMalloc(&d_input, num_input * sizeof(DATA_TYPE)));
    CUDA_CHECK(
        cudaMalloc(&d_weights, input_weights_size * sizeof(DATA_TYPE)));
    CUDA_CHECK(
        cudaMalloc(&d_prev_weights, input_weights_size * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_hidden, num_hidden * sizeof(DATA_TYPE)));
    CUDA_CHECK(cudaMalloc(&d_delta, num_hidden * sizeof(DATA_TYPE)));

    CUDA_CHECK(cudaMemcpy(d_input, h_input, num_input * sizeof(DATA_TYPE),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_weights, h_weights,
                          input_weights_size * sizeof(DATA_TYPE),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_prev_weights, h_prev_weights,
                          input_weights_size * sizeof(DATA_TYPE),
                          cudaMemcpyHostToDevice));
    CUDA_CHECK(cudaMemcpy(d_delta, h_delta, num_hidden * sizeof(DATA_TYPE),
                          cudaMemcpyHostToDevice));

    // Kernel launch configs
    dim3 block_fwd(block_size);
    dim3 grid_fwd((num_hidden + block_size - 1) / block_size);

    dim3 block_adj(block_size);
    dim3 grid_adj(((num_input + 1) + block_size - 1) / block_size, num_hidden);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", num_input);

    printCSVHeader();

    // Benchmark layerforward
    BenchResult r1 = runBenchmark(
        "layerforward", problemSize, iterations, [&]() {
            layerforward<<<grid_fwd, block_fwd>>>(d_input, d_weights, d_hidden,
                                                  num_input, num_hidden);
        });
    printCSVRow(r1);

    // Benchmark adjust_weights
    BenchResult r2 = runBenchmark(
        "adjust_weights", problemSize, iterations, [&]() {
            adjust_weights<<<grid_adj, block_adj>>>(
                d_delta, d_input, d_weights, d_prev_weights, num_input,
                num_hidden, learning_rate, momentum_val);
        });
    printCSVRow(r2);

    // Cleanup
    CUDA_CHECK(cudaFree(d_input));
    CUDA_CHECK(cudaFree(d_weights));
    CUDA_CHECK(cudaFree(d_prev_weights));
    CUDA_CHECK(cudaFree(d_hidden));
    CUDA_CHECK(cudaFree(d_delta));
    free(h_input);
    free(h_weights);
    free(h_prev_weights);
    free(h_hidden);
    free(h_delta);

    return 0;
}
