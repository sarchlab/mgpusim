/*
 * HIP kernel for Matrix Transpose (gfx942 / CDNA3)
 * Translated from MatrixTranspose_Kernels.cl
 *
 * Copies a block to shared memory and copies back the transpose.
 * Each work-item operates on a [4x4] matrix element block using float4.
 */
#include "hip/hip_runtime.h"

extern "C" __global__ void matrixTranspose(
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
    output[index_out]              = make_float4(v0.x, v1.x, v2.x, v3.x);
    output[index_out + wiHeight]   = make_float4(v0.y, v1.y, v2.y, v3.y);
    output[index_out + wiHeight * 2] = make_float4(v0.z, v1.z, v2.z, v3.z);
    output[index_out + wiHeight * 3] = make_float4(v0.w, v1.w, v2.w, v3.w);
}
