// bs.cpp -- Heteromark Black-Scholes option pricing benchmark (HIP)
// Computes call and put option prices using Black-Scholes formula.

#include "bench_common_hip.h"

// ---------------------------------------------------------------------------
// Device helper: Cumulative Normal Distribution (Abramowitz & Stegun approx)
// ---------------------------------------------------------------------------
__device__ float CND(float d) {
    float K = 1.0f / (1.0f + 0.2316419f * fabsf(d));
    float cnd = 0.3989422804f * expf(-0.5f * d * d) *
        (K * (0.319381530f + K * (-0.356563782f + K * (1.781477937f +
         K * (-1.821255978f + K * 1.330274429f)))));
    if (d > 0) cnd = 1.0f - cnd;
    return cnd;
}

// ---------------------------------------------------------------------------
// Kernel: Black-Scholes option pricing
// ---------------------------------------------------------------------------
__global__ void bs_kernel(const float* __restrict__ stockPrice,
                          const float* __restrict__ strikePrice,
                          const float* __restrict__ years,
                          float* __restrict__ callPrice,
                          float* __restrict__ putPrice,
                          float riskFreeRate,
                          float volatility,
                          int N) {
    int i = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (i >= N) return;

    float S = stockPrice[i];
    float X = strikePrice[i];
    float T = years[i];
    float R = riskFreeRate;
    float V = volatility;

    float sqrtT = sqrtf(T);
    float d1 = (logf(S / X) + (R + 0.5f * V * V) * T) / (V * sqrtT);
    float d2 = d1 - V * sqrtT;

    callPrice[i] = S * CND(d1) - X * expf(-R * T) * CND(d2);
    putPrice[i]  = X * expf(-R * T) * CND(-d2) - S * CND(-d1);
}

// ---------------------------------------------------------------------------
// CPU reference for verification
// ---------------------------------------------------------------------------
static float CND_cpu(float d) {
    float K = 1.0f / (1.0f + 0.2316419f * fabsf(d));
    float cnd = 0.3989422804f * expf(-0.5f * d * d) *
        (K * (0.319381530f + K * (-0.356563782f + K * (1.781477937f +
         K * (-1.821255978f + K * 1.330274429f)))));
    if (d > 0) cnd = 1.0f - cnd;
    return cnd;
}

static void bs_cpu(const float* stockPrice, const float* strikePrice,
                   const float* years, float* callPrice, float* putPrice,
                   float R, float V, int N) {
    for (int i = 0; i < N; ++i) {
        float S = stockPrice[i];
        float X = strikePrice[i];
        float T = years[i];
        float sqrtT = sqrtf(T);
        float d1 = (logf(S / X) + (R + 0.5f * V * V) * T) / (V * sqrtT);
        float d2 = d1 - V * sqrtT;
        callPrice[i] = S * CND_cpu(d1) - X * expf(-R * T) * CND_cpu(d2);
        putPrice[i]  = X * expf(-R * T) * CND_cpu(-d2) - S * CND_cpu(-d1);
    }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------
int main(int argc, char** argv) {
    int N          = parseIntParam(argc, argv, "--size", 1048576);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);
    int iters      = parseIterations(argc, argv);
    float riskFreeRate = 0.02f;
    float volatility   = 0.30f;

    size_t bytes = (size_t)N * sizeof(float);

    // Host allocations
    float* h_stockPrice  = (float*)malloc(bytes);
    float* h_strikePrice = (float*)malloc(bytes);
    float* h_years       = (float*)malloc(bytes);
    float* h_callPrice   = (float*)malloc(bytes);
    float* h_putPrice    = (float*)malloc(bytes);
    float* h_callRef     = (float*)malloc(bytes);
    float* h_putRef      = (float*)malloc(bytes);

    // Initialize with realistic values
    srand(42);
    for (int i = 0; i < N; ++i) {
        h_stockPrice[i]  = 5.0f + (float)(rand() % 1000) * 0.1f;
        h_strikePrice[i] = 1.0f + (float)(rand() % 1000) * 0.1f;
        h_years[i]       = 0.25f + (float)(rand() % 100) * 0.1f;
    }

    // Device allocations
    float *d_stockPrice, *d_strikePrice, *d_years;
    float *d_callPrice, *d_putPrice;

    HIP_CHECK(hipMalloc(&d_stockPrice,  bytes));
    HIP_CHECK(hipMalloc(&d_strikePrice, bytes));
    HIP_CHECK(hipMalloc(&d_years,       bytes));
    HIP_CHECK(hipMalloc(&d_callPrice,   bytes));
    HIP_CHECK(hipMalloc(&d_putPrice,    bytes));

    HIP_CHECK(hipMemcpy(d_stockPrice,  h_stockPrice,  bytes, hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_strikePrice, h_strikePrice, bytes, hipMemcpyHostToDevice));
    HIP_CHECK(hipMemcpy(d_years,       h_years,       bytes, hipMemcpyHostToDevice));

    int grid = (N + block_size - 1) / block_size;

    // Benchmark
    char size_str[64];
    snprintf(size_str, sizeof(size_str), "%d", N);

    printCSVHeader();

    BenchResult r = runBenchmark("bs", size_str, iters, [&]() {
        bs_kernel<<<grid, block_size>>>(d_stockPrice, d_strikePrice, d_years,
                                        d_callPrice, d_putPrice,
                                        riskFreeRate, volatility, N);
    });

    printCSVRow(r);

    // Verify
    HIP_CHECK(hipMemcpy(h_callPrice, d_callPrice, bytes, hipMemcpyDeviceToHost));
    HIP_CHECK(hipMemcpy(h_putPrice,  d_putPrice,  bytes, hipMemcpyDeviceToHost));

    bs_cpu(h_stockPrice, h_strikePrice, h_years, h_callRef, h_putRef,
           riskFreeRate, volatility, N);

    double maxErr = 0.0;
    for (int i = 0; i < N; ++i) {
        double errCall = fabs((double)h_callPrice[i] - (double)h_callRef[i]);
        double errPut  = fabs((double)h_putPrice[i]  - (double)h_putRef[i]);
        if (errCall > maxErr) maxErr = errCall;
        if (errPut  > maxErr) maxErr = errPut;
    }

    fprintf(stderr, "Max absolute error: %e\n", maxErr);
    if (maxErr < 1e-3) {
        fprintf(stderr, "PASS\n");
    } else {
        fprintf(stderr, "FAIL: error too large\n");
    }

    // Cleanup
    HIP_CHECK(hipFree(d_stockPrice));
    HIP_CHECK(hipFree(d_strikePrice));
    HIP_CHECK(hipFree(d_years));
    HIP_CHECK(hipFree(d_callPrice));
    HIP_CHECK(hipFree(d_putPrice));
    free(h_stockPrice);
    free(h_strikePrice);
    free(h_years);
    free(h_callPrice);
    free(h_putPrice);
    free(h_callRef);
    free(h_putRef);

    return (maxErr >= 1e-3) ? 1 : 0;
}
