/**********************************************************************
Copyright �2015 Advanced Micro Devices, Inc. All rights reserved.

Redistribution and use in source and binary forms, with or without modification, are permitted provided that the following conditions are met:

�	Redistributions of source code must retain the above copyright notice, this list of conditions and the following disclaimer.
�	Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the following disclaimer in the documentation and/or
 other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
 WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY
 DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS
 OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING
 NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
********************************************************************/

#define TILEX 4
#define TILEX_SHIFT 2
#define TILEY 4
#define TILEY_SHIFT 2

/* Matrix A is cached into local memory block */
/* Required global threads = (widthC / 4, heightC / 4) */
__kernel void mmmKernel_local(__global float4 *matrixA,
                              __global float4 *matrixB,
                              __global float4* matrixC,
                              int widthA,
                              __local float4 *blockA)
{
    int lIdX = get_local_id(0);
    int lIdY = get_local_id(1);
    int lSizeX = get_local_size(0);
    int gIdX = get_global_id(0);
    int gIdY = get_global_id(1);
    int gSizeX = get_global_size(0);

    int blockPos = lIdX + lSizeX * (lIdY << TILEY_SHIFT); //Should be : localId * (TILEX / 4) (float4)

    /* Position of thread will be according to the number of values it writes i.e TILE size */
    int globalPos =  gIdX + (gIdY << TILEY_SHIFT) * gSizeX;

    /* Each thread writes 4 float4s */
    float4 sum0 = (float4)(0);
    float4 sum1 = (float4)(0);
    float4 sum2 = (float4)(0);
    float4 sum3 = (float4)(0);

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

        barrier(CLK_LOCAL_MEM_FENCE);

        /* Calculate global ids of threads from the particular block to load from matrix B depending on i */
        int globalPosB = gIdX + ((i * lSizeX) << TILEY_SHIFT) * gSizeX;

        /* This loop runs for number of threads in horizontal direction in the block of A */
        for(int j = 0; j < lSizeX * 4; j=j+4)
        {
            /* Load 4 float4s from blockA : access patters = strided from local memory */
            float4 tempA0 = blockA[(j >> 2) + lIdY * TILEY * lSizeX];
            float4 tempA1 = blockA[(j >> 2) + (lIdY * TILEY + 1) * lSizeX];
            float4 tempA2 = blockA[(j >> 2) + (lIdY * TILEY + 2) * lSizeX];
            float4 tempA3 = blockA[(j >> 2) + (lIdY * TILEY + 3) * lSizeX];

            /* Load corresponding values from matrixB, access pattern = linear from global memory */
            float4 tempB0 = matrixB[globalPosB  + j *  gSizeX]; //Should be localId.x * (TILEX / 4)
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
		barrier(CLK_LOCAL_MEM_FENCE);
    }
    /* Write 16 values to matrixC */
    matrixC[globalPos] = sum0;
    matrixC[globalPos +  gSizeX] = sum1;
    matrixC[globalPos +  2 * gSizeX] = sum2;
    matrixC[globalPos +  3 * gSizeX] = sum3;

}

