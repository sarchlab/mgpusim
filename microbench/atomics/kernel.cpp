////////////////////////////////////////////////////////////////////////////////
//
// The University of Illinois/NCSA
// Open Source License (NCSA)
//
// Copyright (c) 2016, Advanced Micro Devices, Inc. All rights reserved.
//
// Developed by:
//
//                 AMD Research and AMD HSA Software Development
//
//                 Advanced Micro Devices, Inc.
//
//                 www.amd.com
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal with the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
//  - Redistributions of source code must retain the above copyright notice,
//    this list of conditions and the following disclaimers.
//  - Redistributions in binary form must reproduce the above copyright
//    notice, this list of conditions and the following disclaimers in
//    the documentation and/or other materials provided with the distribution.
//  - Neither the names of Advanced Micro Devices, Inc,
//    nor the names of its contributors may be used to endorse or promote
//    products derived from this Software without specific prior written
//    permission.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE CONTRIBUTORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
// OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS WITH THE SOFTWARE.
//
////////////////////////////////////////////////////////////////////////////////

#include "dispatch.hpp"


using namespace amd::dispatch;

class AtomicAdd : public Dispatch {
private:
  Buffer* sum;
  unsigned length;

public:
  AtomicAdd(int argc, const char **argv)
    : Dispatch(argc, argv), length(1) { }

  bool SetupCodeObject() override {
    return LoadCodeObjectFromFile("kernels.hsaco");
  }

  bool Setup() override {
    if (!AllocateKernarg(1 * sizeof(Buffer*))) { return false; }
    sum = AllocateBuffer(length * sizeof(int));
    for (unsigned i = 0; i < length; ++i) {
      sum->Data<int>(i) = 0;
    }
    if (!CopyTo(sum)) { output << "Error: failed to copy to local" << std::endl; return false; }
   
    Kernarg(sum); 
    SetGridSize(1024);
    SetWorkgroupSize(1024);
    return true;
  }

  bool Verify() override {
    if (!CopyFrom(sum)) { output << "Error: failed to copy from local" << std::endl; return false; }
  
    int res; 
    for (unsigned i = 0; i < length; ++i) {  
        res = sum->Data<int>(i);
    }

    printf("Result is %d \n", res);
  }
};

int main(int argc, const char** argv)
{
  return AtomicAdd(argc, argv).RunMain();
}
