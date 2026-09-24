// backprop.cu: CUDA host program for Rodinia Backprop (one training step of
// a two-layer perceptron).
//
// Kernels: amd/benchmarks/rodinia/backprop/native/rodinia_backprop.cpp
// Host:    follows amd/benchmarks/rodinia/backprop/backprop.go
//          (forward, backward, then weight updates: six launches)
//
// Usage: backprop [-input I] [-hidden H] [-output O]
#include "check.h"
#include "rodinia/backprop/native/rodinia_backprop.cpp"

static const float kLearningRate = 0.1f;

static float pseudoRand(unsigned* seed) {
  *seed = *seed * 1664525u + 1013904223u;
  return static_cast<float>(*seed >> 8) / static_cast<float>(1 << 24);
}

static double sigmoid(double x) { return 1.0 / (1.0 + std::exp(-x)); }

int main(int argc, char** argv) {
  const int in = intArg(argc, argv, "input", 64);
  const int hid = intArg(argc, argv, "hidden", 32);
  const int out = intArg(argc, argv, "output", 4);
  std::printf("Backprop: input=%d hidden=%d output=%d\n", in, hid, out);

  std::vector<float> input(in), w1(in * hid), w2(hid * out), b1(hid, 0.0f), b2(out, 0.0f),
      target(out, 1.0f);
  for (int i = 0; i < in; i++) input[i] = static_cast<float>(i % 256) / 256.0f;
  unsigned seed = 42;
  for (float& w : w1) w = (pseudoRand(&seed) - 0.5f) * 0.2f;
  for (float& w : w2) w = (pseudoRand(&seed) - 0.5f) * 0.2f;

  float *dInput = toDevice(input), *dW1 = toDevice(w1), *dB1 = toDevice(b1),
        *dW2 = toDevice(w2), *dB2 = toDevice(b2), *dTarget = toDevice(target);
  float *dHidden = deviceAlloc<float>(hid), *dOutput = deviceAlloc<float>(out),
        *dDeltaOut = deviceAlloc<float>(out), *dDeltaHid = deviceAlloc<float>(hid);

  forward_hidden<<<ceilDiv(hid, BLOCK_SZ), BLOCK_SZ>>>(dInput, dW1, dB1, dHidden, in, hid);
  CHECK_LAUNCH();
  forward_output<<<ceilDiv(out, BLOCK_SZ), BLOCK_SZ>>>(dHidden, dW2, dB2, dOutput, hid, out);
  CHECK_LAUNCH();
  backward_output_delta<<<ceilDiv(out, BLOCK_SZ), BLOCK_SZ>>>(dOutput, dTarget, dDeltaOut, out);
  CHECK_LAUNCH();
  backward_hidden_delta<<<ceilDiv(hid, BLOCK_SZ), BLOCK_SZ>>>(dHidden, dW2, dDeltaOut, dDeltaHid,
                                                              hid, out);
  CHECK_LAUNCH();
  update_w1<<<dim3(ceilDiv(in, TILE2D), ceilDiv(hid, TILE2D)), dim3(TILE2D, TILE2D)>>>(
      dW1, dInput, dDeltaHid, in, hid, kLearningRate);
  CHECK_LAUNCH();
  update_w2<<<ceilDiv(hid, BLOCK_SZ), BLOCK_SZ>>>(dW2, dHidden, dDeltaOut, hid, out,
                                                  kLearningRate);
  CHECK_LAUNCH();

  std::vector<float> gotW1 = fromDevice(dW1, w1.size());
  std::vector<float> gotW2 = fromDevice(dW2, w2.size());

  std::vector<double> hidden(hid), output(out), deltaOut(out), deltaHid(hid);
  for (int j = 0; j < hid; j++) {
    double sum = b1[j];
    for (int i = 0; i < in; i++) sum += static_cast<double>(input[i]) * w1[i * hid + j];
    hidden[j] = sigmoid(sum);
  }
  for (int k = 0; k < out; k++) {
    double sum = b2[k];
    for (int j = 0; j < hid; j++) sum += hidden[j] * w2[j * out + k];
    output[k] = sigmoid(sum);
  }
  for (int k = 0; k < out; k++) deltaOut[k] = output[k] * (1.0 - output[k]) * (target[k] - output[k]);
  for (int j = 0; j < hid; j++) {
    double sum = 0.0;
    for (int k = 0; k < out; k++) sum += w2[j * out + k] * deltaOut[k];
    deltaHid[j] = hidden[j] * (1.0 - hidden[j]) * sum;
  }
  std::vector<double> refW1(w1.size()), refW2(w2.size());
  for (int i = 0; i < in; i++)
    for (int j = 0; j < hid; j++)
      refW1[i * hid + j] = w1[i * hid + j] + kLearningRate * input[i] * deltaHid[j];
  for (int j = 0; j < hid; j++)
    for (int k = 0; k < out; k++)
      refW2[j * out + k] = w2[j * out + k] + kLearningRate * hidden[j] * deltaOut[k];

  // backprop.go floors the denominator at 1e-3 because the weights are small.
  int mismatches = countMismatches("w1", refW1, gotW1, 1e-3, 1e-3) +
                   countMismatches("w2", refW2, gotW2, 1e-3, 1e-3);
  for (float* p : {dInput, dW1, dB1, dW2, dB2, dTarget, dHidden, dOutput, dDeltaOut, dDeltaHid})
    cudaFree(p);
  return finish(mismatches);
}
