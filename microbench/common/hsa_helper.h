#include <stdio.h>
#include <stdlib.h>
#include <time.h>
#include <cstring>
#include <fstream>
#include <iostream>
#include "hsa.h"

#define check(msg, status)            \
  if (status != HSA_STATUS_SUCCESS) { \
    printf("%s failed.\n", #msg);     \
    exit(1);                          \
  } else {                            \
    printf("%s succeeded.\n", #msg);  \
  }


static hsa_status_t find_regions(hsa_region_t region, void* data);
hsa_status_t find_gpu_device(hsa_agent_t agent, void *data);

class HSAHelper {
 public:
  hsa_agent_t agent;
  hsa_queue_t* queue;
  hsa_region_t system_region;
  hsa_region_t gpu_region;
  hsa_region_t kernarg_region;
  uint8_t* kernarg;
  uint8_t* kernarg_curr;
  uint32_t gridsize_x, gridsize_y, gridsize_z;
  uint32_t groupsize_x, groupsize_y, groupsize_z;

  double now() {
    struct timespec time;
    clock_gettime(CLOCK_MONOTONIC, &time);
    double time_in_sec = static_cast<double>(time.tv_sec) +
                         static_cast<double>(time.tv_nsec) * 1e-9;
    return time_in_sec;
  }

  void init() {
    hsa_status_t err;
    uint64_t queue_size;
    err = hsa_init();
    check(Initializing the hsa runtime, err);

    find_gpu();

    err = hsa_agent_get_info(agent, HSA_AGENT_INFO_QUEUE_MAX_SIZE, &queue_size);
    check(Get agent info queue max size, err);

    err = hsa_queue_create(agent, queue_size, HSA_QUEUE_TYPE_MULTI, NULL, NULL,
                           UINT32_MAX, UINT32_MAX, &queue);
    check(Create queue, err);

    err = hsa_agent_iterate_regions(agent, find_regions, this);
    check(Find region, err);
  }

  uint64_t load_kernel(const char* filename, const char* kernel_name) {
    hsa_status_t err;
    hsa_executable_symbol_t kernel_symbol;
    hsa_code_object_t code_object;
    hsa_executable_t executable;
    uint64_t code_handle;

    std::ifstream in(filename, std::ios::binary | std::ios::ate);
    if (!in) {
      std::cerr << "Error: failed to load " << filename << std::endl;
      return false;
    }
    size_t size = std::string::size_type(in.tellg());
    char* ptr;
    err = hsa_memory_allocate(system_region, size, (void**)&ptr);
    check(Allocate system meory, err) if (!ptr) {
      std::cerr << "Error: failed to allocate memory for code object."
                << std::endl;
      return false;
    }
    in.seekg(0, std::ios::beg);
    std::copy(std::istreambuf_iterator<char>(in),
              std::istreambuf_iterator<char>(), ptr);

    err = hsa_code_object_deserialize(ptr, size, NULL, &code_object);
    check(Deserialize Code Object, err);

    err = hsa_executable_create(HSA_PROFILE_FULL, HSA_EXECUTABLE_STATE_UNFROZEN,
                                NULL, &executable);
    check(Create executable, err);

    // Load code object
    err = hsa_executable_load_code_object(executable, agent, code_object, NULL);
    check(Load code object, err);

    // Freeze executable
    err = hsa_executable_freeze(executable, NULL);
    check(Freeze executable, err);

    // Get symbol handle
    err = hsa_executable_get_symbol(executable, NULL, kernel_name, agent, 0,
                                    &kernel_symbol);
    check(Get symbol handle, err);

    // Get code handle
    err = hsa_executable_symbol_get_info(
        kernel_symbol, HSA_EXECUTABLE_SYMBOL_INFO_KERNEL_OBJECT, &code_handle);
    check(Get code handle, err);

    return code_handle;
  }

  void allocate_kernel_arg(size_t size) {
    hsa_status_t err;
    err = hsa_memory_allocate(kernarg_region, size, (void**)&kernarg);
    check(Allocate kernarg, err);
    kernarg_curr = kernarg;
  }

  void set_kernarg(void* data, size_t size) {
    memcpy(kernarg_curr, data, size);
    kernarg += size;
  }

  void set_grid_size(uint32_t x, uint32_t y, uint32_t z) {
    gridsize_x = x;
    gridsize_y = y;
    gridsize_z = z;
  }

  void set_group_size(uint32_t x, uint32_t y, uint32_t z) {
    groupsize_x = x;
    groupsize_y = y;
    groupsize_z = z;
  }

  void run_kernel(uint64_t code) {
    hsa_status_t err;
    hsa_kernel_dispatch_packet_t* queue_base;
    hsa_kernel_dispatch_packet_t* aql;
    uint32_t queue_mask;
    double kernel_start, kernel_end;
    hsa_signal_t signal;
    uint64_t packet_index;

    err = hsa_signal_create(1, 0, NULL, &signal);
    check(Create signal, err);

    queue_mask = queue->size - 1;
    queue_base = (hsa_kernel_dispatch_packet_t*)queue->base_address;
    packet_index = hsa_queue_add_write_index_relaxed(queue, 1);

    aql = queue_base + (packet_index & queue_mask);
    memset((uint8_t*)aql + 4, 0, sizeof(*aql) - 4);
    aql->completion_signal = signal;
    aql->workgroup_size_x = groupsize_x;
    aql->workgroup_size_y = groupsize_y;
    aql->workgroup_size_z = groupsize_z;
    aql->grid_size_x = gridsize_x;
    aql->grid_size_y = gridsize_y;
    aql->grid_size_z = gridsize_z;
    aql->group_segment_size = 0;
    aql->private_segment_size = 0;
    aql->kernarg_address = kernarg;

    uint16_t header =
        (HSA_PACKET_TYPE_KERNEL_DISPATCH << HSA_PACKET_HEADER_TYPE) |
        (1 << HSA_PACKET_HEADER_BARRIER) |
        (HSA_FENCE_SCOPE_SYSTEM << HSA_PACKET_HEADER_ACQUIRE_FENCE_SCOPE) |
        (HSA_FENCE_SCOPE_SYSTEM << HSA_PACKET_HEADER_RELEASE_FENCE_SCOPE);
    uint16_t dim = 1;
    if (aql->grid_size_y > 1) dim = 2;
    if (aql->grid_size_z > 1) dim = 3;
    // aql->group_segment_size = group_static_size + group_dynamic_size;
    uint16_t setup = dim << HSA_KERNEL_DISPATCH_PACKET_SETUP_DIMENSIONS;
    uint32_t header32 = header | (setup << 16);
    __atomic_store_n((uint32_t*)aql, header32, __ATOMIC_RELEASE);

    // Ring door bell
    kernel_start = now();
    hsa_signal_store_relaxed(queue->doorbell_signal, packet_index);

    hsa_signal_wait_acquire(signal, HSA_SIGNAL_CONDITION_EQ, 0, UINT64_MAX,
                            HSA_WAIT_STATE_ACTIVE);
    kernel_end = now();

    printf("Kernel %0.12f - %0.12f: %0.12f\n", kernel_start, kernel_end,
           kernel_end - kernel_start);
  }

  void shutdown() {
    hsa_status_t err;
    err = hsa_shut_down();
    check(Shutting down the runtime, err);
  }

 private:
  void find_gpu() {
    hsa_status_t err;
    hsa_agent_t agent;
    err = hsa_iterate_agents(find_gpu_device, this);
    if (err == HSA_STATUS_INFO_BREAK) {
      err = HSA_STATUS_SUCCESS;
    }
    check(Getting a gpu agent, err);
  }
};

static hsa_status_t find_regions(hsa_region_t region, void* data) {
  HSAHelper* helper = (HSAHelper*)data;

  hsa_region_segment_t segment;
  hsa_region_get_info(region, HSA_REGION_INFO_SEGMENT, &segment);
  if (HSA_REGION_SEGMENT_GLOBAL != segment) {
    return HSA_STATUS_SUCCESS;
  }

  hsa_region_global_flag_t flags;
  hsa_region_get_info(region, HSA_REGION_INFO_GLOBAL_FLAGS, &flags);

  if (flags & HSA_REGION_GLOBAL_FLAG_FINE_GRAINED) {
    helper->system_region = region;
  }

  if (flags & HSA_REGION_GLOBAL_FLAG_COARSE_GRAINED) {
    helper->gpu_region = region;
  }

  if (flags & HSA_REGION_GLOBAL_FLAG_KERNARG) {
    helper->kernarg_region = region;
  }

  return HSA_STATUS_SUCCESS;
}

hsa_status_t find_gpu_device(hsa_agent_t agent, void *data)
{
  hsa_status_t err;
  hsa_device_type_t hsa_device_type;
  HSAHelper *helper = (HSAHelper *)data;

  if (data == NULL) { return HSA_STATUS_ERROR_INVALID_ARGUMENT; }

  err = hsa_agent_get_info(agent, HSA_AGENT_INFO_DEVICE, &hsa_device_type);
  check(Get agent type, err)

  if (hsa_device_type == HSA_DEVICE_TYPE_GPU) {
    helper->agent = agent;
    return HSA_STATUS_INFO_BREAK;
  }

  return HSA_STATUS_SUCCESS;
}
