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
#include "hsa.h"
#include "hsa_ext_amd.h"
#include <cstring>
#include <fstream>
#include <cstdlib>
#include <iostream>

namespace amd {
namespace dispatch {

Dispatch::Dispatch(int argc, const char** argv)
  : queue_size(0),
    queue(0)
{
  agent.handle = 0;
  cpu_agent.handle = 0;
  signal.handle = 0;
  kernarg_region.handle = 0;
  system_region.handle = 0;
  local_region.handle = 0;
  gpu_local_region.handle = 0;
}

hsa_status_t find_gpu_device(hsa_agent_t agent, void *data)
{
  if (data == NULL) { return HSA_STATUS_ERROR_INVALID_ARGUMENT; }

  hsa_device_type_t hsa_device_type;
  hsa_status_t hsa_error_code = hsa_agent_get_info(agent, HSA_AGENT_INFO_DEVICE, &hsa_device_type);
  if (hsa_error_code != HSA_STATUS_SUCCESS) { return hsa_error_code; }

  if (hsa_device_type == HSA_DEVICE_TYPE_GPU) {
    Dispatch* dispatch = static_cast<Dispatch*>(data);
    if (!dispatch->HasAgent()) {
      dispatch->SetAgent(agent);
    }
  }

  if (hsa_device_type == HSA_DEVICE_TYPE_CPU) {
    Dispatch* dispatch = static_cast<Dispatch*>(data);
    if (!dispatch->HasCpuAgent()) {
      dispatch->SetCpuAgent(agent);
    }
  }

  return HSA_STATUS_SUCCESS;
}

hsa_status_t FindRegions(hsa_region_t region, void* data)
{
  hsa_region_segment_t segment_id;
  hsa_region_get_info(region, HSA_REGION_INFO_SEGMENT, &segment_id);

  if (segment_id != HSA_REGION_SEGMENT_GLOBAL) {
    return HSA_STATUS_SUCCESS;
  }

  hsa_region_global_flag_t flags;
  bool host_accessible_region = false;
  hsa_region_get_info(region, HSA_REGION_INFO_GLOBAL_FLAGS, &flags);
  hsa_region_get_info(region, (hsa_region_info_t)HSA_AMD_REGION_INFO_HOST_ACCESSIBLE, &host_accessible_region);

  Dispatch* dispatch = static_cast<Dispatch*>(data);

  if (flags & HSA_REGION_GLOBAL_FLAG_FINE_GRAINED) {
    dispatch->SetSystemRegion(region);
  }

  if (flags & HSA_REGION_GLOBAL_FLAG_COARSE_GRAINED) {
    if(host_accessible_region){
      dispatch->SetLocalRegion(region);
    }else{
      dispatch->SetGPULocalRegion(region);
    }
  }

  if (flags & HSA_REGION_GLOBAL_FLAG_KERNARG) {
    dispatch->SetKernargRegion(region);
  }

  return HSA_STATUS_SUCCESS;
}

bool Dispatch::HsaError(const char* msg, hsa_status_t status)
{
  const char* err = 0;
  if (status != HSA_STATUS_SUCCESS) {
    hsa_status_string(status, &err);
  }
  output << msg << ": " << (err ? err : "unknown error") << std::endl;
  return false;
}

bool Dispatch::Init()
{
  hsa_status_t status;
  status = hsa_init();
  if (status != HSA_STATUS_SUCCESS) { return HsaError("hsa_init failed", status); }

  // Find GPU
  status = hsa_iterate_agents(find_gpu_device, this);
  assert(status == HSA_STATUS_SUCCESS);

  char agent_name[64];
  status = hsa_agent_get_info(agent, HSA_AGENT_INFO_NAME, agent_name);
  if (status != HSA_STATUS_SUCCESS) { return HsaError("hsa_agent_get_info(HSA_AGENT_INFO_NAME) failed", status); }
  output << "Using agent: " << agent_name << std::endl;

  status = hsa_agent_get_info(agent, HSA_AGENT_INFO_QUEUE_MAX_SIZE, &queue_size);
  if (status != HSA_STATUS_SUCCESS) { return HsaError("hsa_agent_get_info(HSA_AGENT_INFO_QUEUE_MAX_SIZE) failed", status); }

  status = hsa_queue_create(agent, queue_size, HSA_QUEUE_TYPE_MULTI, NULL, NULL, UINT32_MAX, UINT32_MAX, &queue);
  if (status != HSA_STATUS_SUCCESS) { return HsaError("hsa_queue_create failed", status); }

  status = hsa_signal_create(1, 0, NULL, &signal);
  if (status != HSA_STATUS_SUCCESS) { return HsaError("hsa_signal_create failed", status); }

  status = hsa_agent_iterate_regions(agent, FindRegions, this);
  if (status != HSA_STATUS_SUCCESS) { return HsaError("Failed to iterate memory regions", status); }
  if (!kernarg_region.handle) { return HsaError("Failed to find kernarg memory region"); }

  return true;
}

bool Dispatch::InitDispatch()
{
  const uint32_t queue_mask = queue->size - 1;
  packet_index = hsa_queue_add_write_index_relaxed(queue, 1);
  aql = (hsa_kernel_dispatch_packet_t*) (hsa_kernel_dispatch_packet_t*)(queue->base_address) + (packet_index & queue_mask);
  memset((uint8_t*)aql + 4, 0, sizeof(*aql) - 4);
  aql->completion_signal = signal;
  aql->workgroup_size_x = 1;
  aql->workgroup_size_y = 1;
  aql->workgroup_size_z = 1;
  aql->grid_size_x = 1;
  aql->grid_size_y = 1;
  aql->grid_size_z = 1;
  aql->group_segment_size = 0;
  aql->private_segment_size = 0;
  return true;
}

bool Dispatch::RunDispatch()
{
  uint16_t header =
    (HSA_PACKET_TYPE_KERNEL_DISPATCH << HSA_PACKET_HEADER_TYPE) |
    (1 << HSA_PACKET_HEADER_BARRIER) |
    (HSA_FENCE_SCOPE_SYSTEM << HSA_PACKET_HEADER_ACQUIRE_FENCE_SCOPE) |
    (HSA_FENCE_SCOPE_SYSTEM << HSA_PACKET_HEADER_RELEASE_FENCE_SCOPE);
  uint16_t dim = 1;
  if (aql->grid_size_y > 1)
    dim = 2;
  if (aql->grid_size_z > 1)
    dim = 3;
  aql->group_segment_size = group_static_size + group_dynamic_size;
  uint16_t setup = dim << HSA_KERNEL_DISPATCH_PACKET_SETUP_DIMENSIONS;
  uint32_t header32 = header | (setup << 16);
  #if defined(_WIN32) || defined(_WIN64)  // Windows
    _InterlockedExchange(aql, header32);
  #else // Linux
    __atomic_store_n((uint32_t*)aql, header32, __ATOMIC_RELEASE);
  #endif
  // Ring door bell
  kernel_start = Now();
  hsa_signal_store_relaxed(queue->doorbell_signal, packet_index);

  return true;
}

void Dispatch::SetWorkgroupSize(uint16_t sizeX, uint16_t sizeY, uint16_t sizeZ)
{
  aql->workgroup_size_x = sizeX;
  aql->workgroup_size_y = sizeY;
  aql->workgroup_size_z = sizeZ;
}

void Dispatch::SetGridSize(uint32_t sizeX, uint32_t sizeY, uint32_t sizeZ)
{
  aql->grid_size_x = sizeX;
  aql->grid_size_y = sizeY;
  aql->grid_size_z = sizeZ;
}

void Dispatch::SetSystemRegion(hsa_region_t region)
{
  system_region = region;
}

void Dispatch::SetKernargRegion(hsa_region_t region)
{
  kernarg_region = region;
}

void Dispatch::SetGPULocalRegion(hsa_region_t region)
{
  gpu_local_region = region;
}

void Dispatch::SetLocalRegion(hsa_region_t region)
{
  local_region = region;
}

void Dispatch::SetDynamicGroupSegmentSize(uint32_t size)
{
  group_dynamic_size = size;
}

bool Dispatch::AllocateKernarg(uint32_t size)
{
  hsa_status_t status;
  status = hsa_memory_allocate(kernarg_region, size, &kernarg);
  if (status != HSA_STATUS_SUCCESS) { return HsaError("Failed to allocate kernarg", status); }
  aql->kernarg_address = kernarg;
  kernarg_offset = 0;
  return true;
}

bool Dispatch::LoadCodeObjectFromFile(const std::string& filename)
{
  std::ifstream in(filename.c_str(), std::ios::binary | std::ios::ate);
  if (!in) { output << "Error: failed to load " << filename << std::endl; return false; }
  size_t size = std::string::size_type(in.tellg());
  char *ptr = (char*) AllocateSystemMemory(size);
  if (!ptr) {
    output << "Error: failed to allocate memory for code object." << std::endl;
    return false;
  }
  in.seekg(0, std::ios::beg);
  std::copy(std::istreambuf_iterator<char>(in),
            std::istreambuf_iterator<char>(),
            ptr);
/*
  res.assign((std::istreambuf_iterator<char>(in)),
              std::istreambuf_iterator<char>());

*/
  hsa_status_t status = hsa_code_object_deserialize(ptr, size, NULL, &code_object);
  if (status != HSA_STATUS_SUCCESS) { return HsaError("Failed to deserialize code object", status); }
  return true;
}

bool Dispatch::SetupCodeObject()
{
  return false;
}

bool Dispatch::SetupExecutable()
{
  hsa_status_t status;
  hsa_executable_symbol_t kernel_symbol;

  if (!SetupCodeObject()) { return false; }
  status = hsa_executable_create(HSA_PROFILE_FULL, HSA_EXECUTABLE_STATE_UNFROZEN,
                                 NULL, &executable);
  if (status != HSA_STATUS_SUCCESS) { return HsaError("hsa_executable_create failed", status); }

  // Load code object
  status = hsa_executable_load_code_object(executable, agent, code_object, NULL);
  if (status != HSA_STATUS_SUCCESS) { return HsaError("hsa_executable_load_code_object failed", status); }

  // Freeze executable
  status = hsa_executable_freeze(executable, NULL);
  if (status != HSA_STATUS_SUCCESS) { return HsaError("hsa_executable_freeze failed", status); }

  // Get symbol handle
  status = hsa_executable_get_symbol(executable, NULL, "microbench", agent,
                                     0, &kernel_symbol);
  if (status != HSA_STATUS_SUCCESS) { return HsaError("hsa_executable_get_symbol failed", status); }

  // Get code handle
  uint64_t code_handle;
  status = hsa_executable_symbol_get_info(kernel_symbol,
                                          HSA_EXECUTABLE_SYMBOL_INFO_KERNEL_OBJECT,
                                          &code_handle);
  if (status != HSA_STATUS_SUCCESS) { return HsaError("hsa_executable_symbol_get_info failed", status); }

  status = hsa_executable_symbol_get_info(kernel_symbol,
                HSA_EXECUTABLE_SYMBOL_INFO_KERNEL_GROUP_SEGMENT_SIZE,
                &group_static_size);
  if (status != HSA_STATUS_SUCCESS) { return HsaError("hsa_executable_symbol_get_info failed", status); }

  aql->kernel_object = code_handle;

  return true;
}

uint64_t TIMEOUT = 120;

bool Dispatch::Wait()
{
  clock_t beg = clock();
  hsa_signal_value_t result;
  do {
    result = hsa_signal_wait_acquire(signal,
      HSA_SIGNAL_CONDITION_EQ, 0, ~0ULL, HSA_WAIT_STATE_ACTIVE);
    clock_t clocks = clock() - beg;
    if (clocks > (clock_t) TIMEOUT * CLOCKS_PER_SEC) {
      output << "Kernel execution timed out, elapsed time: " << (long) clocks << std::endl;
      return false;
    }
  } while (result != 0);

  kernel_end = Now();
  printf("Kernel %0.12f - %0.12f: %0.12f\n", kernel_start, kernel_end, 
         kernel_end - kernel_start);
  return true;
}

void* Dispatch::AllocateGPULocalMemory(size_t size)
{
  assert(gpu_local_region.handle != 0);
  void *p = NULL;

  hsa_status_t status = hsa_memory_allocate(gpu_local_region, size, (void **)&p);
  if (status != HSA_STATUS_SUCCESS) { HsaError("hsa_memory_allocate(gpu_local_region) failed", status); return 0; }
  return p;
}

void* Dispatch::AllocateLocalMemory(size_t size)
{
  assert(local_region.handle != 0);
  void *p = NULL;

  hsa_status_t status = hsa_memory_allocate(local_region, size, (void **)&p);
  if (status != HSA_STATUS_SUCCESS) { HsaError("hsa_memory_allocate(local_region) failed", status); return 0; }
  //status = hsa_memory_assign_agent(p, agent, HSA_ACCESS_PERMISSION_RW);
  //if (status != HSA_STATUS_SUCCESS) { HsaError("hsa_memory_assign_agent failed", status); return 0; }
  return p;
}

void* Dispatch::AllocateSystemMemory(size_t size)
{
  void *p = NULL;
  hsa_status_t status = hsa_memory_allocate(system_region, size, (void **)&p);
  if (status != HSA_STATUS_SUCCESS) { HsaError("hsa_memory_allocate(system_region) failed", status); return 0; }
  return p;
}

bool Dispatch::CopyToLocal(void* dest, void* src, size_t size)
{
  hsa_status_t status;
  status = hsa_memory_copy(dest, src, size);
  if (status != HSA_STATUS_SUCCESS) { HsaError("hsa_memory_copy failed", status); return false; }
  //status = hsa_memory_assign_agent(dest, agent, HSA_ACCESS_PERMISSION_RW);
  //if (status != HSA_STATUS_SUCCESS) { HsaError("hsa_memory_assign_agent failed", status); return false; }
  return true;
}

bool Dispatch::CopyFromLocal(void* dest, void* src, size_t size)
{
  hsa_status_t status;
  status = hsa_memory_assign_agent(src, cpu_agent, HSA_ACCESS_PERMISSION_RW);
  if (status != HSA_STATUS_SUCCESS) { HsaError("hsa_memory_assign_agent failed", status); return false; }
  status = hsa_memory_copy(dest, src, size);
  if (status != HSA_STATUS_SUCCESS) { HsaError("hsa_memory_copy failed", status); return false; }
  return true;
}

Buffer* Dispatch::AllocateBuffer(size_t size, bool prefer_gpu_local)
{
  void* system_ptr = AllocateSystemMemory(size);
  if (!system_ptr) { return 0; }

  if (prefer_gpu_local && gpu_local_region.handle != 0) {
    void* local_ptr = AllocateGPULocalMemory(size);
    if (!local_ptr) { free(system_ptr); return 0; }
    return new Buffer(size, local_ptr, system_ptr);
  }

  if (local_region.handle != 0) {
    void* local_ptr = AllocateLocalMemory(size);
    if (!local_ptr) { free(system_ptr); return 0; }
    return new Buffer(size, local_ptr, system_ptr);
  } else if (gpu_local_region.handle != 0) {
    void* local_ptr = AllocateGPULocalMemory(size);
    if (!local_ptr) { free(system_ptr); return 0; }
    return new Buffer(size, local_ptr, system_ptr);
  } else {
    return new Buffer(size, system_ptr);
  }
}

bool Dispatch::CopyTo(Buffer* buffer)
{
  if (buffer->IsLocal()) {
    return CopyToLocal(buffer->LocalPtr(), buffer->SystemPtr(), buffer->Size());
  }
  return true;
}

bool Dispatch::CopyFrom(Buffer* buffer)
{
  if (buffer->IsLocal()) {
    return CopyFromLocal(buffer->SystemPtr(), buffer->LocalPtr(), buffer->Size());
  }
  return true;
}

void Dispatch::KernargRaw(const void* ptr, size_t size, size_t align)
{
  assert((align & (align - 1)) == 0);
  kernarg_offset = ((kernarg_offset + align - 1) / align) * align;
  memcpy((char*) kernarg + kernarg_offset, ptr, size);
  kernarg_offset += size;
}

void Dispatch::Kernarg(Buffer* buffer)
{
  void* localPtr = buffer->LocalPtr();
  Kernarg(&localPtr);
}

bool Dispatch::Shutdown() {
  hsa_status_t status;
  status = hsa_shut_down();
  if (status != HSA_STATUS_SUCCESS) { return HsaError("hsa_shut_down failed", status); }
  return true;
 
}

bool Dispatch::Run()
{
  bool res =
    Init() &&
    InitDispatch() &&
    SetupExecutable() &&
    Setup() &&
    RunDispatch() &&
    Wait() &&
    Verify() &&
    Shutdown();
  std::string out = output.str();
  if (!out.empty()) {
    std::cout << out << std::endl;
  }
  std::cout << (res ? "Success" : "Failed") << std::endl;
  return res;
}

int Dispatch::RunMain()
{
  return Run() ? 0 : 1;
}

uint64_t Dispatch::GetTimestampFrequency()
{
  uint64_t frequency;
  hsa_status_t status;
  status = hsa_system_get_info(HSA_SYSTEM_INFO_TIMESTAMP_FREQUENCY, &frequency);
  if (status != HSA_STATUS_SUCCESS) {
    HsaError("hsa_system_get_info(HSA_SYSTEM_INFO_TIMESTAMP_FREQUENCY) failed", status);
    return 0;
  }

  return frequency;
}

}
}
