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
    in  = AllocateBuffer(64);
    Kernarg(in);
    SetGridSize(1);
    SetWorkgroupSize(1);
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
