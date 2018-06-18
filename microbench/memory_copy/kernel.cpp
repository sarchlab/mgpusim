#include "hsa_helper.h"

#define BYTE_SIZE 1048576

int main(int argc, const char** argv)
{
  hsa_agent_t gpu;
  hsa_region_t global_mem;
  double start;
  double end;

  init();	
  gpu = find_gpu();
  global_mem = find_global_memory(gpu);

  uint8_t *src = (uint8_t *)malloc(BYTE_SIZE);
  uint8_t *dst;
  check(Memory Allocate, hsa_memory_allocate(global_mem, BYTE_SIZE, (void **)&dst));

  start = now();
  hsa_memory_copy(dst, src, BYTE_SIZE);
  end = now();

  printf("Kernel %0.12f - %0.12f: %0.12f\n", start, end, end - start);

  free(src);

  shutdown();
}
