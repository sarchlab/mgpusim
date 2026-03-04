// fft.cpp — HIP benchmark for 1D FFT (512-point radix-8)
// Kernel copied from amd/benchmarks/shoc/fft/native/fft.cpp
// Problem size: 1MB data (131072 complex float2 pairs = 512*256 elements)

#include "bench_common.h"

#ifndef M_PI
#define M_PI 3.14159265358979323846f
#endif

#ifndef M_SQRT1_2
#define M_SQRT1_2 0.70710678118654752440f
#endif

__device__ __forceinline__ float2 cmplx_mul(float2 a, float2 b) {
    return make_float2(a.x*b.x - a.y*b.y, a.x*b.y + a.y*b.x);
}

__device__ __forceinline__ float2 cm_fl_mul(float2 a, float b) {
    return make_float2(b*a.x, b*a.y);
}

__device__ __forceinline__ float2 cmplx_add(float2 a, float2 b) {
    return make_float2(a.x + b.x, a.y + b.y);
}

__device__ __forceinline__ float2 cmplx_sub(float2 a, float2 b) {
    return make_float2(a.x - b.x, a.y - b.y);
}

__device__ __forceinline__ float2 exp_i(float phi) {
    return make_float2(cosf(phi), sinf(phi));
}

__device__ __forceinline__ void FFT2(float2 *a0, float2 *a1) {
    float2 c0 = *a0;
    *a0 = cmplx_add(c0, *a1);
    *a1 = cmplx_sub(c0, *a1);
}

__device__ __forceinline__ void FFT4(float2 *a0, float2 *a1, float2 *a2, float2 *a3) {
    float2 exp14 = make_float2(0.0f, -1.0f);
    FFT2(a0, a2);
    FFT2(a1, a3);
    *a3 = cmplx_mul(*a3, exp14);
    FFT2(a0, a1);
    FFT2(a2, a3);
}

__device__ __forceinline__ void FFT8(float2 *a) {
    float2 exp18 = make_float2(1.0f, -1.0f);
    float2 exp14 = make_float2(0.0f, -1.0f);
    float2 exp38 = make_float2(-1.0f, -1.0f);

    FFT2(&a[0], &a[4]);
    FFT2(&a[1], &a[5]);
    FFT2(&a[2], &a[6]);
    FFT2(&a[3], &a[7]);

    a[5] = cm_fl_mul(cmplx_mul(a[5], exp18), M_SQRT1_2);
    a[6] = cmplx_mul(a[6], exp14);
    a[7] = cm_fl_mul(cmplx_mul(a[7], exp38), M_SQRT1_2);

    FFT4(&a[0], &a[1], &a[2], &a[3]);
    FFT4(&a[4], &a[5], &a[6], &a[7]);
}

__device__ __forceinline__ void globalLoads8(float2 *data, float2 *in, int stride) {
    data[0] = in[0*stride];
    data[1] = in[1*stride];
    data[2] = in[2*stride];
    data[3] = in[3*stride];
    data[4] = in[4*stride];
    data[5] = in[5*stride];
    data[6] = in[6*stride];
    data[7] = in[7*stride];
}

__device__ __forceinline__ void globalStores8(float2 *data, float2 *out, int stride) {
    out[0*stride] = data[0];
    out[1*stride] = data[4];
    out[2*stride] = data[2];
    out[3*stride] = data[6];
    out[4*stride] = data[1];
    out[5*stride] = data[5];
    out[6*stride] = data[3];
    out[7*stride] = data[7];
}

__device__ __forceinline__ void storex8(float2 *a, float *x, int sx) {
    x[0*sx] = a[0].x;
    x[1*sx] = a[4].x;
    x[2*sx] = a[2].x;
    x[3*sx] = a[6].x;
    x[4*sx] = a[1].x;
    x[5*sx] = a[5].x;
    x[6*sx] = a[3].x;
    x[7*sx] = a[7].x;
}

__device__ __forceinline__ void storey8(float2 *a, float *x, int sx) {
    x[0*sx] = a[0].y;
    x[1*sx] = a[4].y;
    x[2*sx] = a[2].y;
    x[3*sx] = a[6].y;
    x[4*sx] = a[1].y;
    x[5*sx] = a[5].y;
    x[6*sx] = a[3].y;
    x[7*sx] = a[7].y;
}

