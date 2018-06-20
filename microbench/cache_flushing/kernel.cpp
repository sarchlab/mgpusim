#include <iostream>
#include "dispatch.hpp"


using namespace amd::dispatch;

class CacheFlushingDispatch : public Dispatch {
private:
  Buffer* buf;

public:
  CacheFlushingDispatch(int argc, const char **argv)
    : Dispatch(argc, argv) { }

  bool SetupCodeObject() override {
    return LoadCodeObjectFromFile("kernels.hsaco");
  }

  bool CreateBuffer() {
    buf = AllocateBuffer(16*1024, true);
    return true;
  }

  bool Setup() override {
    if (!AllocateKernarg(32)) { return false; }
    Kernarg(buf);
    SetGridSize(1024*64);
    SetWorkgroupSize(64);
    return true;
  }

  bool MemCopy() {
    double start, end;
    start = Now();
    CopyTo(buf);
    end = Now();
    std::cout << "Memcpy " << end-start << "\n";
  }

  bool Run() override {
    bool res =
      Init() &&
      CreateBuffer() &&
      InitDispatch() &&
      SetupExecutable() &&
      Setup() &&
      MemCopy() &&
      RunDispatch() &&
      Wait() &&
      //MemCopy() &&
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


  bool Verify() override {
    return true;
  }
};



int main(int argc, const char **argv) {

  return CacheFlushingDispatch(argc, argv).RunMain();


  //HSAHelper helper;
  //double start, end;
  //uint64_t kernel;

  //helper.init();
  //kernel = helper.load_kernel("kernels.hsaco", "microbench");

  //uint8_t *src = (uint8_t *)malloc(BYTE_SIZE);
  //uint8_t *dst;
  //check(Memory Allocate,
        //hsa_memory_allocate(helper.system_region, BYTE_SIZE, (void **)&dst));

  //start = helper.now();
  //hsa_memory_copy(dst, src, BYTE_SIZE);
  //end = helper.now();

  //printf("Kernel %0.12f - %0.12f: %0.12f\n", start, end, end - start);

  //helper.allocate_kernel_arg(32);
  //helper.set_kernarg((void *)&dst, 8);
  //helper.set_group_size(64, 1, 1);
  //helper.set_grid_size(64 * 64, 1, 1);
  //helper.run_kernel(kernel);

  //start = helper.now();
  //hsa_memory_copy(dst, src, BYTE_SIZE);
  //end = helper.now();

  //helper.allocate_kernel_arg(32);
  //helper.set_kernarg((void *)&dst, 8);
  //helper.set_group_size(64, 1, 1);
  //helper.set_grid_size(64 * 64, 1, 1);
  //helper.run_kernel(kernel);

  //printf("Kernel %0.12f - %0.12f: %0.12f\n", start, end, end - start);

  //free(src);

  //helper.shutdown();
}
