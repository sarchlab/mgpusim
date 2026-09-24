// hip_runtime.h: a minimal HIP-to-CUDA shim.
//
// The kernels under amd/benchmarks/*/*/native/ are written in HIP and include
// "hip/hip_runtime.h". Putting this directory first on nvcc's include path
// lets nvcc compile those kernel files unchanged. Only the HIP names used by
// the shared kernels are mapped; add more here when you port another kernel.
#ifndef MGPUSIM_NVIDIA_HIP_RUNTIME_SHIM_H
#define MGPUSIM_NVIDIA_HIP_RUNTIME_SHIM_H

#include <cuda_runtime.h>

#define hipThreadIdx_x threadIdx.x
#define hipThreadIdx_y threadIdx.y
#define hipThreadIdx_z threadIdx.z
#define hipBlockIdx_x blockIdx.x
#define hipBlockIdx_y blockIdx.y
#define hipBlockIdx_z blockIdx.z
#define hipBlockDim_x blockDim.x
#define hipBlockDim_y blockDim.y
#define hipBlockDim_z blockDim.z
#define hipGridDim_x gridDim.x
#define hipGridDim_y gridDim.y
#define hipGridDim_z gridDim.z

#endif  // MGPUSIM_NVIDIA_HIP_RUNTIME_SHIM_H
