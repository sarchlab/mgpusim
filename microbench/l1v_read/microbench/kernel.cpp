#include <cstdlib>
#include "dispatch.hpp"

using namespace amd::dispatch;

class L1VRead : public Dispatch {
private:
  Buffer* in;
  unsigned repeat;

public:
  L1VRead(int argc, const char **argv)
    : Dispatch(argc, argv) {
    repeat = strtoul(argv[1], nullptr, 10);
  }

  bool SetupCodeObject() override {
    return LoadCodeObjectFromFile("kernels.hsaco");
  }

  bool Setup() override {
    if (!AllocateKernarg(48)) { return false; }
    in  = AllocateBuffer(64);
    Kernarg(in);
    Kernarg(&repeat);
    SetGridSize(64);
    SetWorkgroupSize(64);
    return true;
  }

  bool Verify() override {
    return true;
  }
};

int main(int argc, const char** argv)
{
  return L1VRead(argc, argv).RunMain();
}
