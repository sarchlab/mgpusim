// matrixtranspose.cpp — HIP benchmark for matrix transpose
// Kernel copied from amd/benchmarks/amdappsdk/matrixtranspose/native/matrixtranspose_hip.cpp
// Problem size: 1024x1024 matrix per issue #153
//
// The kernel operates on float4 elements with a 16x16 work-group,
// each thread handling a 4x4 block of elements.

#include "bench_common.h"

__global__ void matrixTranspose(
    float4* __restrict__ output,
    float4* __restrict__ input,
    float4* __restrict__ block, // unused — LDS allocated dynamically
    unsigned int wiWidth,
    unsigned int wiHeight,
    unsigned int num_of_blocks_x,
    unsigned int group_x_offset,
    unsigned int group_y_offset)
{
    HIP_DYNAMIC_SHARED(float4, lds);

    unsigned int lix = hipThreadIdx_x;
    unsigned int liy = hipThreadIdx_y;

    unsigned int gix_t = hipBlockIdx_x;
    unsigned int giy_t = hipBlockIdx_y;

    gix_t += group_x_offset;
    giy_t += group_y_offset;

    // break memory banks dependency by "reshuffling" global indices
    unsigned int giy = gix_t;
    unsigned int gix = (gix_t + giy_t) % num_of_blocks_x;

    unsigned int blockSize = hipBlockDim_x;

    unsigned int ix = gix * blockSize + lix;
    unsigned int iy = giy * blockSize + liy;
    int index_in = ix + (iy) * wiWidth * 4;

    // coalesced copy from input global memory into LDS
    int ind = liy * blockSize * 4 + lix;
    lds[ind]                 = input[index_in];
    lds[ind + blockSize]     = input[index_in + wiWidth];
    lds[ind + blockSize * 2] = input[index_in + wiWidth * 2];
    lds[ind + blockSize * 3] = input[index_in + wiWidth * 3];

    // wait until the whole block is filled
    __syncthreads();

    // calculate the corresponding target
    ix = giy * blockSize + lix;
    iy = gix * blockSize + liy;
    int index_out = ix + (iy) * wiHeight * 4;

    ind = lix * blockSize * 4 + liy;
    float4 v0 = lds[ind];
    float4 v1 = lds[ind + blockSize];
    float4 v2 = lds[ind + blockSize * 2];
    float4 v3 = lds[ind + blockSize * 3];

    // coalesced copy of transposed data in LDS into output global memory
    output[index_out]                = make_float4(v0.x, v1.x, v2.x, v3.x);
    output[index_out + wiHeight]     = make_float4(v0.y, v1.y, v2.y, v3.y);
    output[index_out + wiHeight * 2] = make_float4(v0.z, v1.z, v2.z, v3.z);
    output[index_out + wiHeight * 3] = make_float4(v0.w, v1.w, v2.w, v3.w);
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);
    // WIDTH must be a multiple of 64 (BLOCK_SIZE=16 * ELEMS_PER_THREAD_1D=4)
    int WIDTH = parseIntParam(argc, argv, "--size", 1024);
    const int ELEMS_PER_THREAD_1D = 4;    // each thread handles 4x4 block
    const int BLOCK_SIZE = 16;            // work-group size per dim

    const int numData = WIDTH * WIDTH;

    // Host allocations (uint32 stored as float4 for the kernel)
    size_t dataBytes = numData * sizeof(float);
    float* hInput  = (float*)malloc(dataBytes);
    float* hOutput = (float*)malloc(dataBytes);

    for (int i = 0; i < numData; i++) {
        hInput[i] = (float)i;
    }

    // Device allocations
    float *dInput, *dOutput;
    HIP_CHECK(hipMalloc(&dInput, dataBytes));
    HIP_CHECK(hipMalloc(&dOutput, dataBytes));
    HIP_CHECK(hipMemcpy(dInput, hInput, dataBytes, hipMemcpyHostToDevice));

    // Kernel launch parameters
    unsigned int wiWidth  = WIDTH / ELEMS_PER_THREAD_1D;   // 256
    unsigned int wiHeight = WIDTH / ELEMS_PER_THREAD_1D;   // 256
    unsigned int numWGWidth = wiWidth / BLOCK_SIZE;         // 16

    dim3 block(BLOCK_SIZE, BLOCK_SIZE, 1);
    dim3 grid(wiWidth / BLOCK_SIZE, wiHeight / BLOCK_SIZE, 1);

    // LDS size: blockSize * blockSize * elemsPerThread1D^2 * sizeof(float)
    size_t ldsBytes = BLOCK_SIZE * BLOCK_SIZE *
                      ELEMS_PER_THREAD_1D * ELEMS_PER_THREAD_1D * sizeof(float);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%dx%d", WIDTH, WIDTH);

    BenchResult r = runBenchmark("matrixtranspose", problemSize, iterations, [&]() {
        matrixTranspose<<<grid, block, ldsBytes>>>(
            (float4*)dOutput, (float4*)dInput, nullptr,
            wiWidth, wiHeight, numWGWidth,
            0, 0);
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(dInput));
    HIP_CHECK(hipFree(dOutput));
    free(hInput);
    free(hOutput);

    return 0;
}