__device__ __forceinline__ void loadx8(float2 *a, float *x, int sx) {
    a[0].x = x[0*sx];
    a[1].x = x[1*sx];
    a[2].x = x[2*sx];
    a[3].x = x[3*sx];
    a[4].x = x[4*sx];
    a[5].x = x[5*sx];
    a[6].x = x[6*sx];
    a[7].x = x[7*sx];
}

__device__ __forceinline__ void loady8(float2 *a, float *x, int sx) {
    a[0].y = x[0*sx];
    a[1].y = x[1*sx];
    a[2].y = x[2*sx];
    a[3].y = x[3*sx];
    a[4].y = x[4*sx];
    a[5].y = x[5*sx];
    a[6].y = x[6*sx];
    a[7].y = x[7*sx];
}

__device__ __forceinline__ void doTranspose(
    float2 *a, float *s, int ds, float *l, int dl, int sync
) {
    storex8(a, s, ds);  if (sync & 8) __syncthreads();
    loadx8 (a, l, dl);  if (sync & 4) __syncthreads();
    storey8(a, s, ds);  if (sync & 2) __syncthreads();
    loady8 (a, l, dl);  if (sync & 1) __syncthreads();
}

__device__ __forceinline__ void twiddle8(float2 *a, int idx, int n) {
    a[1] = cmplx_mul(a[1], exp_i((-2.0f*M_PI*4/n)*idx));
    a[2] = cmplx_mul(a[2], exp_i((-2.0f*M_PI*2/n)*idx));
    a[3] = cmplx_mul(a[3], exp_i((-2.0f*M_PI*6/n)*idx));
    a[4] = cmplx_mul(a[4], exp_i((-2.0f*M_PI*1/n)*idx));
    a[5] = cmplx_mul(a[5], exp_i((-2.0f*M_PI*5/n)*idx));
    a[6] = cmplx_mul(a[6], exp_i((-2.0f*M_PI*3/n)*idx));
    a[7] = cmplx_mul(a[7], exp_i((-2.0f*M_PI*7/n)*idx));
}

extern "C" __global__ __launch_bounds__(64, 1)
void fft1D_512(float2 *work) {
    int tid = hipThreadIdx_x;
    int blockIdx_val = hipBlockIdx_x * 512 + tid;
    int hi = tid >> 3;
    int lo = tid & 7;
    float2 data[8];
    __shared__ float smem[8 * 8 * 9];

    work = work + blockIdx_val;

    globalLoads8(data, work, 64);

    FFT8(data);

    twiddle8(data, tid, 512);
    doTranspose(data, &smem[hi * 8 + lo], 66, &smem[lo * 66 + hi], 8, 0xf);

    FFT8(data);

    twiddle8(data, hi, 64);
    doTranspose(data, &smem[hi * 8 + lo], 8 * 9, &smem[hi * 8 * 9 + lo], 8, 0xE);

    FFT8(data);

    globalStores8(data, work, 64);
}

int main(int argc, char** argv) {
    int iterations = parseIterations(argc, argv);

    // 1 MB = 1048576 bytes; each float2 = 8 bytes => 131072 complex elements
    // Must be a multiple of 512 for fft1D_512
    const int NUM_ELEMENTS = 131072; // 256 * 512
    const int DATA_SIZE = NUM_ELEMENTS * sizeof(float2);

    // Host allocation
    std::vector<float2> h_data(NUM_ELEMENTS);
    srand(42);
    for (int i = 0; i < NUM_ELEMENTS; i++) {
        h_data[i].x = (float)(rand() % 1000) / 500.0f - 1.0f;
        h_data[i].y = (float)(rand() % 1000) / 500.0f - 1.0f;
    }

    // Device allocation
    float2 *d_data;
    HIP_CHECK(hipMalloc(&d_data, DATA_SIZE));
    HIP_CHECK(hipMemcpy(d_data, h_data.data(), DATA_SIZE, hipMemcpyHostToDevice));

    // 512-point FFTs, 64 threads per block, NUM_ELEMENTS/512 blocks
    const int FFT_SIZE = 512;
    const int THREADS = 64;
    int numFFTs = NUM_ELEMENTS / FFT_SIZE;
    dim3 block(THREADS);
    dim3 grid(numFFTs);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "1MB_%d_elements", NUM_ELEMENTS);

    BenchResult r = runBenchmark("fft1D_512", problemSize, iterations, [&]() {
        fft1D_512<<<grid, block>>>(d_data);
    });

    printCSVHeader();
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_data));

    return 0;
}
