#pragma OPENCL EXTENSION cl_khr_global_int32_base_atomics : enable
#pragma OPENCL EXTENSION cl_khr_local_int32_base_atomics : enable
#pragma OPENCL EXTENSION cl_khr_local_int32_extended_atomics: enable
#pragma OPENCL EXTENSION cl_khr_global_int32_extended_atomics: enable


//Sungpack Hong, Sang Kyun Kim, Tayo Oguntebi, and Kunle Olukotun. 2011.
//Accelerating CUDA graph algorithms at maximum warp.
//In Proceedings of the 16th ACM symposium on Principles and practice of
//parallel programming (PPoPP '11). ACM, New York, NY, USA, 267-276.
// ****************************************************************************
// Function: BFS_kernel_warp
//
// Purpose:
//   Perform BFS on the given graph
//
// Arguments:
//   levels: array that stores the level of vertices
//   edgeArray: array that gives offset of a vertex in edgeArrayAux
//   edgeArrayAux: array that gives the edge list of a vertex
//   W_SZ: the warp size to use to process vertices
//   CHUNK_SZ: the number of vertices each warp processes
//   numVertices: number of vertices in the given graph
//   curr: the current BFS level
//   flag: set when more vertices remain to be traversed
//
// Returns:  nothing
//
// Programmer: Aditya Sarwade
// Creation: June 16, 2011
//
// Modifications:
//
// ****************************************************************************
__kernel void BFS_kernel_warp(
        __global unsigned int *levels,
        __global unsigned int *edgeArray,
        __global unsigned int *edgeArrayAux,
        int W_SZ,
        int CHUNK_SZ,
        unsigned int numVertices,
        int curr,
        __global int *flag)
{

    int tid = get_global_id(0);
    int W_OFF = tid % W_SZ;
    int W_ID = tid / W_SZ;
    int v1= W_ID * CHUNK_SZ;
    int chk_sz=CHUNK_SZ+1;

    if((v1+CHUNK_SZ)>=numVertices)
    {
        chk_sz =  numVertices-v1+1;//(v1+CHUNK_SZ) - numVertices;
        if(chk_sz<0)
            chk_sz=0;
    }

    //each warp processes nodes one by one
    for(int v=v1; v< chk_sz-1+v1; v++)
    {
        if(levels[v] == curr)
        {
            unsigned int num_nbr = edgeArray[v+1]-edgeArray[v];
            unsigned int nbr_off = edgeArray[v];
            for(int i=W_OFF; i<num_nbr; i+=W_SZ)
            {
               int v = edgeArrayAux[i + nbr_off];
               if(levels[v]==UINT_MAX)
               {
                    levels[v] = curr + 1;
                    *flag = 1;
               }
            }
        }
    }
}