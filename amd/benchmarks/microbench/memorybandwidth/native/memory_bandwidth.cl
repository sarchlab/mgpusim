/*
 * Copyright (c) Advanced Micro Devices, Inc., or its affiliates.
 *
 * SPDX-License-Identifier: MIT
 */

// ROCr copy kernel used by hipMemcpyPeer/hipMemcpy D2D in the calibration
// run. Source: ROCm/rocm-systems at 43fa692e568e65a1fb84f2d1c74415b042839f10,
// projects/clr/rocclr/device/rocm/blitcl.cpp.
__attribute__((reqd_work_group_size(512, 1, 1)))
__kernel void __amd_rocclr_copyBuffer(__global uchar* src, __global uchar* dst, ulong size,
                                      uint remainder, uint aligned_size, ulong end_ptr,
                                      uint next_chunk, uint workgroup_size) {
  uint l = __builtin_amdgcn_workitem_id_x();
  uint g = __builtin_amdgcn_workgroup_id_x();
  ulong id = (g * workgroup_size + l);
  ulong id_remainder = id;

  if (aligned_size == sizeof(ulong2)) {
    __global ulong2* srcD = (__global ulong2*)(src);
    __global ulong2* dstD = (__global ulong2*)(dst);
    while ((ulong)(&dstD[id]) < end_ptr) {
      dstD[id] = srcD[id];
      id += next_chunk;
    }
  } else {
    __global uint* srcD = (__global uint*)(src);
    __global uint* dstD = (__global uint*)(dst);
    while ((ulong)(&dstD[id]) < end_ptr) {
      dstD[id] = srcD[id];
      id += next_chunk;
    }
  }
  if ((remainder != 0) && (id_remainder == 0)) {
    for (ulong i = size - remainder; i < size; ++i) {
      dst[i] = src[i];
    }
  }
}
