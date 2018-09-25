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

__kernel void FIR(__global float* output, __global float* coeff,
                  __global float* input, __global float* history, 
                  uint num_tap) {
  uint tid = get_global_id(0);
  uint num_data = get_global_size(0);

  float sum = 0;
  uint i = 0;
  for (i = 0; i < num_tap; i++) {
    if (tid >= i) {
        sum = sum + coeff[i] * input[tid - i];
    } else {
        sum = sum + coeff[i] * history[num_tap - (i - tid)];
    }
  }
  output[tid] = sum;

  /*barrier(CLK_GLOBAL_MEM_FENCE);*/

  /*[> fill the history buffer <]*/
  /*if (tid >= numData - numTap + 1)*/
    /*temp_input[tid - (numData - numTap + 1)] = temp_input[xid];*/

  /*barrier(CLK_GLOBAL_MEM_FENCE);*/
}
