/**********************************************************************
Copyright ©2014 Advanced Micro Devices, Inc. All rights reserved.

Redistribution and use in source and binary forms, with or without modification,
are permitted provided that the following conditions are met:

• Redistributions of source code must retain the above copyright notice, this
list of conditions and the following disclaimer.

• Redistributions in binary form must reproduce the above copyright notice, this
list of conditions and the following disclaimer in the documentation and/or
other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
 ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
 WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
 DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
 FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
 DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
 SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
 CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
 OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
 OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 ********************************************************************/

/*
 * For a description of the algorithm and the terms used, please see the
 * documentation for this sample.
 *
 * One invocation of this kernel, i.e one work thread writes two output values.
 * Since every pass of this algorithm does width/2 comparisons, each compare
 * operation is done by one work thread.
 *
 * Depending of the direction of sort for the work thread, the output values
 * are written either as greater value to left element or lesser value to the
 * left element. Right element and left element are the two elements we are
 * comparing and "left" is the element with a smaller index into the array.
 *
 * if direction is CL_TRUE, i.e evaluates to non zero, it means "increasing".
 *
 * For an explanation of the terms "blockWidth", "sameDirectionBlockWidth",
 * stage, pass, pairDistance please go through the document shipped with this
 * sample.
 *
 * Since an explanation of the terms and the code here would be quite lengthy,
 * confusing and will greatly reduce the readability of this kernel, the code
 * has been explained in detail in the document mentioned above. */

__kernel void BitonicSort(__global uint* array, const uint stage,
                          const uint passOfStage, const uint direction) {
  uint sortIncreasing = direction;
  uint threadId = get_global_id(0);

  uint pairDistance = 1 << (stage - passOfStage);
  uint blockWidth = 2 * pairDistance;

  uint leftId =
      (threadId % pairDistance) + (threadId / pairDistance) * blockWidth;

  uint rightId = leftId + pairDistance;

  uint leftElement = array[leftId];
  uint rightElement = array[rightId];

  uint sameDirectionBlockWidth = 1 << stage;

  if ((threadId / sameDirectionBlockWidth) % 2 == 1)
    sortIncreasing = 1 - sortIncreasing;

  uint greater;
  uint lesser;
  if (leftElement > rightElement) {
    greater = leftElement;
    lesser = rightElement;
  } else {
    greater = rightElement;
    lesser = leftElement;
  }

  if (sortIncreasing) {
    array[leftId] = lesser;
    array[rightId] = greater;
  } else {
    array[leftId] = greater;
    array[rightId] = lesser;
  }
}
