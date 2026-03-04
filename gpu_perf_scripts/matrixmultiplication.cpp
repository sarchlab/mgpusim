// matrixmultiplication.cpp — HIP benchmark for tiled matrix multiplication
// Kernel copied from amd/benchmarks/amdappsdk/matrixmultiplication/native/matrixmultiplication.cpp
// Problem size: x=128, y=128, z=128 (A[128x128] * B[128x128] = C[128x128])

#include "bench_common.h"

#define TILEX 4
#define TILEX_SHIFT 2
#define TILEY 4
#define TILEY_SHIFT 2

/* Matrix A is cached into local memory block */
/* Required global threads = (widthC / 4, heightC / 4) */
extern "C" __global__ void mmmKernel_local(float4 *matrixA,
                              float4 *matrixB,
                              float4* matrixC,
                              int widthA,
                              float4 *blockA)
{
    int lIdX = hipThreadIdx_x;
    int lIdY = hipThreadIdx_y;
    int lSizeX = hipBlockDim_x;
    int gIdX = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
    int gIdY = hipBlockDim_y * hipBlockIdx_y + hipThreadIdx_y;
    int gSizeX = hipGridDim_x * hipBlockDim_x;

    int blockPos = lIdX + lSizeX * (lIdY << TILEY_SHIFT);

    /* Position of thread will be according to the number of values it writes i.e TILE size */
    int globalPos =  gIdX + (gIdY << TILEY_SHIFT) * gSizeX;

    /* Each thread writes 4 float4s */
    float4 sum0 = make_float4(0.0f, 0.0f, 0.0f, 0.0f);
    float4 sum1 = make_float4(0.0f, 0.0f, 0.0f, 0.0f);
    float4 sum2 = make_float4(0.0f, 0.0f, 0.0f, 0.0f);
    float4 sum3 = make_float4(0.0f, 0.0f, 0.0f, 0.0f);

    int temp = widthA / 4;
    int numLoops = temp / lSizeX;

    /* This loop runs for number of blocks of A in horizontal direction */
    for(int i = 0; i < numLoops; i++)
    {
        /* Calculate global ids of threads from the particular block to load from matrix A depending on i */
        int globalPosA = i * lSizeX + lIdX + (lIdY << TILEY_SHIFT) * temp;

        /* Load values in blockA from matrixA */
        blockA[blockPos]                = matrixA[globalPosA];
        blockA[blockPos + lSizeX]       = matrixA[globalPosA + temp];
        blockA[blockPos + 2 * lSizeX]   = matrixA[globalPosA + 2 * temp];
        blockA[blockPos + 3 * lSizeX]   = matrixA[globalPosA + 3 * temp];

        __syncthreads();

        /* Calculate global ids of threads from the particular block to load from matrix B depending on i */
        int globalPosB = gIdX + ((i * lSizeX) << TILEY_SHIFT) * gSizeX;

        /* This loop runs for number of threads in horizontal direction in the block of A */
        for(int j = 0; j < lSizeX * 4; j=j+4)
        {
            /* Load 4 float4s from blockA */
            float4 tempA0 = blockA[(j >> 2) + lIdY * TILEY * lSizeX];
            float4 tempA1 = blockA[(j >> 2) + (lIdY * TILEY + 1) * lSizeX];
            float4 tempA2 = blockA[(j >> 2) + (lIdY * TILEY + 2) * lSizeX];
            float4 tempA3 = blockA[(j >> 2) + (lIdY * TILEY + 3) * lSizeX];

            /* Load corresponding values from matrixB */
            float4 tempB0 = matrixB[globalPosB  + j *  gSizeX];
            float4 tempB1 = matrixB[globalPosB  + (j + 1) * gSizeX];
            float4 tempB2 = matrixB[globalPosB  + (j + 2) * gSizeX];
            float4 tempB3 = matrixB[globalPosB  + (j + 3) * gSizeX];

            sum0.x += tempA0.x * tempB0.x + tempA0.y * tempB1.x + tempA0.z * tempB2.x + tempA0.w * tempB3.x;
            sum0.y += tempA0.x * tempB0.y + tempA0.y * tempB1.y + tempA0.z * tempB2.y + tempA0.w * tempB3.y;
            sum0.z += tempA0.x * tempB0.z + tempA0.y * tempB1.z + tempA0.z * tempB2.z + tempA0.w * tempB3.z;
            sum0.w += tempA0.x * tempB0.w + tempA0.y * tempB1.w + tempA0.z * tempB2.w + tempA0.w * tempB3.w;

            sum1.x += tempA1.x * tempB0.x + tempA1.y * tempB1.x + tempA1.z * tempB2.x + tempA1.w * tempB3.x;
            sum1.y += tempA1.x * tempB0.y + tempA1.y * tempB1.y + tempA1.z * tempB2.y + tempA1.w * tempB3.y;
            sum1.z += tempA1.x * tempB0.z + tempA1.y * tempB1.z + tempA1.z * tempB2.z + tempA1.w * tempB3.z;
            sum1.w += tempA1.x * tempB0.w + tempA1.y * tempB1.w + tempA1.z * tempB2.w + tempA1.w * tempB3.w;

            sum2.x += tempA2.x * tempB0.x + tempA2.y * tempB1.x + tempA2.z * tempB2.x + tempA2.w * tempB3.x;
            sum2.y += tempA2.x * tempB0.y + tempA2.y * tempB1.y + tempA2.z * tempB2.y + tempA2.w * tempB3.y;
            sum2.z += tempA2.x * tempB0.z + tempA2.y * tempB1.z + tempA2.z * tempB2.z + tempA2.w * tempB3.z;
            sum2.w += tempA2.x * tempB0.w + tempA2.y * tempB1.w + tempA2.z * tempB2.w + tempA2.w * tempB3.w;

            sum3.x += tempA3.x * tempB0.x + tempA3.y * tempB1.x + tempA3.z * tempB2.x + tempA3.w * tempB3.x;
            sum3.y += tempA3.x * tempB0.y + tempA3.y * tempB1.y + tempA3.z * tempB2.y + tempA3.w * tempB3.y;
            sum3.z += tempA3.x * tempB0.z + tempA3.y * tempB1.z + tempA3.z * tempB2.z + tempA3.w * tempB3.z;
            sum3.w += tempA3.x * tempB0.w + tempA3.y * tempB1.w + tempA3.z * tempB2.w + tempA3.w * tempB3.w;
        }
        __syncthreads();
    }
    /* Write 16 values to matrixC */
    matrixC[globalPos] = sum0;
    matrixC[globalPos +  gSizeX] = sum1;
    matrixC[globalPos +  2 * gSizeX] = sum2;
    matrixC[globalPos +  3 * gSizeX] = sum3;
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);

    // Matrix dimensions: A[heightA x widthA] * B[widthA x widthB] = C[heightA x widthB]
    // For the acceptance test: x=128, y=128, z=128
    const int heightA = 128; // y
    const int widthA  = 128; // z (shared dimension)
    const int widthB  = 128; // x

    // Sizes in float4 (each float4 packs 4 floats)
    int sizeA_f4 = heightA * (widthA / 4);
    int sizeB_f4 = widthA * (widthB / 4);
    int sizeC_f4 = heightA * (widthB / 4);

    // Host allocations
    std::vector<float> h_A(heightA * widthA);
    std::vector<float> h_B(widthA * widthB);

    srand(42);
    for (int i = 0; i < heightA * widthA; i++)
        h_A[i] = (float)(rand() % 100) / 50.0f - 1.0f;
    for (int i = 0; i < widthA * widthB; i++)
        h_B[i] = (float)(rand() % 100) / 50.0f - 1.0f;

    // Device allocations
    float4 *d_A, *d_B, *d_C, *d_blockA;
    HIP_CHECK(hipMalloc(&d_A, sizeA_f4 * sizeof(float4)));
    HIP_CHECK(hipMalloc(&d_B, sizeB_f4 * sizeof(float4)));
    HIP_CHECK(hipMalloc(&d_C, sizeC_f4 * sizeof(float4)));

    // blockA is shared memory passed as global (same size as one block's worth)
    // The kernel uses blockA as a device-side scratch buffer
    // Allocate enough for all blocks: each block needs blockDim.x * blockDim.y * TILEY float4s
    int blockDimX = 8;  // threads per block in x (widthB/4 / TILEX = 128/4/4 = 8)
    int blockDimY = 8;  // threads per block in y (heightA / TILEY / gridDimY)
    int blockA_size = blockDimX * blockDimY * TILEY;
    int gridX = (widthB / 4) / blockDimX;
    int gridY = (heightA / 4) / blockDimY;
    int totalBlocks = gridX * gridY;
    HIP_CHECK(hipMalloc(&d_blockA, totalBlocks * blockA_size * sizeof(float4)));

    HIP_CHECK(hipMemcpy(d_A, h_A.data(), heightA * widthA * sizeof(float), hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_B, h_B.data(), widthA * widthB * sizeof(float), hipMemcpyHostToDevice));

    // Grid/block: global threads = (widthC/4, heightC/4) = (32, 32)
    dim3 block(blockDimX, blockDimY);
    dim3 grid(gridX, gridY);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%dx%dx%d", heightA, widthA, widthB);

    BenchResult r = runBenchmark("matrixmultiplication", problemSize, iterations, [&]() {
        mmmKernel_local<<<grid, block>>>(d_A, d_B, d_C, widthA, d_blockA);
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_A));
    HIP_CHECK(hipFree(d_B));
    HIP_CHECK(hipFree(d_C));
    HIP_CHECK(hipFree(d_blockA));

    return 0;
}
