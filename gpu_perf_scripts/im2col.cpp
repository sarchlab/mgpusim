// im2col.cpp — HIP benchmark for im2col transformation
// Kernel copied from amd/benchmarks/dnn/gputensor/native/im2col_hip.cpp
// Problem size: N=1, C=1, H=28, W=28, kernel=3x3

#include "bench_common.h"

extern "C" __global__ void im2col(float *input, float *output,
                     const uint2 inputDimensions, const uint2 maskDimensions,
                     const uint2 stride, const uint2 pad, const uint2 dilation,
                     const unsigned int channel, const unsigned int batch) {
    unsigned int tid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

    int width = inputDimensions.x;
    int height = inputDimensions.y;

    int maskWidth = maskDimensions.x;
    int maskHeight = maskDimensions.y;

    int effKernelHeight = (maskDimensions.y - 1) * dilation.y + 1;
    int effKernelWidth = (maskDimensions.x - 1) * dilation.x + 1;

    int fieldHeight = (height - effKernelHeight + 2 * pad.y) / stride.y + 1;
    int fieldWidth = (width - effKernelWidth + 2 * pad.x) / stride.x + 1;

    int outWidth = fieldHeight * fieldWidth * batch;
    int outHeight = maskHeight * maskWidth * channel;

    int frame_size = width * height;
    int picture_size = channel * frame_size;
    int mask_size = maskWidth * maskHeight;

    int batch_id = tid / (fieldWidth * fieldHeight);
    int block_id = tid % (fieldWidth * fieldHeight);
    int block_x = block_id % fieldWidth;
    int block_y = block_id / fieldWidth;

    if (batch_id >= (int)batch)
        return;

    for (int i = 0; i < outHeight; i++) {
        int channel_id = i / mask_size;
        int y = i % mask_size / maskWidth;
        int x = i % maskWidth;

        int real_x = block_x * stride.x - pad.x + dilation.x * x;
        int real_y = block_y * stride.y - pad.y + dilation.y * y;
        int input_index = batch_id * picture_size + channel_id * frame_size +
                          real_y * width + real_x;
        int output_index = i * outWidth + tid;

        float out = 0;
        if (real_x >= 0 && real_y >= 0 && real_x < width && real_y < height) {
            out = input[input_index];
        }
        output[output_index] = out;
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);

    // Input dimensions
    const unsigned int BATCH = 1;
    const unsigned int CHANNEL = 1;
    unsigned int HEIGHT = (unsigned int)parseIntParam(argc, argv, "--size", 28);
    unsigned int WIDTH  = HEIGHT;
    const unsigned int MASK_H = 3;
    const unsigned int MASK_W = 3;
    const unsigned int STRIDE_X = 1;
    const unsigned int STRIDE_Y = 1;
    const unsigned int PAD_X = 0;
    const unsigned int PAD_Y = 0;
    const unsigned int DIL_X = 1;
    const unsigned int DIL_Y = 1;

    // Compute output field dimensions
    int effKernelHeight = (MASK_H - 1) * DIL_Y + 1;
    int effKernelWidth = (MASK_W - 1) * DIL_X + 1;
    int fieldHeight = (HEIGHT - effKernelHeight + 2 * PAD_Y) / STRIDE_Y + 1;
    int fieldWidth = (WIDTH - effKernelWidth + 2 * PAD_X) / STRIDE_X + 1;
    int outWidth = fieldHeight * fieldWidth * BATCH;   // number of output columns
    int outHeight = MASK_H * MASK_W * CHANNEL;          // number of output rows

    int inputSize = BATCH * CHANNEL * HEIGHT * WIDTH;
    int outputSize = outHeight * outWidth;

    // Host allocations
    std::vector<float> h_input(inputSize);
    srand(42);
    for (int i = 0; i < inputSize; i++) {
        h_input[i] = (float)(rand() % 1000) / 500.0f - 1.0f;
    }

    // Device allocations
    float *d_input, *d_output;
    HIP_CHECK(hipMalloc(&d_input, inputSize * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_output, outputSize * sizeof(float)));
    HIP_CHECK(hipMemcpy(d_input, h_input.data(), inputSize * sizeof(float), hipMemcpyHostToDevice));

    // Each thread handles one spatial output position
    int numThreads = fieldHeight * fieldWidth * BATCH;
    const int THREADS = 256;
    dim3 block(THREADS);
    dim3 grid((numThreads + THREADS - 1) / THREADS);

    uint2 inputDims = make_uint2(WIDTH, HEIGHT);
    uint2 maskDims = make_uint2(MASK_W, MASK_H);
    uint2 strideDims = make_uint2(STRIDE_X, STRIDE_Y);
    uint2 padDims = make_uint2(PAD_X, PAD_Y);
    uint2 dilDims = make_uint2(DIL_X, DIL_Y);

    char problemSize[128];
    snprintf(problemSize, sizeof(problemSize), "N%d_C%d_%dx%d_k%dx%d",
             BATCH, CHANNEL, HEIGHT, WIDTH, MASK_H, MASK_W);

    BenchResult r = runBenchmark("im2col", problemSize, iterations, [&]() {
        im2col<<<grid, block>>>(d_input, d_output,
            inputDims, maskDims, strideDims, padDims, dilDims,
            CHANNEL, BATCH);
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_input));
    HIP_CHECK(hipFree(d_output));

    return 0;
}
