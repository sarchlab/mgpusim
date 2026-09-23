#include "hip/hip_runtime.h"
#include <climits>

extern "C" __global__ void BFS_kernel_warp(
    unsigned int* levels,
    unsigned int* edgeArray,
    unsigned int* edgeArrayAux,
    int W_SZ,
    int CHUNK_SZ,
    unsigned int numVertices,
    int curr,
    int* flag)
{
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    int W_OFF = tid % W_SZ;
    int W_ID = tid / W_SZ;
    int v1 = W_ID * CHUNK_SZ;
    int chk_sz = CHUNK_SZ + 1;

    if ((v1 + CHUNK_SZ) >= (int)numVertices) {
        chk_sz = (int)numVertices - v1 + 1;
        if (chk_sz < 0) chk_sz = 0;
    }

    for (int v = v1; v < chk_sz - 1 + v1; v++) {
        if (levels[v] == (unsigned int)curr) {
            unsigned int num_nbr = edgeArray[v + 1] - edgeArray[v];
            unsigned int nbr_off = edgeArray[v];
            for (int i = W_OFF; i < (int)num_nbr; i += W_SZ) {
                int nv = edgeArrayAux[i + nbr_off];
                if (levels[nv] == UINT_MAX) {
                    levels[nv] = curr + 1;
                    *flag = 1;
                }
            }
        }
    }
}
