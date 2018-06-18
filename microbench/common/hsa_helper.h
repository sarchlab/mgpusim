#include <stdio.h>
#include <stdlib.h>
#include <time.h>
#include "hsa.h"

#define check(msg, status) \
  if (status != HSA_STATUS_SUCCESS) { \
    printf("%s failed.\n", #msg); \
    exit(1); \
  } else { \
    printf("%s succeeded.\n", #msg); \
  }

void init() {
  hsa_status_t err;
  err = hsa_init();
  check(Initializing the hsa runtime, err);
}

void shutdown() {
  hsa_status_t err;
  err=hsa_shut_down();
  check(Shutting down the runtime, err);
}

/*
 * Determines if the given agent is of type HSA_DEVICE_TYPE_GPU
 * and sets the value of data to the agent handle if it is.
 */
static hsa_status_t get_gpu_agent(hsa_agent_t agent, void *data) {
  hsa_status_t status;
  hsa_device_type_t device_type;
  status = hsa_agent_get_info(agent, HSA_AGENT_INFO_DEVICE, &device_type);
  if (HSA_STATUS_SUCCESS == status && HSA_DEVICE_TYPE_GPU == device_type) {
    hsa_agent_t* ret = (hsa_agent_t*)data;
    *ret = agent;
    return HSA_STATUS_INFO_BREAK;
  }
  return HSA_STATUS_SUCCESS;
}

hsa_agent_t find_gpu() {
  hsa_status_t err;
  hsa_agent_t agent;
  err = hsa_iterate_agents(get_gpu_agent, &agent);
  if(err == HSA_STATUS_INFO_BREAK) { err = HSA_STATUS_SUCCESS; }
  check(Getting a gpu agent, err);
  return agent;
}



/*
 * Determines if a memory region can be used for kernarg
 * allocations.
 */
static hsa_status_t get_kernarg_memory_region(hsa_region_t region, void* data) {
  hsa_region_segment_t segment;
  hsa_region_get_info(region, HSA_REGION_INFO_SEGMENT, &segment);
  if (HSA_REGION_SEGMENT_GLOBAL != segment) {
    return HSA_STATUS_SUCCESS;
  }

  hsa_region_global_flag_t flags;
  hsa_region_get_info(region, HSA_REGION_INFO_GLOBAL_FLAGS, &flags);
  if (flags & HSA_REGION_GLOBAL_FLAG_KERNARG) {
    hsa_region_t* ret = (hsa_region_t*) data;
    *ret = region;
    return HSA_STATUS_INFO_BREAK;
  }

  return HSA_STATUS_SUCCESS;
}

/*
 * Determines if a memory region is global memory
 */
static hsa_status_t get_global_memory_region(hsa_region_t region, void* data) {
  hsa_region_segment_t segment;
  hsa_region_get_info(region, HSA_REGION_INFO_SEGMENT, &segment);
  if (HSA_REGION_SEGMENT_GLOBAL != segment) {
    return HSA_STATUS_SUCCESS;
  }

  hsa_region_t* ret = (hsa_region_t*) data;
  *ret = region;
  return HSA_STATUS_INFO_BREAK;
}

hsa_region_t find_global_memory(hsa_agent_t agent) {
  hsa_status_t err;

  hsa_region_t region;
  region.handle=(uint64_t)-1;
  hsa_agent_iterate_regions(agent, get_global_memory_region, &region);
  err = (region.handle == (uint64_t)-1) ? HSA_STATUS_ERROR : HSA_STATUS_SUCCESS;
  check(Finding a global memory region, err);

  return region;
}

double now() {
  struct timespec time;
  clock_gettime(CLOCK_MONOTONIC, &time);
  double time_in_sec = static_cast<double>(time.tv_sec) + 
                       static_cast<double>(time.tv_nsec) * 1e-9;
  return time_in_sec;
}




