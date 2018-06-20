#include "hsa_helper.h"

#define BYTE_SIZE 1048576

int main(int argc, const char **argv) {
  HSAHelper helper;
  double start, end;
  uint64_t kernel;

  helper.init();
  kernel = helper.load_kernel("kernels.hsaco", "microbench");

  uint8_t *src = (uint8_t *)malloc(BYTE_SIZE);
  uint8_t *dst;
  check(Memory Allocate,
        hsa_memory_allocate(helper.system_region, BYTE_SIZE, (void **)&dst));

  start = helper.now();
  hsa_memory_copy(dst, src, BYTE_SIZE);
  end = helper.now();

  printf("Kernel %0.12f - %0.12f: %0.12f\n", start, end, end - start);

  helper.allocate_kernel_arg(32);
  helper.set_kernarg((void *)&dst, 8);
  helper.set_group_size(64, 1, 1);
  helper.set_grid_size(64 * 64, 1, 1);
  helper.run_kernel(kernel);

  start = helper.now();
  hsa_memory_copy(dst, src, BYTE_SIZE);
  end = helper.now();

  helper.allocate_kernel_arg(32);
  helper.set_kernarg((void *)&dst, 8);
  helper.set_group_size(64, 1, 1);
  helper.set_grid_size(64 * 64, 1, 1);
  helper.run_kernel(kernel);

  printf("Kernel %0.12f - %0.12f: %0.12f\n", start, end, end - start);

  free(src);

  helper.shutdown();
}
