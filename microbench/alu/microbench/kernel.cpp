#include <cstdlib>
#include "dispatch.hpp"

using namespace amd::dispatch;

class EmptyKernelDispatch : public Dispatch {
private:
  Buffer* out;

public:
  EmptyKernelDispatch(int argc, const char **argv)
    : Dispatch(argc, argv) {
  }

  bool SetupCodeObject() override {
    return LoadCodeObjectFromFile("kernels.hsaco");
  }

  bool Setup() override {
    if (!AllocateKernarg(1024)) { return false; }
    out = AllocateBuffer(1024);
    Kernarg(out);
    SetWorkgroupSize(64);
    SetGridSize(64);
    return true;
  }

  bool Verify() override {
    if (!CopyFrom(out)) {
      output << "Error: failed to copy from local" << std::endl;
      return false;
    }
    
    return true;
  }
};

int main(int argc, const char** argv)
{
  return EmptyKernelDispatch(argc, argv).RunMain();
}
