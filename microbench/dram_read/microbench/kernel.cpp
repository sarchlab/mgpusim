#include <cstdlib>
#include "dispatch.hpp"

using namespace amd::dispatch;

class L1VRead : public Dispatch {
private:
  Buffer* in;

public:
  L1VRead(int argc, const char **argv)
    : Dispatch(argc, argv) {
  }

  bool SetupCodeObject() override {
    return LoadCodeObjectFromFile("kernels.hsaco");
  }

  bool Setup() override {
    if (!AllocateKernarg(32)) { return false; }
    in  = AllocateBuffer(1 << 21); // 2MB
    Kernarg(in);
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
