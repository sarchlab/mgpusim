// GPU Benchmarks v1.1, tier2/rodinia_nw/hip/rodinia_nw.hip.
/* BLOSUM62 table from Rodinia 3.1, pin366b283456506ef2fe2c2b7dc5e83e941ffc5524.
LICENSE TERMS

Copyright (c)2008-2011 University of Virginia
All rights reserved.

Redistribution and use in source and binary forms, with or without modification, are permitted without royalty fees or other restrictions, provided that the following conditions are met:

    * Redistributions of source code must retain the above copyright notice, this list of conditions and the following disclaimer.
    * Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the following disclaimer in the documentation and/or other materials provided with the distribution.
    * Neither the name of the University of Virginia, the Dept. of Computer Science, nor the names of its contributors may be used to endorse or promote products derived from this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE UNIVERSITY OF VIRGINIA OR THE SOFTWARE AUTHORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/
#include <hip/hip_runtime.h>

extern "C" __global__ void nw_kernel1(const int* __restrict__ d_reference,
                            int* __restrict__ d_matrix,
                            int cols,
                            int penalty,
                            int block_idx,
                            int block_size)
{
    int bx = hipBlockIdx_x;
    int tx = hipThreadIdx_x;

    // Filter out blocks beyond diagonal
    if (bx > block_idx) return;

    int b_row = block_idx - bx;
    int b_col = bx;

    // Global starting position (1-indexed within the matrix)
    int begin_row = b_row * block_size + 1;
    int begin_col = b_col * block_size + 1;

    // Shared memory layout:
    //   temp[0..(block_size+1)^2-1] : local DP tile (block_size+1 x block_size+1)
    //   ref_shared[0..block_size^2-1] : BLOSUM62 reference tile
    extern __shared__ int shared[];
    int* temp       = shared;
    int* ref_shared = &shared[(block_size + 1) * (block_size + 1)];

    // Cooperatively load the precomputed BLOSUM62 tile, as in Rodinia.
    for (int row = 0; row < block_size; ++row)
        ref_shared[row * block_size + tx] =
            d_reference[(begin_row + row) * cols + begin_col + tx];
    __syncthreads();

    // Thread 0 loads the border row and column from global memory
    if (tx == 0) {
        // Left border column (from global matrix)
        for (int i = 0; i <= block_size; i++) {
            temp[i * (block_size + 1)] =
                d_matrix[(begin_row + i - 1) * cols + (begin_col - 1)];
        }
        // Top border row (from global matrix)
        for (int j = 0; j <= block_size; j++) {
            temp[j] = d_matrix[(begin_row - 1) * cols + (begin_col + j - 1)];
        }
    }
    __syncthreads();

    // Anti-diagonal sweep within the tile
    for (int diag = 0; diag < 2 * block_size - 1; diag++) {
        // Determine this thread's (i, j) within the tile [1..block_size]
        int i = (tx <= diag && tx < block_size && (diag - tx) < block_size)
                    ? tx + 1 : -1;
        int j = (i > 0) ? diag - tx + 1 : -1;

        if (i > 0 && j > 0 && i <= block_size && j <= block_size) {
            int idx_diag   = i * (block_size + 1) + j;
            int idx_up     = (i - 1) * (block_size + 1) + j;
            int idx_left   = i * (block_size + 1) + (j - 1);
            int idx_upleft = (i - 1) * (block_size + 1) + (j - 1);

            int match = temp[idx_upleft] +
                        ref_shared[(i - 1) * block_size + j - 1];
            int del   = temp[idx_up]   - penalty;
            int ins   = temp[idx_left] - penalty;

            int val = match;
            if (del > val) val = del;
            if (ins > val) val = ins;
            temp[idx_diag] = val;
        }
        __syncthreads();
    }

    // Write computed tile back to global matrix
    if (tx < block_size) {
        for (int i = 1; i <= block_size; i++) {
            d_matrix[(begin_row + i - 1) * cols + (begin_col + tx)] =
                temp[i * (block_size + 1) + tx + 1];
        }
    }
}

// ---------------------------------------------------------------------------
// Kernel 2: lower-right triangle of the block dependency graph
//
// For diagonal block_idx = 0..num_blocks-2, dispatches (num_blocks-1-block_idx) blocks.
// Block bx handles block position (row = num_blocks-1-bx, col = block_idx+1+bx).
// ---------------------------------------------------------------------------
extern "C" __global__ void nw_kernel2(const int* __restrict__ d_reference,
                            int* __restrict__ d_matrix,
                            int cols,
                            int penalty,
                            int block_idx,
                            int num_blocks,
                            int block_size)
{
    int bx = hipBlockIdx_x;
    int tx = hipThreadIdx_x;

    int blocks_on_diag = num_blocks - 1 - block_idx;
    if (bx >= blocks_on_diag) return;

    int b_row = num_blocks - 1 - bx;
    int b_col = block_idx + 1 + bx;

    int begin_row = b_row * block_size + 1;
    int begin_col = b_col * block_size + 1;

    extern __shared__ int shared[];
    int* temp       = shared;
    int* ref_shared = &shared[(block_size + 1) * (block_size + 1)];

    for (int row = 0; row < block_size; ++row)
        ref_shared[row * block_size + tx] =
            d_reference[(begin_row + row) * cols + begin_col + tx];
    __syncthreads();

    if (tx == 0) {
        for (int i = 0; i <= block_size; i++) {
            temp[i * (block_size + 1)] =
                d_matrix[(begin_row + i - 1) * cols + (begin_col - 1)];
        }
        for (int j = 0; j <= block_size; j++) {
            temp[j] = d_matrix[(begin_row - 1) * cols + (begin_col + j - 1)];
        }
    }
    __syncthreads();

    for (int diag = 0; diag < 2 * block_size - 1; diag++) {
        int i = (tx <= diag && tx < block_size && (diag - tx) < block_size)
                    ? tx + 1 : -1;
        int j = (i > 0) ? diag - tx + 1 : -1;

        if (i > 0 && j > 0 && i <= block_size && j <= block_size) {
            int idx_diag   = i * (block_size + 1) + j;
            int idx_up     = (i - 1) * (block_size + 1) + j;
            int idx_left   = i * (block_size + 1) + (j - 1);
            int idx_upleft = (i - 1) * (block_size + 1) + (j - 1);

            int match = temp[idx_upleft] +
                        ref_shared[(i - 1) * block_size + j - 1];
            int del   = temp[idx_up]   - penalty;
            int ins   = temp[idx_left] - penalty;

            int val = match;
            if (del > val) val = del;
            if (ins > val) val = ins;
            temp[idx_diag] = val;
        }
        __syncthreads();
    }

    if (tx < block_size) {
        for (int i = 1; i <= block_size; i++) {
            d_matrix[(begin_row + i - 1) * cols + (begin_col + tx)] =
                temp[i * (block_size + 1) + tx + 1];
        }
    }
}
