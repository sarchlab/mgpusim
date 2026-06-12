#include <hip/hip_runtime.h>

typedef float DATA_TYPE;

// Forward pass kernel: compute output of one layer using sigmoid activation
__global__ void layerforward(DATA_TYPE *input, DATA_TYPE *weights,
                             DATA_TYPE *output, int num_input,
                             int num_output) {
    int idx = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
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
    int by = hipBlockIdx_y;
    int tx = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;

    if (by < num_output && tx <= num_input) {
        int idx = by * (num_input + 1) + tx;
        DATA_TYPE inp = (tx < num_input) ? input[tx] : 1.0f; // bias
        DATA_TYPE new_dw = learning_rate * delta[by] * inp +
                           momentum * prev_weights[idx];
        weights[idx] += new_dw;
        prev_weights[idx] = new_dw;
    }
}
