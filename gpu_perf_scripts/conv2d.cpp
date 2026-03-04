// conv2d.cpp — HIP benchmark for 2D convolution via im2col + GEMM
// Kernels copied from amd/benchmarks/dnn/gputensor/native/im2col_hip.cpp and gemm_hip.cpp
// Problem size: N=1, C=1, H=28, W=28, kernel=3x3, outputC=3

#include "bench_common.h"

// ---- im2col kernel (from im2col_hip.cpp) ----
extern "C" __global__ void im2col_kernel(float *input, float *output,
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

// ---- GEMM kernel (from gemm_hip.cpp) ----
#define TILE_SIZE 16

extern "C" __global__ void gemm(int m, int n, int k, float alpha, float beta,
                   const float *a, const float *b,
                   const float *c, float *d) {
    __shared__ float subTileM[TILE_SIZE][TILE_SIZE];
    __shared__ float subTileN[TILE_SIZE][TILE_SIZE];

    int globalX = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
    int globalY = hipBlockDim_y * hipBlockIdx_y + hipThreadIdx_y;
    int bx = globalX / hipBlockDim_x;
    int by = globalY / hipBlockDim_y;
    int tx = hipThreadIdx_x;
    int ty = hipThreadIdx_y;

    int Row = by * TILE_SIZE + ty;
    int Col = bx * TILE_SIZE + tx;

    d[Row * n + Col] = 0;
    float Pvalue = 0;
    for (int i = 0; i < ((k - 1) / TILE_SIZE + 1); i++) {
        int curL = Row * k + i * TILE_SIZE + tx;
        int curR = (i * TILE_SIZE + ty) * n + Col;

        if (i * TILE_SIZE + tx < k && Row < m) {
            subTileM[ty][tx] = a[curL];
        } else {
            subTileM[ty][tx] = 0.0f;
        }

        if (i * TILE_SIZE + ty < k && Col < n) {
            subTileN[ty][tx] = b[curR];
        } else {
            subTileN[ty][tx] = 0.0f;
        }

        __syncthreads();
        for (int j = 0; j < TILE_SIZE; j++) {
            if (j + TILE_SIZE * i < k) {
                Pvalue += subTileM[ty][j] * subTileN[j][tx];
            }
        }
        __syncthreads();
    }

    if (Row < m && Col < n) {
        d[Row * n + Col] = alpha * Pvalue + beta * c[Row * n + Col];
    }
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);

    // Conv2D parameters
    const unsigned int BATCH = 1;
    const unsigned int IN_CHANNELS = 1;
    const unsigned int HEIGHT = 28;
    const unsigned int WIDTH = 28;
    const unsigned int MASK_H = 3;
    const unsigned int MASK_W = 3;
    const unsigned int OUT_CHANNELS = 3;
    const unsigned int STRIDE_X = 1;
    const unsigned int STRIDE_Y = 1;
    const unsigned int PAD_X = 0;
    const unsigned int PAD_Y = 0;
    const unsigned int DIL_X = 1;
    const unsigned int DIL_Y = 1;

    // Compute im2col output dimensions
    int effKernelHeight = (MASK_H - 1) * DIL_Y + 1;
    int effKernelWidth = (MASK_W - 1) * DIL_X + 1;
    int fieldHeight = (HEIGHT - effKernelHeight + 2 * PAD_Y) / STRIDE_Y + 1;
    int fieldWidth = (WIDTH - effKernelWidth + 2 * PAD_X) / STRIDE_X + 1;

    // im2col output: [outHeight x outWidth]
    // outHeight = MASK_H * MASK_W * IN_CHANNELS = 9
    // outWidth = fieldHeight * fieldWidth * BATCH = 26 * 26 * 1 = 676
    int im2colHeight = MASK_H * MASK_W * IN_CHANNELS;  // k for GEMM
    int im2colWidth = fieldHeight * fieldWidth * BATCH; // n for GEMM

    // GEMM: weight[OUT_CHANNELS x im2colHeight] * im2col_output[im2colHeight x im2colWidth]
    //     = result[OUT_CHANNELS x im2colWidth]
    int gemmM = OUT_CHANNELS;         // 3
    int gemmN = im2colWidth;          // 676
    int gemmK = im2colHeight;         // 9

    int inputSize = BATCH * IN_CHANNELS * HEIGHT * WIDTH;
    int im2colSize = im2colHeight * im2colWidth;
    int weightSize = OUT_CHANNELS * im2colHeight;
    int outputSize = gemmM * gemmN;

    // Host allocations
    std::vector<float> h_input(inputSize);
    std::vector<float> h_weight(weightSize);
    std::vector<float> h_bias(outputSize, 0.0f); // bias (c matrix in GEMM), zero

    srand(42);
    for (int i = 0; i < inputSize; i++)
        h_input[i] = (float)(rand() % 1000) / 500.0f - 1.0f;
    for (int i = 0; i < weightSize; i++)
        h_weight[i] = (float)(rand() % 1000) / 500.0f - 1.0f;

    // Device allocations
    float *d_input, *d_im2col, *d_weight, *d_bias, *d_output;
    HIP_CHECK(hipMalloc(&d_input, inputSize * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_im2col, im2colSize * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_weight, weightSize * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_bias, outputSize * sizeof(float)));
    HIP_CHECK(hipMalloc(&d_output, outputSize * sizeof(float)));

    HIP_CHECK(hipMemcpy(d_input, h_input.data(), inputSize * sizeof(float), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_weight, h_weight.data(), weightSize * sizeof(float), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_bias, h_bias.data(), outputSize * sizeof(float), hipMemcpyHostToDevice));

    // im2col launch config
    int numIm2colThreads = fieldHeight * fieldWidth * BATCH;
    const int IM2COL_THREADS = 256;
    dim3 im2colBlock(IM2COL_THREADS);
    dim3 im2colGrid((numIm2colThreads + IM2COL_THREADS - 1) / IM2COL_THREADS);

    uint2 inputDims = make_uint2(WIDTH, HEIGHT);
    uint2 maskDims = make_uint2(MASK_W, MASK_H);
    uint2 strideDims = make_uint2(STRIDE_X, STRIDE_Y);
    uint2 padDims = make_uint2(PAD_X, PAD_Y);
    uint2 dilDims = make_uint2(DIL_X, DIL_Y);

    // GEMM launch config
    dim3 gemmBlock(TILE_SIZE, TILE_SIZE);
    dim3 gemmGrid((gemmN + TILE_SIZE - 1) / TILE_SIZE,
                  (gemmM + TILE_SIZE - 1) / TILE_SIZE);

    char problemSize[128];
    snprintf(problemSize, sizeof(problemSize), "N%d_C%d_%dx%d_k%dx%d_OC%d",
             BATCH, IN_CHANNELS, HEIGHT, WIDTH, MASK_H, MASK_W, OUT_CHANNELS);

    BenchResult r = runBenchmark("conv2d", problemSize, iterations, [&]() {
        // Step 1: im2col
        im2col_kernel<<<im2colGrid, im2colBlock>>>(d_input, d_im2col,
            inputDims, maskDims, strideDims, padDims, dilDims,
            IN_CHANNELS, BATCH);

        // Step 2: GEMM (weight * im2col_output)
        gemm<<<gemmGrid, gemmBlock>>>(gemmM, gemmN, gemmK,
            1.0f, 0.0f,
            d_weight, d_im2col, d_bias, d_output);
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_input));
    HIP_CHECK(hipFree(d_im2col));
    HIP_CHECK(hipFree(d_weight));
    HIP_CHECK(hipFree(d_bias));
    HIP_CHECK(hipFree(d_output));

    return 0;
}
