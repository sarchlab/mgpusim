/**
 * kmeans.cpp: HIP implementation of K-means clustering kernels
 * Translated from kernels.cl for gfx942 CDNA3 architecture
 */

#include "hip/hip_runtime.h"

#ifndef FLT_MAX
#define FLT_MAX 3.40282347e+38
#endif

extern "C" __global__ void kmeans_kernel_compute(float *feature,
                                    float *clusters,
                                    int *membership, int npoints,
                                    int nclusters, int nfeatures, int offset,
                                    int size) {
  unsigned int point_id = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;

  int index = 0;
  if (point_id < npoints) {
    float min_dist = FLT_MAX;
    for (int i = 0; i < nclusters; i++) {
      float dist = 0;
      float ans = 0;
      for (int l = 0; l < nfeatures; l++) {
        ans += (feature[l * npoints + point_id] - clusters[i * nfeatures + l]) *
               (feature[l * npoints + point_id] - clusters[i * nfeatures + l]);
      }

      dist = ans;
      if (dist < min_dist) {
        min_dist = dist;
        index = i;
      }
    }
    membership[point_id] = index;
  }

  return;
}

// hint: 2D transpose
extern "C" __global__ void kmeans_kernel_swap(float *feature,
                                 float *feature_swap, int npoints,
                                 int nfeatures) {
  unsigned int tid = hipBlockDim_x * hipBlockIdx_x + hipThreadIdx_x;
  if (tid >= npoints) return;

  for (int i = 0; i < nfeatures; i++)
    feature_swap[i * npoints + tid] = feature[tid * nfeatures + i];
}
