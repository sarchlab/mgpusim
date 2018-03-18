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

#ifndef DISPATCH_HPP__
#define DISPATCH_HPP__

#include <sstream>
#include <cassert>
#include "hsa.h"
#include <string>

namespace amd {
namespace dispatch {

class Buffer {
private:
  size_t size;
  void *local_ptr, *system_ptr;

public:
  Buffer(size_t size_, void *local_ptr_, void *system_ptr_)
    : size(size_), local_ptr(local_ptr_), system_ptr(system_ptr_) { }
  Buffer(size_t size_, void *system_ptr_)
    : size(size_), local_ptr(system_ptr_), system_ptr(system_ptr_) { }
  void *LocalPtr() const { return local_ptr; }
  void *SystemPtr() { return system_ptr; }
  template <typename T>
  T* Ptr() { return (T*) system_ptr; }
  template <typename T>
  const T& Data(size_t i) const { return ((const T*) system_ptr)[i]; }
  template <typename T>
  T& Data(size_t i) { return ((T*) system_ptr)[i]; }
  bool IsLocal() const { return local_ptr != system_ptr; }
  size_t Size() const { return size; }
};

class Dispatch {
private:
  hsa_agent_t agent;
  hsa_agent_t cpu_agent;
  uint32_t queue_size;
  hsa_queue_t* queue;
  hsa_signal_t signal;
  hsa_region_t system_region;
  hsa_region_t kernarg_region;
  hsa_region_t local_region;
  hsa_region_t gpu_local_region;
  hsa_kernel_dispatch_packet_t* aql;
  uint64_t packet_index;
  uint32_t group_static_size;
  uint32_t group_dynamic_size;
  void *kernarg;
  size_t kernarg_offset;
  hsa_code_object_t code_object;
  hsa_executable_t executable;

  bool Init();
  bool InitDispatch();
  bool RunDispatch();
  bool Wait();

protected:
  std::ostringstream output;
  bool Error(const char *msg);
  bool HsaError(const char *msg, hsa_status_t status = HSA_STATUS_SUCCESS);

public:
  Dispatch(int argc, const char** argv);

  void SetAgent(hsa_agent_t agent) { assert(!this->agent.handle); this->agent = agent; }
  bool HasAgent() const { return agent.handle != 0; }
  void SetCpuAgent(hsa_agent_t agent) { assert(!this->cpu_agent.handle); this->cpu_agent = agent; }
  bool HasCpuAgent() const { return cpu_agent.handle != 0; }
  void SetWorkgroupSize(uint16_t sizeX, uint16_t sizeY = 1, uint16_t sizeZ = 1);
  void SetGridSize(uint32_t sizeX, uint32_t sizeY = 1, uint32_t sizeZ = 1);
  void SetSystemRegion(hsa_region_t region);
  void SetKernargRegion(hsa_region_t region);
  void SetLocalRegion(hsa_region_t region);
  void SetGPULocalRegion(hsa_region_t region);
  void SetDynamicGroupSegmentSize(uint32_t size);
  bool AllocateKernarg(uint32_t size);
  bool Run();
  int RunMain();
  virtual bool SetupExecutable();
  virtual bool SetupCodeObject();
  bool LoadCodeObjectFromFile(const std::string& filename);
  void* AllocateLocalMemory(size_t size);
  void* AllocateGPULocalMemory(size_t size);
  void* AllocateSystemMemory(size_t size);
  bool CopyToLocal(void* dest, void* src, size_t size);
  bool CopyFromLocal(void* dest, void* src, size_t size);
  Buffer* AllocateBuffer(size_t size, bool prefer_gpu_local=true);
  bool CopyTo(Buffer* buffer);
  bool CopyFrom(Buffer* buffer);
  virtual bool Setup() { return true; }
  virtual bool Verify() { return true; }
  void KernargRaw(const void* ptr, size_t size, size_t align);

  template <typename T>
  void Kernarg(const T* ptr, size_t size = sizeof(T), size_t align = sizeof(T)) {
    KernargRaw(ptr, size, align);
  }

  void Kernarg(Buffer* buffer);
  uint64_t GetTimestampFrequency();
};

}
}

#endif // DISPATCH_HPP__
