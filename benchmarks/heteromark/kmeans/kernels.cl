/*
 * Copyright (c) 2015 Northeastern University
 * All rights reserved.
 *
 * Developed by:Northeastern University Computer Architecture Research (NUCAR)
 * Group, Northeastern University, http://www.ece.neu.edu/groups/nucar/
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 *  with the Software without restriction, including without limitation
 * the rights to use, copy, modify, merge, publish, distribute, sublicense, and/
 * or sell copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 *   Redistributions of source code must retain the above copyright notice, this
 *   list of conditions and the following disclaimers. Redistributions in binary
 *   form must reproduce the above copyright notice, this list of conditions and
 *   the following disclaimers in the documentation and/or other materials
 *   provided with the distribution. Neither the names of NUCAR, Northeastern
 *   University, nor the names of its contributors may be used to endorse or
 *   promote products derived from this Software without specific prior written
 *   permission.
 *
 *   THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 *   IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 *   FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 *   CONTRIBUTORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 *   LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
 *   FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
 *   DEALINGS WITH THE SOFTWARE.
 */

#ifndef FLT_MAX
#define FLT_MAX 3.40282347e+38
#endif

__kernel void kmeans_kernel_compute(__global float *feature,
                                    __global float *clusters,
                                    __global int *membership, int npoints,
                                    int nclusters, int nfeatures, int offset,
                                    int size) {
  unsigned int point_id = get_global_id(0);

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
__kernel void kmeans_kernel_swap(__global float *feature,
                                 __global float *feature_swap, int npoints,
                                 int nfeatures) {
  unsigned int tid = get_global_id(0);
  if (tid >= npoints) return;

  for (int i = 0; i < nfeatures; i++)
    feature_swap[i * npoints + tid] = feature[tid * nfeatures + i];
}
