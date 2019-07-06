/**********************************************************************
Copyright ©2015 Advanced Micro Devices, Inc. All rights reserved.

Redistribution and use in source and binary forms, with or without modification, are permitted provided that the following conditions are met:

•	Redistributions of source code must retain the above copyright notice, this list of conditions and the following disclaimer.
•	Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the following disclaimer in the documentation and/or
 other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
 WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY
 DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS
 OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING
 NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
********************************************************************/

/*
 * Copies a block to the local memory 
 * and copies back the transpose from local memory to output.
 * Each kernel/WI works on [4x4] matrix elements.
 * @param output output matrix
 * @param input  input matrix
 * @param block  local memory buffer
 */

__kernel 
void matrixTranspose(__global float4 * output,
					 __global float4 * input,
					 __local  float4 * block,
                     uint wiWidth, 
                     uint wiHeight,
                     uint num_of_blocks_x,
                     uint group_x_offset,
                     uint group_y_offset
                     )
{
	uint lix = get_local_id(0);
	uint liy = get_local_id(1);

    /*__asm__ volatile ("v_log_legacy_f32 v24, %0":"=v" (lix));*/
    /*__asm__ volatile ("v_log_legacy_f32 v25, %0":"=v" (liy));*/


    uint gix_t = get_group_id(0);
    uint giy_t = get_group_id(1);	

    gix_t += group_x_offset;
    giy_t += group_y_offset;

    // break memory banks dependency by "reshuffling" global indeces
    uint giy = gix_t;
    uint gix = (gix_t+giy_t)%num_of_blocks_x;

    uint blockSize = get_local_size(0);

    uint ix = gix*blockSize + lix;
    uint iy = giy*blockSize + liy;
    int index_in = ix + (iy)*wiWidth*4;

    /*__asm__ volatile ("v_log_legacy_f32 v26, %0":"=v" (index_in));*/

    // coalesced copy from input global memory into LDS
    int ind = liy*blockSize*4+lix;
    block[ind]              = input[index_in];
    block[ind+blockSize]	= input[index_in+wiWidth];
    block[ind+blockSize*2]  = input[index_in+wiWidth*2];
    block[ind+blockSize*3]  = input[index_in+wiWidth*3];
        
    // wait until the whole block is filled
    barrier(CLK_LOCAL_MEM_FENCE);
    
    // calculate the corresponding target 
    // as location inside block of transposed location
    ix = giy*blockSize + lix;
    iy = gix*blockSize + liy;
    int index_out = ix + (iy)*wiHeight*4;

    ind = lix*blockSize*4+liy;
    float4 v0 = block[ind];
    float4 v1 = block[ind+blockSize];
    float4 v2 = block[ind+blockSize*2];
    float4 v3 = block[ind+blockSize*3];
    
    // coalesced copy of transposed data in LDS into output global memory
    output[index_out]			= (float4)(v0.x, v1.x, v2.x, v3.x);
    output[index_out+wiHeight]	= (float4)(v0.y, v1.y, v2.y, v3.y);
    output[index_out+wiHeight*2]	= (float4)(v0.z, v1.z, v2.z, v3.z);
    output[index_out+wiHeight*3]	= (float4)(v0.w, v1.w, v2.w, v3.w);
}
